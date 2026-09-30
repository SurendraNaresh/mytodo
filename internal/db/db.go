package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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

	CREATE TABLE IF NOT EXISTS voting_event (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT,
		event_type TEXT NOT NULL DEFAULT 'Vote' CHECK (event_type IN ('Vote', 'Internal', 'External', 'Personal')),
		event_class TEXT NOT NULL DEFAULT 'Public',
		event_date TEXT NOT NULL DEFAULT '',
		opens_at TEXT NOT NULL,
		closes_at TEXT NOT NULL,
		owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
		is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
	);

	CREATE TABLE IF NOT EXISTS vote (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		voting_event_id INTEGER NOT NULL REFERENCES voting_event(id) ON DELETE CASCADE,
		voter_user_id INTEGER NOT NULL,
		choice TEXT NOT NULL,
		comments TEXT,
		UNIQUE (voting_event_id, voter_user_id)
	);

	CREATE INDEX IF NOT EXISTS ix_vote_parent ON vote(voting_event_id);

	-- ============================================================
	-- BLUEPRINT-DRIVEN OBJECTS
	-- ============================================================
	CREATE TABLE IF NOT EXISTS object_types (
		type_id        INTEGER PRIMARY KEY,
		type_key       TEXT NOT NULL UNIQUE,
		parent_type_id INTEGER REFERENCES object_types(type_id),
		version        INTEGER NOT NULL DEFAULT 1,
		created_at     TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS objects (
		obj_id       INTEGER PRIMARY KEY AUTOINCREMENT,
		type_id      INTEGER NOT NULL REFERENCES object_types(type_id),
		parent_obj_id INTEGER REFERENCES objects(obj_id) ON DELETE CASCADE,
		owner_user_id INTEGER NOT NULL REFERENCES users(id),
		data         TEXT NOT NULL DEFAULT '{}',
		version      INTEGER NOT NULL DEFAULT 1,
		created_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at   TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS hierarchy_permission (
		role TEXT NOT NULL,
		parent_type_id INTEGER NOT NULL REFERENCES object_types(type_id),
		child_type_id INTEGER NOT NULL REFERENCES object_types(type_id),
		can_create INTEGER NOT NULL DEFAULT 0 CHECK (can_create IN (0, 1)),
		PRIMARY KEY(role, parent_type_id, child_type_id)
	);

	CREATE INDEX IF NOT EXISTS ix_objects_parent ON objects(parent_obj_id);
	CREATE INDEX IF NOT EXISTS ix_objects_type ON objects(type_id);

	CREATE TRIGGER IF NOT EXISTS trg_objects_version
	AFTER UPDATE OF data, parent_obj_id, owner_user_id ON objects
	WHEN NEW.version = OLD.version
	BEGIN
		UPDATE objects
		SET version = OLD.version + 1, updated_at = CURRENT_TIMESTAMP
		WHERE obj_id = OLD.obj_id;
	END;
	`

	_, err := conn.Exec(schema)
	if err != nil {
		return fmt.Errorf("schema migration failed: %w", err)
	}
	if err := ensureColumn(conn, "voting_event", "event_type", "TEXT NOT NULL DEFAULT 'Vote'"); err != nil {
		return fmt.Errorf("add voting event type: %w", err)
	}
	if err := ensureColumn(conn, "voting_event", "event_class", "TEXT NOT NULL DEFAULT 'Public'"); err != nil {
		return fmt.Errorf("add event classification: %w", err)
	}
	if err := ensureColumn(conn, "voting_event", "event_date", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add event date: %w", err)
	}
	if _, err := conn.Exec(`UPDATE voting_event SET event_date = substr(closes_at, 1, 10) WHERE event_date = ''`); err != nil {
		return fmt.Errorf("backfill event date: %w", err)
	}
	if err := ensureColumn(conn, "voting_event", "owner_user_id", "INTEGER REFERENCES users(id) ON DELETE CASCADE"); err != nil {
		return fmt.Errorf("add event owner: %w", err)
	}
	if err := ensureColumn(conn, "voting_event", "is_active", "INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))"); err != nil {
		return fmt.Errorf("add event status: %w", err)
	}
	if err := migrateVotingEventType(conn); err != nil {
		return fmt.Errorf("update event type constraint: %w", err)
	}
	if err := ensureColumn(conn, "vote", "comments", "TEXT"); err != nil {
		return fmt.Errorf("add vote comments: %w", err)
	}

	return nil
}

func migrateVotingEventType(conn *sql.DB) error {
	var tableSQL string
	if err := conn.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'voting_event'`).Scan(&tableSQL); err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(tableSQL), "'personal'") {
		return nil
	}

	connection, err := conn.Conn(context.Background())
	if err != nil {
		return err
	}
	defer connection.Close()
	if _, err := connection.ExecContext(context.Background(), `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	defer connection.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`)

	tx, err := connection.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		`CREATE TABLE voting_event_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			description TEXT,
			event_type TEXT NOT NULL DEFAULT 'Vote' CHECK (event_type IN ('Vote', 'Internal', 'External', 'Personal')),
			event_class TEXT NOT NULL DEFAULT 'Public',
			event_date TEXT NOT NULL DEFAULT '',
			opens_at TEXT NOT NULL,
			closes_at TEXT NOT NULL,
			owner_user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
			is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))
		)`,
		`INSERT INTO voting_event_new (id, title, description, event_type, event_class, event_date, opens_at, closes_at, owner_user_id, is_active)
			SELECT id, title, description, event_type, event_class, event_date, opens_at, closes_at, owner_user_id, is_active FROM voting_event`,
		`DROP TABLE voting_event`,
		`ALTER TABLE voting_event_new RENAME TO voting_event`,
		`CREATE INDEX IF NOT EXISTS ix_vote_parent ON vote(voting_event_id)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ensureColumn(conn *sql.DB, table, column, definition string) error {
	rows, err := conn.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	_, err = conn.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
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

// OpenWithRestore restores backupPath before opening and migrating the database.
func OpenWithRestore(backupPath string) error {
	if backupPath != "" {
		if err := RestoreDatabase(backupPath); err != nil {
			return err
		}
	}
	return Open()
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
