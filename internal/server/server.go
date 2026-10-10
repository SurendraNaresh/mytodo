package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/db"
	"github.com/SurendraNaresh/mytodo/internal/model"
)

const maxImportBytes = 256 << 20

type Server struct {
	mux            *http.ServeMux
	requestMu      sync.RWMutex
	sessionMu      sync.Mutex
	sessions       map[string]session
	sessionSecret  []byte
	magicMu        sync.Mutex
	magic          map[string]magicToken
	requested      map[string]time.Time
	sendMagicEmail func(string, string, string) error
	probeDuration  func(string) (float64, error)
}

type session struct {
	userID  int64
	expires time.Time
}

type sessionClaims struct {
	Subject int64  `json:"sub"`
	Role    string `json:"role"`
	Issued  int64  `json:"iat"`
	Expires int64  `json:"exp"`
	Nonce   string `json:"jti"`
}

type contextKey int

const authenticatedUserKey contextKey = iota

func New() *Server {
	secret := []byte(os.Getenv("SESSION_SECRET"))
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			secret = []byte("local-development-session-secret")
		}
	}
	s := &Server{mux: http.NewServeMux(), sessions: make(map[string]session), sessionSecret: secret, magic: make(map[string]magicToken), requested: make(map[string]time.Time)}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/api/v1/admin/import" || r.URL.Path == "/api/v1/auth/bootstrap" {
		s.requestMu.Lock()
		defer s.requestMu.Unlock()
	} else {
		s.requestMu.RLock()
		defer s.requestMu.RUnlock()
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.healthz)
	const prefix = "/api/v1"
	s.mux.HandleFunc("GET "+prefix+"/status", s.status)
	s.mux.HandleFunc("GET "+prefix+"/eras", s.listEras)
	s.mux.HandleFunc("GET "+prefix+"/eras/{slug}/albums", s.listEraAlbums)
	s.mux.HandleFunc("GET "+prefix+"/albums/{id}/media", s.listAlbumMedia)
	s.mux.HandleFunc("GET "+prefix+"/layout", s.listLayoutFrames)
	s.mux.HandleFunc("GET "+prefix+"/donate/link", s.donateLink)
	s.mux.HandleFunc("GET "+prefix+"/donate/qr", s.donateQR)
	s.mux.HandleFunc("GET "+prefix+"/donate/status", s.donateStatus)
	s.mux.HandleFunc("GET "+prefix+"/media/{id}/comments", s.authenticated(s.listMediaComments))
	s.mux.HandleFunc("POST "+prefix+"/media/{id}/comments", s.authenticated(s.createMediaComment))
	s.mux.HandleFunc("PUT "+prefix+"/admin/layout/{region}", s.authenticated(s.saveLayoutFrame))
	s.mux.HandleFunc("POST "+prefix+"/admin/eras", s.authenticated(s.createEra))
	s.mux.HandleFunc("POST "+prefix+"/admin/eras/{id}/albums", s.authenticated(s.createAlbum))
	s.mux.HandleFunc("POST "+prefix+"/admin/albums/{id}/media", s.authenticated(s.uploadMedia))
	s.mux.HandleFunc("GET /media/{id}", s.serveMedia)
	s.mux.HandleFunc("GET /thumbs/{id}", s.serveThumbnail)
	s.mux.HandleFunc("POST "+prefix+"/auth/login", s.login)
	s.mux.HandleFunc("POST "+prefix+"/auth/magic-link", s.requestMagicLink)
	s.mux.HandleFunc("POST "+prefix+"/auth/verify", s.verifyMagicLink)
	s.mux.HandleFunc("POST "+prefix+"/auth/logout", s.authenticated(s.logout))
	s.mux.HandleFunc("GET "+prefix+"/auth/me", s.authenticated(s.me))
	s.mux.HandleFunc("POST "+prefix+"/auth/password/change", s.authenticated(s.changePassword))
	s.mux.HandleFunc("POST "+prefix+"/auth/password/reset-requests", s.authenticated(s.createPasswordResetRequest))
	s.mux.HandleFunc("GET "+prefix+"/admin/password-policy", s.authenticated(s.getPasswordPolicy))
	s.mux.HandleFunc("PUT "+prefix+"/admin/password-policy", s.authenticated(s.updatePasswordPolicy))
	s.mux.HandleFunc("GET "+prefix+"/admin/password-reset-requests", s.authenticated(s.listPasswordResetRequests))
	s.mux.HandleFunc("POST "+prefix+"/admin/password-reset-requests/{id}/reset", s.authenticated(s.resetRequestedPassword))
	s.mux.HandleFunc("GET "+prefix+"/users", s.authenticated(s.listUsers))
	s.mux.HandleFunc("POST "+prefix+"/auth/bootstrap", s.bootstrap)
	s.mux.HandleFunc("POST "+prefix+"/users", s.authenticated(s.createUser))
	s.mux.HandleFunc("GET "+prefix+"/users/{id}", s.authenticated(s.getUser))
	s.mux.HandleFunc("PUT "+prefix+"/users/{id}", s.authenticated(s.updateUser))
	s.mux.HandleFunc("DELETE "+prefix+"/users/{id}", s.authenticated(s.deleteUser))
	s.mux.HandleFunc("GET "+prefix+"/projects", s.authenticated(s.listProjects))
	s.mux.HandleFunc("GET "+prefix+"/projects/{id}", s.authenticated(s.getProject))
	s.mux.HandleFunc("POST "+prefix+"/projects", s.authenticated(s.createProject))
	s.mux.HandleFunc("PUT "+prefix+"/projects/{id}", s.authenticated(s.updateProject))
	s.mux.HandleFunc("DELETE "+prefix+"/projects/{id}", s.authenticated(s.deleteProject))
	s.mux.HandleFunc("GET "+prefix+"/projects/{id}/tasks", s.authenticated(s.listTasks))
	s.mux.HandleFunc("POST "+prefix+"/projects/{id}/tasks", s.authenticated(s.createTask))
	s.mux.HandleFunc("PUT "+prefix+"/tasks/{id}", s.authenticated(s.updateTask))
	s.mux.HandleFunc("DELETE "+prefix+"/tasks/{id}", s.authenticated(s.deleteTask))
	s.mux.HandleFunc("GET "+prefix+"/tasks/{id}", s.authenticated(s.getTask))
	s.mux.HandleFunc("GET "+prefix+"/events", s.authenticated(s.listEvents))
	s.mux.HandleFunc("GET "+prefix+"/event-invitees", s.authenticated(s.listEventInvitees))
	s.mux.HandleFunc("POST "+prefix+"/events", s.authenticated(s.createEvent))
	s.mux.HandleFunc("PUT "+prefix+"/events/{id}", s.authenticated(s.updateEvent))
	s.mux.HandleFunc("DELETE "+prefix+"/events/{id}", s.authenticated(s.deleteEvent))
	s.mux.HandleFunc("POST "+prefix+"/events/{id}/deactivate", s.authenticated(s.deactivateEvent))
	s.mux.HandleFunc("GET "+prefix+"/events/{id}/summary", s.authenticated(s.eventVoteSummary))
	s.mux.HandleFunc("GET "+prefix+"/events/{id}/vote", s.authenticated(s.getVote))
	s.mux.HandleFunc("PUT "+prefix+"/events/{id}/vote", s.authenticated(s.saveVote))
	s.mux.HandleFunc("GET "+prefix+"/artifacts", s.authenticated(s.listArtifacts))
	s.mux.HandleFunc("POST "+prefix+"/artifacts", s.authenticated(s.createArtifact))
	s.mux.HandleFunc("GET "+prefix+"/artifacts/{id}", s.authenticated(s.getArtifact))
	s.mux.HandleFunc("GET "+prefix+"/artifacts/{id}/file", s.authenticated(s.downloadArtifact))
	s.mux.HandleFunc("PUT "+prefix+"/artifacts/{id}", s.authenticated(s.updateArtifact))
	s.mux.HandleFunc("DELETE "+prefix+"/artifacts/{id}", s.authenticated(s.deleteArtifact))
	s.mux.HandleFunc("GET "+prefix+"/admin/backup", s.authenticated(s.backupDatabase))
	s.mux.HandleFunc("POST "+prefix+"/admin/import", s.authenticated(s.importDatabase))
}

