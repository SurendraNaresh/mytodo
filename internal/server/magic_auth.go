package server

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/model"
)

type magicToken struct {
	userID  int64
	expires time.Time
}

func (s *Server) requestMagicLink(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	address, err := normalizeEmail(input.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, "a valid email address is required")
		return
	}
	if s.sendMagicEmail == nil && os.Getenv("SMTP_HOST") == "" {
		writeError(w, http.StatusServiceUnavailable, "passwordless email is not configured")
		return
	}

	user, err := loadMagicUser(address)
	if err != nil && err != sql.ErrNoRows {
		writeError(w, http.StatusInternalServerError, "could not look up account")
		return
	}
	if err == sql.ErrNoRows {
		name := strings.TrimSpace(input.Name)
		if name == "" || len(name) > 120 {
			writeError(w, http.StatusBadRequest, "your name is required to create an account")
			return
		}
		result, createErr := db.DB().Exec(`INSERT INTO users (name, dob, email, role, password_hash) VALUES (?, '', ?, 'Member', '')`, name, address)
		if createErr != nil {
			writeError(w, http.StatusConflict, "could not create account")
			return
		}
		userID, idErr := result.LastInsertId()
		if idErr != nil {
			writeError(w, http.StatusInternalServerError, "could not create account")
			return
		}
		user, err = loadMagicUserByID(userID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load account")
			return
		}
	}

	s.magicMu.Lock()
	now := time.Now()
	clientIP, _, splitErr := net.SplitHostPort(r.RemoteAddr)
	if splitErr != nil {
		clientIP = r.RemoteAddr
	}
	emailKey, ipKey := "email:"+address, "ip:"+clientIP
	for key, last := range s.requested {
		if now.Sub(last) >= time.Hour {
			delete(s.requested, key)
		}
	}
	for token, pending := range s.magic {
		if now.After(pending.expires) {
			delete(s.magic, token)
		}
	}
	_, emailLimited := s.requested[emailKey]
	_, ipLimited := s.requested[ipKey]
	if (emailLimited && now.Sub(s.requested[emailKey]) < time.Minute) || (ipLimited && now.Sub(s.requested[ipKey]) < time.Minute) {
		s.magicMu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]string{"message": "If this address can join the archive, a sign-in link is on its way."})
		return
	}
	s.requested[emailKey], s.requested[ipKey] = now, now
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		s.magicMu.Unlock()
		writeError(w, http.StatusInternalServerError, "could not create sign-in link")
		return
	}
	token := hex.EncodeToString(tokenBytes)
	s.magic[token] = magicToken{userID: user.ID, expires: time.Now().Add(15 * time.Minute)}
	s.magicMu.Unlock()

	link, err := magicLinkURL(token)
	if err == nil {
		sender := s.sendMagicEmail
		if sender == nil {
			sender = sendMagicLinkEmail
		}
		err = sender(address, user.Name, link)
	}
	if err != nil {
		s.magicMu.Lock()
		delete(s.magic, token)
		s.magicMu.Unlock()
		writeError(w, http.StatusServiceUnavailable, "could not send sign-in link")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"message": "If this address can join the archive, a sign-in link is on its way."})
}

func (s *Server) verifyMagicLink(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &input); err != nil || len(input.Token) != 64 {
		writeError(w, http.StatusBadRequest, "invalid sign-in link")
		return
	}
	s.magicMu.Lock()
	entry, exists := s.magic[input.Token]
	delete(s.magic, input.Token)
	s.magicMu.Unlock()
	if !exists || time.Now().After(entry.expires) {
		writeError(w, http.StatusUnauthorized, "sign-in link is invalid or expired")
		return
	}
	user, err := model.GetUser(entry.userID)
	if err != nil || user == nil {
		writeError(w, http.StatusUnauthorized, "account is no longer available")
		return
	}
	expires := time.Now().Add(12 * time.Hour)
	sessionToken, err := s.issueSession(user, expires)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user), "token": sessionToken})
}

func loadMagicUser(email string) (*model.User, error) {
	var id int64
	err := db.DB().QueryRow(`SELECT id FROM users WHERE lower(email) = lower(?)`, email).Scan(&id)
	if err != nil {
		return nil, err
	}
	return loadMagicUserByID(id)
}

func loadMagicUserByID(id int64) (*model.User, error) {
	var user model.User
	var role string
	err := db.DB().QueryRow(`SELECT id, parent_id, name, dob, email, role, password_hash FROM users WHERE id = ?`, id).
		Scan(&user.ID, &user.ParentID, &user.Name, &user.DOB, &user.Email, &role, &user.PasswordHash)
	user.Role = model.Role(role)
	return &user, err
}

func normalizeEmail(value string) (string, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || len(value) > 254 {
		return "", fmt.Errorf("invalid email")
	}
	return value, nil
}

func magicLinkURL(token string) (string, error) {
	base := strings.TrimRight(os.Getenv("MYTODO_PUBLIC_URL"), "/")
	if base == "" {
		base = "http://localhost:8080"
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
		return "", fmt.Errorf("invalid public URL")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/"
	parsed.RawQuery = "token=" + url.QueryEscape(token)
	return parsed.String(), nil
}

func sendMagicLinkEmail(to, name, link string) error {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	from := strings.TrimSpace(os.Getenv("SMTP_FROM"))
	if host == "" || from == "" {
		return fmt.Errorf("SMTP_HOST and SMTP_FROM are required")
	}
	port := strings.TrimSpace(os.Getenv("SMTP_PORT"))
	if port == "" {
		port = "587"
	}
	fromAddress, err := mail.ParseAddress(from)
	if err != nil || fromAddress.Address != from {
		return fmt.Errorf("invalid SMTP_FROM")
	}
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Your Little Years sign-in link\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nHello %s,\r\n\r\nUse this one-time link to sign in. It expires in 15 minutes.\r\n\r\n%s\r\n", from, to, strings.ReplaceAll(strings.ReplaceAll(name, "\r", ""), "\n", ""), link)
	var auth smtp.Auth
	if username := os.Getenv("SMTP_USER"); username != "" {
		auth = smtp.PlainAuth("", username, os.Getenv("SMTP_PASSWORD"), host)
	}
	return smtp.SendMail(net.JoinHostPort(host, port), auth, fromAddress.Address, []string{to}, []byte(message))
}
