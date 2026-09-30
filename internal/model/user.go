package model

import (
	"database/sql"
	"errors"
	"strings"
	"sync/atomic"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/db"
	"golang.org/x/crypto/bcrypt"
)

type Role string

const (
	RoleAdmin   Role = "Admin"
	RoleManager Role = "Manager"
	RoleMember  Role = "Member"
	RoleStaff   Role = "Staff"
	RoleVisitor Role = "Visitor"
)

func Roles() []Role {
	return []Role{
		RoleAdmin,
		RoleManager,
		RoleMember,
		RoleStaff,
		RoleVisitor,
	}
}

type User struct {
	ID       int64
	ParentID sql.NullInt64

	Name  string
	DOB   string
	Email string
	Role  Role

	PasswordHash string
}

var remoteAPIEnabled atomic.Bool

func UseRemoteAPI(enabled bool) {
	remoteAPIEnabled.Store(enabled)
}

func (u *User) VerifyPassword(password string) bool {
	return bcrypt.CompareHashAndPassword(
		[]byte(u.PasswordHash),
		[]byte(password),
	) == nil
}

func CreateUser(
	name string,
	dob string,
	email string,
	role Role,
	password string,
	parentID sql.NullInt64,
) (*User, error) {

	name = strings.TrimSpace(name)
	email = strings.TrimSpace(strings.ToLower(email))

	if name == "" {
		return nil, errors.New("name is required")
	}

	if email == "" {
		return nil, errors.New("email is required")
	}

	if password == "" {
		return nil, errors.New("password is required")
	}
	if remoteAPIEnabled.Load() {
		client, err := api.Default()
		if err != nil {
			return nil, err
		}
		remoteUser, err := client.SaveUser(api.User{
			Name: name, DOB: dob, Email: email, Role: string(role), ParentID: nullableIDPointer(parentID),
		}, password)
		if err != nil {
			return nil, err
		}
		return UserFromAPI(remoteUser), nil
	}

	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, err
	}

	result, err := db.DB().Exec(`
		INSERT INTO users
		    (parent_id, name, dob, email, role, password_hash)
		VALUES (?, ?, ?, ?, ?, ?)
	`,
		nullableID(parentID),
		name,
		dob,
		email,
		string(role),
		string(hash),
	)

	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetUser(id)
}

func UpdateUser(u *User, newPassword string) error {
	if u == nil {
		return errors.New("nil user")
	}

	u.Name = strings.TrimSpace(u.Name)
	u.Email = strings.TrimSpace(strings.ToLower(u.Email))

	if u.Name == "" {
		return errors.New("name is required")
	}

	if u.Email == "" {
		return errors.New("email is required")
	}
	if remoteAPIEnabled.Load() {
		client, err := api.Default()
		if err != nil {
			return err
		}
		_, err = client.SaveUser(api.User{
			ID: u.ID, Name: u.Name, DOB: u.DOB, Email: u.Email, Role: string(u.Role), ParentID: nullableIDPointer(u.ParentID),
		}, newPassword)
		return err
	}

	if newPassword != "" {
		hash, err := bcrypt.GenerateFromPassword(
			[]byte(newPassword),
			bcrypt.DefaultCost,
		)
		if err != nil {
			return err
		}

		u.PasswordHash = string(hash)
	}

	_, err := db.DB().Exec(`
		UPDATE users
		   SET parent_id     = ?,
		       name          = ?,
		       dob           = ?,
		       email         = ?,
		       role          = ?,
		       password_hash = ?
		 WHERE id = ?
	`,
		nullableID(u.ParentID),
		u.Name,
		u.DOB,
		u.Email,
		string(u.Role),
		u.PasswordHash,
		u.ID,
	)

	return err
}

func DeleteUser(id int64) error {
	if remoteAPIEnabled.Load() {
		client, err := api.Default()
		if err != nil {
			return err
		}
		return client.DeleteUser(id)
	}
	_, err := db.DB().Exec(`
		DELETE FROM users
		WHERE id = ?
	`, id)

	return err
}

func GetUser(id int64) (*User, error) {
	if remoteAPIEnabled.Load() {
		client, err := api.Default()
		if err != nil {
			return nil, err
		}
		remoteUser, err := client.User(id)
		if err != nil {
			return nil, err
		}
		return UserFromAPI(remoteUser), nil
	}
	row := db.DB().QueryRow(`
		SELECT
		    id,
		    parent_id,
		    name,
		    dob,
		    email,
		    role,
		    password_hash
		  FROM users
		 WHERE id = ?
	`, id)

	return scanUser(row)
}

func GetUserByEmail(email string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))

	row := db.DB().QueryRow(`
		SELECT
		    id,
		    parent_id,
		    name,
		    dob,
		    email,
		    role,
		    password_hash
		  FROM users
		 WHERE lower(email) = lower(?)
	`, email)

	u, err := scanUser(row)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	return u, err
}

func GetUsers() ([]User, error) {
	if remoteAPIEnabled.Load() {
		client, err := api.Default()
		if err != nil {
			return nil, err
		}
		remoteUsers, err := client.ListUsers()
		if err != nil {
			return nil, err
		}
		users := make([]User, 0, len(remoteUsers))
		for _, user := range remoteUsers {
			users = append(users, *UserFromAPI(user))
		}
		return users, nil
	}
	rows, err := db.DB().Query(`
		SELECT
		    id,
		    parent_id,
		    name,
		    dob,
		    email,
		    role,
		    password_hash
		  FROM users
		 ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User

	for rows.Next() {
		var u User
		var role string

		if err := rows.Scan(
			&u.ID,
			&u.ParentID,
			&u.Name,
			&u.DOB,
			&u.Email,
			&role,
			&u.PasswordHash,
		); err != nil {
			return nil, err
		}

		u.Role = Role(role)
		users = append(users, u)
	}

	return users, rows.Err()
}

func UserFromAPI(remoteUser api.User) *User {
	user := &User{ID: remoteUser.ID, Name: remoteUser.Name, DOB: remoteUser.DOB, Email: remoteUser.Email, Role: Role(remoteUser.Role)}
	if remoteUser.ParentID != nil {
		user.ParentID = sql.NullInt64{Int64: *remoteUser.ParentID, Valid: true}
	}
	return user
}

func nullableIDPointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	var role string

	err := row.Scan(
		&u.ID,
		&u.ParentID,
		&u.Name,
		&u.DOB,
		&u.Email,
		&role,
		&u.PasswordHash,
	)

	if err != nil {
		return nil, err
	}

	u.Role = Role(role)

	return &u, nil
}

func nullableID(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}