func (s *Server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		claims, err := s.parseSessionToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired bearer token")
			return
		}
		s.sessionMu.Lock()
		sessionID := s.sessionID(token)
		stored, ok := s.sessions[sessionID]
		if ok && time.Now().After(stored.expires) {
			delete(s.sessions, sessionID)
			ok = false
		}
		s.sessionMu.Unlock()
		if !ok {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		if claims.Subject != stored.userID || claims.Expires != stored.expires.Unix() {
			writeError(w, http.StatusUnauthorized, "session is no longer valid")
			return
		}
		user, err := model.GetUser(stored.userID)
		if err != nil || user == nil || claims.Role != string(user.Role) {
			writeError(w, http.StatusUnauthorized, "session is no longer valid")
			return
		}
		r = r.WithContext(withUser(r, user))
		next(w, r)
	}
}

func withUser(r *http.Request, user *model.User) context.Context {
	return context.WithValue(r.Context(), authenticatedUserKey, user)
}

func currentUser(r *http.Request) *model.User {
	user, _ := r.Context().Value(authenticatedUserKey).(*model.User)
	return user
}

func (s *Server) sessionID(token string) string {
	mac := hmac.New(sha256.New, s.sessionSecret)
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Server) storeSession(token string, value session) {
	s.sessionMu.Lock()
	s.sessions[s.sessionID(token)] = value
	s.sessionMu.Unlock()
}

