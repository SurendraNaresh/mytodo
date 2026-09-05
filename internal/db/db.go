package db

import (
	"database/sql"
	"errors"
	"strings"
	"fmt"				 
	  
//	"github.com/SurendraNaresh/mytodo/internal/db"
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

var conn *sql.DB

func migrate() error {
	schema := `
	PRAGMA foreign_keys = ON;

	-- ============================================================
	-- USERS
	-- Self-referencing hierarchy:
	-- parent_id -> users.id
	-- ============================================================
	CREATE TABLE IF NOT EXISTS users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		parent_id     INTEGER NULL,

		name          TEXT NOT NULL,
		dob           TEXT,
		email         TEXT NOT NULL UNIQUE,

		role          TEXT NOT NULL DEFAULT 'Member'
		              CHECK (
		                  role IN (
		                      'Admin',
		                      'Manager',
		                      'Member',
		                      'Staff',
		                      'Visitor'
		                  )
		              ),

		password_hash TEXT NOT NULL,

		created_at    TEXT NOT NULL
		              DEFAULT CURRENT_TIMESTAMP,

		FOREIGN KEY (parent_id)
			REFERENCES users(id)
			ON DELETE SET NULL
	);

	-- ============================================================
	-- PROJECTS
	-- Parent/Master table
	-- ============================================================
	CREATE TABLE IF NOT EXISTS projects (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,

		owner_id    INTEGER NOT NULL,

		name        TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',

		created_at  TEXT NOT NULL
		            DEFAULT CURRENT_TIMESTAMP,

		FOREIGN KEY (owner_id)
			REFERENCES users(id)
			ON DELETE CASCADE
	);

	-- ============================================================
	-- TASKS
	-- Child/Transaction table
	--
	-- parent_id -> projects.id
	-- user_id   -> users.id
	-- ============================================================
	CREATE TABLE IF NOT EXISTS tasks (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,

		parent_id   INTEGER NOT NULL,
		user_id     INTEGER NOT NULL,

		name        TEXT NOT NULL,
		due_date    TEXT,

		done        INTEGER NOT NULL DEFAULT 0
		            CHECK (done IN (0, 1)),

		created_at  TEXT NOT NULL
		            DEFAULT CURRENT_TIMESTAMP,

		FOREIGN KEY (parent_id)
			REFERENCES projects(id)
			ON DELETE CASCADE,

		FOREIGN KEY (user_id)
			REFERENCES users(id)
			ON DELETE CASCADE
	);

	-- ============================================================
	-- INDEXES
	-- ============================================================

	CREATE INDEX IF NOT EXISTS ix_users_parent
		ON users(parent_id);

	CREATE INDEX IF NOT EXISTS ix_users_role
		ON users(role);

	CREATE INDEX IF NOT EXISTS ix_projects_owner
		ON projects(owner_id);

	CREATE INDEX IF NOT EXISTS ix_tasks_parent
		ON tasks(parent_id);

	CREATE INDEX IF NOT EXISTS ix_tasks_user
		ON tasks(user_id);

	CREATE INDEX IF NOT EXISTS ix_tasks_due_date
		ON tasks(due_date);
	`

	_, err := conn.Exec(schema)
	if err != nil {
		return fmt.Errorf("schema migration failed: %w", err)
	}

	return nil
}

func Open() error {
	var err error

	conn, err = InitDB()
	if err != nil {
		return err
	}

	if err := migrate(); err != nil {
		conn.Close()
		conn = nil
		return fmt.Errorf("database migration failed: %w", err)
	}

	return nil
}

func DB() *sql.DB {
	if conn == nil {
		panic("database has not been opened")
	}

	return conn
}

func Close() error {
	if conn == nil {
		return nil
	}

	err := conn.Close()
	conn = nil
	return err
}

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

	hash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, err
	}

	result, err := DB().Exec(`
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

	_, err := DB().Exec(`
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
	_, err := DB().Exec(`
		DELETE FROM users
		WHERE id = ?
	`, id)

	return err
}

func GetUser(id int64) (*User, error) {
	row := DB().QueryRow(`
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

	row := DB().QueryRow(`
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
	rows, err := DB().Query(`
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
