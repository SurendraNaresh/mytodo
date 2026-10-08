package server

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/db"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	user := currentUser(r)
	if !user.VerifyPassword(input.CurrentPassword) {
		writeError(w, http.StatusUnauthorized, "current password is incorrect")
		return
	}
	if err := validateNewPassword(input.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash new password")
		return
	}
	if _, err := db.DB().Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update password")
		return
	}
	user.PasswordHash = string(hash)
	currentToken := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	currentSessionID := s.sessionID(currentToken)
	s.sessionMu.Lock()
	for sessionID, activeSession := range s.sessions {
		if activeSession.userID == user.ID && sessionID != currentSessionID {
			delete(s.sessions, sessionID)
		}
	}
	s.sessionMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createPasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TimeframeStart string `json:"timeframe_start"`
		TimeframeEnd   string `json:"timeframe_end"`
		Reason         string `json:"reason"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	start, startErr := time.Parse(time.RFC3339, input.TimeframeStart)
	end, endErr := time.Parse(time.RFC3339, input.TimeframeEnd)
	if startErr != nil || endErr != nil || !end.After(start) || len(input.Reason) > 1000 {
		writeError(w, http.StatusBadRequest, "provide a valid timeframe and a reason of at most 1000 characters")
		return
	}
	result, err := db.DB().Exec(`INSERT INTO password_reset_request (user_id, timeframe_start, timeframe_end, reason) VALUES (?, ?, ?, ?)`, currentUser(r).ID, start.Format(time.RFC3339), end.Format(time.RFC3339), strings.TrimSpace(input.Reason))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create password reset request")
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read password reset request")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

func (s *Server) getPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	policy, err := readPasswordPolicy()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read password policy")
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) updatePasswordPolicy(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	var policy api.PasswordPolicy
	if err := decodeJSON(w, r, &policy); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if policy.MinimumLength < 1 || policy.MinimumLength > 72 {
		writeError(w, http.StatusBadRequest, "minimum length must be between 1 and 72")
		return
	}
	if _, err := db.DB().Exec(`UPDATE password_policy SET minimum_length = ?, require_special = ?, require_mixed_case = ? WHERE id = 1`, policy.MinimumLength, policy.RequireSpecial, policy.RequireMixedCase); err != nil {
		writeError(w, http.StatusInternalServerError, "could not update password policy")
		return
	}
	writeJSON(w, http.StatusOK, policy)
}

func (s *Server) listPasswordResetRequests(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	rows, err := db.DB().Query(`SELECT request.id, request.user_id, user.name, user.email, request.timeframe_start, request.timeframe_end, request.reason, request.status, request.created_at FROM password_reset_request AS request JOIN users AS user ON user.id = request.user_id ORDER BY CASE request.status WHEN 'Pending' THEN 0 ELSE 1 END, request.id DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list password reset requests")
		return
	}
	defer rows.Close()
	requests := make([]api.PasswordResetRequest, 0)
	for rows.Next() {
		var request api.PasswordResetRequest
		if err := rows.Scan(&request.ID, &request.UserID, &request.UserName, &request.UserEmail, &request.TimeframeStart, &request.TimeframeEnd, &request.Reason, &request.Status, &request.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "could not read password reset requests")
			return
		}
		requests = append(requests, request)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not read password reset requests")
		return
	}
	writeJSON(w, http.StatusOK, requests)
}

func (s *Server) resetRequestedPassword(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		writeError(w, http.StatusForbidden, "administrator access required")
		return
	}
	requestID, err := pathID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var input struct {
		DefaultPassword string `json:"default_password"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateNewPassword(input.DefaultPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.DefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash default password")
		return
	}
	tx, err := db.DB().Begin()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not begin password reset")
		return
	}
	defer tx.Rollback()
	var userID int64
	if err := tx.QueryRow(`SELECT user_id FROM password_reset_request WHERE id = ? AND status = 'Pending'`, requestID).Scan(&userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "pending password reset request not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not read password reset request")
		return
	}
	if _, err := tx.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, string(hash), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not reset user password")
		return
	}
	if _, err := tx.Exec(`UPDATE password_reset_request SET status = 'Reset', resolved_at = CURRENT_TIMESTAMP, resolved_by = ? WHERE id = ?`, currentUser(r).ID, requestID); err != nil {
		writeError(w, http.StatusInternalServerError, "could not mark password reset request complete")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "could not complete password reset")
		return
	}
	s.sessionMu.Lock()
	for tokenID, session := range s.sessions {
		if session.userID == userID {
			delete(s.sessions, tokenID)
		}
	}
	s.sessionMu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func readPasswordPolicy() (api.PasswordPolicy, error) {
	var policy api.PasswordPolicy
	var requireSpecial, requireMixedCase int
	err := db.DB().QueryRow(`SELECT minimum_length, require_special, require_mixed_case FROM password_policy WHERE id = 1`).Scan(&policy.MinimumLength, &requireSpecial, &requireMixedCase)
	policy.RequireSpecial = requireSpecial != 0
	policy.RequireMixedCase = requireMixedCase != 0
	return policy, err
}

func validateNewPassword(password string) error {
	policy, err := readPasswordPolicy()
	if err != nil {
		return fmt.Errorf("could not read password policy")
	}
	if utf8.RuneCountInString(password) < policy.MinimumLength {
		return fmt.Errorf("password must be at least %d characters", policy.MinimumLength)
	}
	if len(password) > 72 {
		return errors.New("password must be at most 72 bytes")
	}
	var hasLower, hasUpper, hasSpecial bool
	for _, character := range password {
		hasLower = hasLower || unicode.IsLower(character)
		hasUpper = hasUpper || unicode.IsUpper(character)
		hasSpecial = hasSpecial || strings.ContainsRune("%$#@!^&~`", character)
	}
	if policy.RequireSpecial && !hasSpecial {
		return errors.New("password must contain at least one of %$#@!^&~`")
	}
	if policy.RequireMixedCase && (!hasLower || !hasUpper) {
		return errors.New("password must contain both uppercase and lowercase letters")
	}
	return nil
}