func (s *Server) issueSession(user *model.User, expires time.Time) (string, error) {
	if user == nil || user.ID <= 0 || !expires.After(time.Now()) {
		return "", errors.New("invalid session identity or expiry")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	claims := sessionClaims{
		Subject: user.ID, Role: string(user.Role), Issued: time.Now().Unix(),
		Expires: expires.Unix(), Nonce: hex.EncodeToString(nonce),
	}
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.sessionSecret)
	_, _ = mac.Write([]byte(unsigned))
	token := unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	s.storeSession(token, session{userID: user.ID, expires: time.Unix(expires.Unix(), 0)})
	return token, nil
}

func (s *Server) parseSessionToken(token string) (sessionClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return sessionClaims{}, errors.New("invalid JWT")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return sessionClaims{}, errors.New("invalid JWT header")
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return sessionClaims{}, errors.New("unsupported JWT header")
	}
	unsigned := parts[0] + "." + parts[1]
	mac := hmac.New(sha256.New, s.sessionSecret)
	_, _ = mac.Write([]byte(unsigned))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return sessionClaims{}, errors.New("invalid JWT signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return sessionClaims{}, errors.New("invalid JWT claims")
	}
	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil || claims.Subject <= 0 || claims.Role == "" || claims.Nonce == "" || claims.Expires <= time.Now().Unix() || claims.Issued > time.Now().Add(time.Minute).Unix() {
		return sessionClaims{}, errors.New("invalid or expired JWT claims")
	}
	return claims, nil
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	users, err := model.GetUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"needs_setup": len(users) == 0})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := model.GetUserByEmail(input.Email)
	if err != nil || user == nil || !user.VerifyPassword(input.Password) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	token, err := s.issueSession(user, time.Now().Add(12*time.Hour))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": publicUser(user), "token": token})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.sessionMu.Lock()
	delete(s.sessions, s.sessionID(token))
	s.sessionMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, publicUser(currentUser(r)))
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	users, err := model.GetUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]api.User, 0, len(users))
	for index := range users {
		result = append(result, publicUser(&users[index]))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	current := currentUser(r)
	if id != current.ID && !isAdmin(r) && (!current.ParentID.Valid || current.ParentID.Int64 != id) {
		writeError(w, http.StatusForbidden, "user access denied")
		return
	}
	user, err := model.GetUser(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, publicUser(user))
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	var input struct {
		User     api.User `json:"user"`
		Password string   `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateNewPassword(input.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var parentID sql.NullInt64
	if input.User.ParentID != nil {
		parentID = sql.NullInt64{Int64: *input.User.ParentID, Valid: true}
	}
	user, err := model.CreateUser(input.User.Name, input.User.DOB, input.User.Email, model.Role(input.User.Role), input.Password, parentID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(user))
}

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	users, err := model.GetUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(users) != 0 {
		writeError(w, http.StatusConflict, "an administrator already exists")
		return
	}
	var input struct {
		User     api.User `json:"user"`
		Password string   `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.User.Role != string(model.RoleAdmin) {
		writeError(w, http.StatusBadRequest, "the first account must be an administrator")
		return
	}
	if err := validateNewPassword(input.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user, err := model.CreateUser(input.User.Name, input.User.DOB, input.User.Email, model.RoleAdmin, input.Password, sql.NullInt64{})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, publicUser(user))
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input struct {
		User     api.User `json:"user"`
		Password string   `json:"password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Password != "" {
		if err := validateNewPassword(input.Password); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	user, err := model.GetUser(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	user.Name, user.DOB, user.Email, user.Role = input.User.Name, input.User.DOB, input.User.Email, model.Role(input.User.Role)
	user.ParentID = sql.NullInt64{}
	if input.User.ParentID != nil {
		user.ParentID = sql.NullInt64{Int64: *input.User.ParentID, Valid: true}
	}
	if err := model.UpdateUser(user, input.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicUser(user))
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if id == user.ID {
		writeError(w, http.StatusBadRequest, "cannot delete the logged-in user")
		return
	}
	if err := model.DeleteUser(id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := model.ProjectsForUser(currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]api.Project, 0, len(projects))
	for _, project := range projects {
		result = append(result, projectDTO(project))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	project, err := ownedProject(id, currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, http.StatusOK, projectDTO(*project))
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var input api.Project
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	project, err := model.CreateProject(currentUser(r).ID, input.Name, input.Description)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, projectDTO(*project))
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	project, err := model.GetProject(id)
	if err != nil || project.OwnerID != currentUser(r).ID {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var input api.Project
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	project.Name, project.Description = input.Name, input.Description
	if err := model.UpdateProject(project); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, projectDTO(*project))
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := model.DeleteProject(id, currentUser(r).ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	projectID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := ownedProject(projectID, currentUser(r).ID); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	tasks, err := model.TasksForParent(projectID, currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	result := make([]api.Task, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, taskDTO(task))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err := model.GetTask(id)
	if err != nil || task.UserID != currentUser(r).ID {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, http.StatusOK, taskDTO(*task))
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	projectID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := ownedProject(projectID, currentUser(r).ID); err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	var input api.Task
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err := model.CreateTask(projectID, currentUser(r).ID, input.Name, input.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, taskDTO(*task))
}

func (s *Server) updateTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	task, err := model.GetTask(id)
	if err != nil || task.UserID != currentUser(r).ID {
		writeError(w, http.StatusNotFound, "task not found")
		return
	}
	var input api.Task
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := ownedProject(input.ParentID, currentUser(r).ID); err != nil {
		writeError(w, http.StatusBadRequest, "project not found")
		return
	}
	task.ParentID, task.Name, task.DueDate, task.Done = input.ParentID, input.Name, input.DueDate, input.Done
	if err := model.UpdateTask(task); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, taskDTO(*task))
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := model.DeleteTask(id, currentUser(r).ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	query := `SELECT id, title, COALESCE(description, ''), event_type, event_class, event_date, opens_at, closes_at, COALESCE(owner_user_id, 0), is_active,
		(SELECT COUNT(*) FROM vote WHERE vote.voting_event_id = voting_event.id) FROM voting_event`
	query += ` WHERE (event_type != 'Personal' AND event_class = 'Public' AND is_active = 1) OR owner_user_id = ? OR
		(event_type = 'Personal' AND is_active = 1 AND EXISTS (
			SELECT 1 FROM voting_event_invitee invitee WHERE invitee.event_id = voting_event.id AND invitee.user_id = ?))`
	args := []any{currentUser(r).ID, currentUser(r).ID}
	query += ` ORDER BY opens_at, id`
	rows, err := db.DB().Query(query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	result := make([]api.Event, 0)
	for rows.Next() {
		var event api.Event
		if err := rows.Scan(&event.ID, &event.Title, &event.Description, &event.EventType, &event.EventClass, &event.EventDate, &event.OpensAt, &event.ClosesAt, &event.OwnerID, &event.IsActive, &event.VoteCount); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if event.OwnerID == currentUser(r).ID {
			event.InviteeIDs, err = eventInviteeIDs(event.ID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		} else if event.EventType == "Personal" {
			event.InviteeIDs = []int64{currentUser(r).ID}
		}
		result = append(result, event)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listEventInvitees(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB().Query(`SELECT id, name, email, role FROM users WHERE role = ? AND id != ? ORDER BY name, id`, string(model.RoleMember), currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	result := make([]api.User, 0)
	for rows.Next() {
		var user api.User
		if err := rows.Scan(&user.ID, &user.Name, &user.Email, &user.Role); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		result = append(result, user)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) createEvent(w http.ResponseWriter, r *http.Request) {
	var event api.Event
	if err := decodeJSON(w, r, &event); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if event.EventClass == "" {
		event.EventClass = "Public"
	}
	if err := validateEvent(event); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !isAdmin(r) && event.EventType != "Personal" {
		writeError(w, http.StatusForbidden, "only Personal events can be created by non-administrators")
		return
	}
	if err := validateEventInvitees(event, currentUser(r).ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event.OwnerID = currentUser(r).ID
	event.IsActive = true
	result, err := db.DB().Exec(`INSERT INTO voting_event (title, description, event_type, event_class, event_date, opens_at, closes_at, owner_user_id, is_active) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`, strings.TrimSpace(event.Title), event.Description, event.EventType, event.EventClass, event.EventDate, event.OpensAt, event.ClosesAt, nullableEventOwner(event.OwnerID))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event.ID, err = result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := replaceEventInvitees(event.ID, event.InviteeIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, event)
}

func (s *Server) updateEvent(w http.ResponseWriter, r *http.Request) {
	eventID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var event api.Event
	if err := decodeJSON(w, r, &event); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	current, voteCount, err := getEventState(eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if !isAdmin(r) {
		if current.EventType != "Personal" || current.OwnerID != currentUser(r).ID {
			writeError(w, http.StatusForbidden, "event does not belong to this user")
			return
		}
		if event.EventType != "Personal" {
			writeError(w, http.StatusForbidden, "only administrators can change an event type")
			return
		}
		if voteCount > 0 || eventCurrentlyOpen(current.OpensAt, current.ClosesAt, time.Now()) {
			writeError(w, http.StatusForbidden, "only administrators can edit voted or currently open events")
			return
		}
		event.OwnerID = current.OwnerID
		event.IsActive = current.IsActive
	}
	if event.EventClass == "" {
		event.EventClass = current.EventClass
	}
	if err := validateEvent(event); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateEventInvitees(event, currentUser(r).ID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	_, err = db.DB().Exec(`UPDATE voting_event SET title = ?, description = ?, event_type = ?, event_class = ?, event_date = ?, opens_at = ?, closes_at = ? WHERE id = ?`, strings.TrimSpace(event.Title), event.Description, event.EventType, event.EventClass, event.EventDate, event.OpensAt, event.ClosesAt, eventID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := replaceEventInvitees(eventID, event.InviteeIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	event.ID = eventID
	event.OwnerID = current.OwnerID
	event.InviteeIDs = append([]int64(nil), event.InviteeIDs...)
	event.IsActive = current.IsActive
	event.VoteCount = voteCount
	writeJSON(w, http.StatusOK, event)
}

func (s *Server) deleteEvent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, voteCount, err := getEventState(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if !isAdmin(r) {
		if event.EventType != "Personal" || event.OwnerID != currentUser(r).ID {
			writeError(w, http.StatusForbidden, "event does not belong to this user")
			return
		}
		if !event.IsActive || voteCount > 0 || eventCurrentlyOpen(event.OpensAt, event.ClosesAt, time.Now()) {
			writeError(w, http.StatusForbidden, "only administrators can delete deactivated, voted, or currently open events")
			return
		}
	}
	if _, err := db.DB().Exec(`DELETE FROM voting_event WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deactivateEvent(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, _, err := getEventState(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if !isAdmin(r) && (event.EventType != "Personal" || event.OwnerID != currentUser(r).ID) {
		writeError(w, http.StatusForbidden, "event does not belong to this user")
		return
	}
	if _, err := db.DB().Exec(`UPDATE voting_event SET is_active = 0 WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) eventVoteSummary(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, _, err := getEventState(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if event.OwnerID != currentUser(r).ID {
		writeError(w, http.StatusForbidden, "only the event owner can view vote results")
		return
	}
	var summary api.VoteSummary
	err = db.DB().QueryRow(`SELECT
		COALESCE(SUM(choice = 'Yes'), 0),
		COALESCE(SUM(choice = 'No'), 0),
		COALESCE(SUM(choice = 'Abstain'), 0)
		FROM vote WHERE voting_event_id = ?`, id).Scan(&summary.Yes, &summary.No, &summary.Abstain)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) getVote(w http.ResponseWriter, r *http.Request) {
	eventID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, _, err := getEventState(eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if !eventIsVisibleTo(event, currentUser(r).ID) {
		writeError(w, http.StatusForbidden, "event is private")
		return
	}
	var vote api.Vote
	err = db.DB().QueryRow(`SELECT choice, COALESCE(comments, '') FROM vote WHERE voting_event_id = ? AND voter_user_id = ?`, eventID, currentUser(r).ID).Scan(&vote.Choice, &vote.Comments)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, vote)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vote)
}

func (s *Server) saveVote(w http.ResponseWriter, r *http.Request) {
	if isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrators cannot submit votes")
		return
	}
	eventID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	event, _, err := getEventState(eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "event not found")
		return
	}
	if !eventIsVisibleTo(event, currentUser(r).ID) {
		writeError(w, http.StatusForbidden, "event is private")
		return
	}
	var eventType, opensAt, closesAt string
	var active bool
	if err := db.DB().QueryRow(`SELECT event_type, opens_at, closes_at, is_active FROM voting_event WHERE id = ?`, eventID).Scan(&eventType, &opensAt, &closesAt, &active); err != nil {
		writeError(w, http.StatusNotFound, "voting event not found")
		return
	}
	if eventType != "Vote" && eventType != "Personal" {
		writeError(w, http.StatusBadRequest, "voting is available only for Vote or Personal events")
		return
	}
	if !active {
		writeError(w, http.StatusBadRequest, "event is deactivated")
		return
	}
	parsedEvent := api.Event{EventType: eventType, OpensAt: opensAt, ClosesAt: closesAt, IsActive: active}
	if err := votingOpen(parsedEvent, time.Now()); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var vote api.Vote
	if err := decodeJSON(w, r, &vote); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if vote.Choice != "Yes" && vote.Choice != "No" && vote.Choice != "Abstain" {
		writeError(w, http.StatusBadRequest, "choose Yes, No, or Abstain")
		return
	}
	if vote.Choice == "Abstain" && strings.TrimSpace(vote.Comments) == "" {
		writeError(w, http.StatusBadRequest, "comments are required when abstaining")
		return
	}
	_, err = db.DB().Exec(`INSERT INTO vote (voting_event_id, voter_user_id, choice, comments) VALUES (?, ?, ?, ?) ON CONFLICT (voting_event_id, voter_user_id) DO UPDATE SET choice = excluded.choice, comments = excluded.comments`, eventID, currentUser(r).ID, vote.Choice, strings.TrimSpace(vote.Comments))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) importDatabase(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxImportBytes)
	file, _, err := r.FormFile("database")
	if err != nil {
		writeError(w, http.StatusBadRequest, "a SQLite database file is required")
		return
	}
	defer file.Close()
	dir, err := os.MkdirTemp("", "mytodo-import-")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer os.RemoveAll(dir)
	backupPath := filepath.Join(dir, "upload.db")
	output, err := os.Create(backupPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := io.Copy(output, file); err != nil {
		output.Close()
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := output.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := db.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	databasePath := db.CurrentFilename()
	if err := db.RestoreDatabaseAt(backupPath, databasePath); err != nil {
		_ = db.OpenAtPath(databasePath)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := db.OpenAtPath(databasePath); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.sessionMu.Lock()
	s.sessions = make(map[string]session)
	s.sessionMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) backupDatabase(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	backup, err := os.CreateTemp("", "mytodo-backup-*.db")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	backupPath := backup.Name()
	defer os.Remove(backupPath)
	if err := backup.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := db.DB().Exec("VACUUM INTO ?", backupPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("create database backup: %v", err))
		return
	}
	backup, err = os.Open(backupPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer backup.Close()
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="mytodo.db"`)
	if _, err := io.Copy(w, backup); err != nil {
		return
	}
}

func isAdmin(r *http.Request) bool {
	user := currentUser(r)
	return user != nil && user.Role == model.RoleAdmin
}

func publicUser(user *model.User) api.User {
	result := api.User{ID: user.ID, Name: user.Name, DOB: user.DOB, Email: user.Email, Role: string(user.Role)}
	if user.ParentID.Valid {
		parentID := user.ParentID.Int64
		result.ParentID = &parentID
	}
	return result
}

func projectDTO(project model.Project) api.Project {
	return api.Project{ID: project.ID, OwnerID: project.OwnerID, Name: project.Name, Description: project.Description}
}

func taskDTO(task model.Task) api.Task {
	return api.Task{ID: task.ID, ParentID: task.ParentID, UserID: task.UserID, Name: task.Name, DueDate: task.DueDate, Done: task.Done}
}

func ownedProject(id, ownerID int64) (*model.Project, error) {
	project, err := model.GetProject(id)
	if err != nil || project.OwnerID != ownerID {
		return nil, fmt.Errorf("project not found")
	}
	return project, nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func nullableEventOwner(ownerID int64) any {
	if ownerID == 0 {
		return nil
	}
	return ownerID
}

func getEventState(id int64) (api.Event, int, error) {
	var event api.Event
	var voteCount int
	err := db.DB().QueryRow(`SELECT id, title, COALESCE(description, ''), event_type, event_class, opens_at, closes_at, COALESCE(owner_user_id, 0), is_active
		FROM voting_event WHERE id = ?`, id).Scan(&event.ID, &event.Title, &event.Description, &event.EventType, &event.EventClass, &event.OpensAt, &event.ClosesAt, &event.OwnerID, &event.IsActive)
	if err != nil {
		return api.Event{}, 0, err
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM vote WHERE voting_event_id = ?`, id).Scan(&voteCount); err != nil {
		return api.Event{}, 0, err
	}
	event.VoteCount = voteCount
	return event, voteCount, nil
}

func eventIsVisibleTo(event api.Event, userID int64) bool {
	if event.EventType == "Personal" {
		if event.OwnerID == userID {
			return true
		}
		var exists bool
		err := db.DB().QueryRow(`SELECT EXISTS(SELECT 1 FROM voting_event_invitee WHERE event_id = ? AND user_id = ?)`, event.ID, userID).Scan(&exists)
		return err == nil && exists
	}
	if event.EventClass == "Public" || event.OwnerID == userID {
		return true
	}
	return false
}

func eventInviteeIDs(eventID int64) ([]int64, error) {
	rows, err := db.DB().Query(`SELECT user_id FROM voting_event_invitee WHERE event_id = ? ORDER BY user_id`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func validateEventInvitees(event api.Event, ownerID int64) error {
	if event.EventType != "Personal" {
		if len(event.InviteeIDs) > 0 {
			return fmt.Errorf("only Personal events can have invitees")
		}
		return nil
	}
	if len(event.InviteeIDs) == 0 {
		return fmt.Errorf("Personal events require at least one Member invitee")
	}
	seen := make(map[int64]struct{}, len(event.InviteeIDs))
	for _, id := range event.InviteeIDs {
		if id <= 0 || id == ownerID {
			return fmt.Errorf("invitees must be other Member accounts")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("duplicate event invitee")
		}
		seen[id] = struct{}{}
		var role string
		if err := db.DB().QueryRow(`SELECT role FROM users WHERE id = ?`, id).Scan(&role); err != nil || role != string(model.RoleMember) {
			return fmt.Errorf("invitees must be Member accounts")
		}
	}
	return nil
}

func replaceEventInvitees(eventID int64, inviteeIDs []int64) error {
	if _, err := db.DB().Exec(`DELETE FROM voting_event_invitee WHERE event_id = ?`, eventID); err != nil {
		return err
	}
	for _, id := range inviteeIDs {
		if _, err := db.DB().Exec(`INSERT INTO voting_event_invitee (event_id, user_id) VALUES (?, ?)`, eventID, id); err != nil {
			return err
		}
	}
	return nil
}

func eventCurrentlyOpen(opensAt, closesAt string, now time.Time) bool {
	opens, openErr := time.ParseInLocation("2006-01-02 15:04", opensAt, time.Local)
	closes, closeErr := time.ParseInLocation("2006-01-02 15:04", closesAt, time.Local)
	return openErr == nil && closeErr == nil && !now.Before(opens) && now.Before(closes)
}

func validateEvent(event api.Event) error {
	if strings.TrimSpace(event.Title) == "" {
		return fmt.Errorf("event title is required")
	}
	if event.EventType != "Vote" && event.EventType != "Internal" && event.EventType != "External" && event.EventType != "Personal" {
		return fmt.Errorf("invalid event type")
	}
	if event.EventClass != "Public" && event.EventClass != "Private" {
		return fmt.Errorf("invalid event classification")
	}
	eventDate, err := time.ParseInLocation("2006-01-02", event.EventDate, time.Local)
	if err != nil {
		return fmt.Errorf("invalid event date")
	}
	opens, err := time.ParseInLocation("2006-01-02 15:04", event.OpensAt, time.Local)
	if err != nil {
		return fmt.Errorf("invalid opening time")
	}
	if !clockInRange(opens.Format("15:04"), "06:00", "22:00") {
		return fmt.Errorf("opening time must be between 06:00 and 22:00")
	}
	closes, err := time.ParseInLocation("2006-01-02 15:04", event.ClosesAt, time.Local)
	if err != nil {
		return fmt.Errorf("invalid closing time")
	}
	if !clockInRange(closes.Format("15:04"), "06:00", "22:00") {
		return fmt.Errorf("closing time must be between 06:00 and 22:00")
	}
	if !closes.After(opens) {
		return fmt.Errorf("closing time must be after opening time")
	}
	eventDeadline := time.Date(eventDate.Year(), eventDate.Month(), eventDate.Day(), 23, 59, 0, 0, time.Local)
	if !opens.Before(eventDeadline) || !closes.Before(eventDeadline) {
		return fmt.Errorf("opening and closing times must be before the event date at 23:59")
	}
	return nil
}

func votingOpen(event api.Event, now time.Time) error {
	if event.EventType != "Vote" || !event.IsActive {
		return fmt.Errorf("voting is available only for Vote events")
	}
	opens, openErr := time.ParseInLocation("2006-01-02 15:04", event.OpensAt, time.Local)
	closes, closeErr := time.ParseInLocation("2006-01-02 15:04", event.ClosesAt, time.Local)
	if openErr != nil || closeErr != nil || now.Before(opens) || !now.Before(closes) {
		return fmt.Errorf("voting is available only while the event is open")
	}
	return nil
}

func clockInRange(value, minimum, maximum string) bool {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return false
	}
	min, minErr := time.Parse("15:04", minimum)
	max, maxErr := time.Parse("15:04", maximum)
	return minErr == nil && maxErr == nil && !parsed.Before(min) && !parsed.After(max)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
