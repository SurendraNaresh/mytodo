package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const DBName = "mytodo.db"

// InitDB creates/opens the SQLite database in a writable
// application-specific directory.
//
// The location is:
//   Android/Linux/macOS/etc. -> user's application data directory
//   Windows                  -> %LOCALAPPDATA%\mytodo
//
// No root/admin privileges are required.
func InitDB() (*sql.DB, error) {
	dir, err := appDataDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine app data directory: %w", err)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create app data directory: %w", err)
	}

	dbPath := filepath.Join(dir, DBName)

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", dbPath, err)
	}

	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("cannot initialize sqlite: %w", err)
	}

	// Required for foreign-key relationships:
	// User -> User
	// Project -> User
	// Task -> Project
	// Task -> User
	if _, err := conn.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		conn.Close()
		return nil, fmt.Errorf("cannot enable foreign keys: %w", err)
	}

	return conn, nil
}

// appDataDir returns a directory writable by the application.
//
// MYTODO_DATA_DIR can optionally override the location. This is
// useful for testing, development and sandboxed environments.
func appDataDir() (string, error) {
	if dir := os.Getenv("MYTODO_DATA_DIR"); dir != "" {
		return filepath.Abs(dir)
	}

	// os.UserConfigDir() returns a user-writable application
	// configuration/data location without requiring root.
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(base, "mytodo"), nil
}
