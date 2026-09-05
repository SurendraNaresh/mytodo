package auth

import (
	"errors"

	"github.com/SurendraNaresh/mytodo/internal/model"
)

type Session struct {
	User *model.User
}

func NewSession() *Session {
	return &Session{}
}

func (s *Session) Login(
	email string,
	password string,
) error {

	u, err := model.GetUserByEmail(email)
	if err != nil {
		return err
	}

	if u == nil {
		return errors.New("invalid email or password")
	}

	if !u.VerifyPassword(password) {
		return errors.New("invalid email or password")
	}

	s.User = u
	return nil
}

func (s *Session) Logout() {
	s.User = nil
}

func (s *Session) IsAuthenticated() bool {
	return s != nil && s.User != nil && s.User.ID > 0
}

func (s *Session) Validate() error {
	if !s.IsAuthenticated() {
		return errors.New("no active session")
	}

	// Re-read user so deleted/changed users do not retain stale sessions.
	u, err := model.GetUser(s.User.ID)
	if err != nil {
		s.Logout()
		return errors.New("session no longer valid")
	}

	s.User = u
	return nil
}

func (s *Session) IsAdmin() bool {
	return s.IsAuthenticated() &&
		s.User.Role == model.RoleAdmin
}

func (s *Session) HasRole(roles ...model.Role) bool {
	if !s.IsAuthenticated() {
		return false
	}

	for _, role := range roles {
		if s.User.Role == role {
			return true
		}
	}

	return false
}
