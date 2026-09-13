package db

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const DBName = "mytodo.db"

// InitDB creates/opens the SQLite database in a writable
// application-specific directory.
//
// The location is:
//
//	Android/Linux/macOS/etc. -> user's application data directory
//	Windows                  -> %LOCALAPPDATA%\mytodo
//
// No root/admin privileges are required.
func InitDB() (*sql.DB, error) {
	dir, err := DataDir()
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
	if _, err := conn.Exec(`PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000`); err != nil {
		conn.Close()
		return nil, fmt.Errorf("cannot enable foreign keys: %w", err)
	}

	return conn, nil
}

// appDataDir returns a directory writable by the application.
//
// MYTODO_DATA_DIR can optionally override the location. This is
// useful for testing, development and sandboxed environments.
func DataDir() (string, error) {
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

func DatabaseExists() (bool, error) {
	dir, err := DataDir()
	if err != nil {
		return false, err
	}

	_, err = os.Stat(filepath.Join(dir, DBName))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// RestoreDatabase replaces the application database with a verified backup.
// It must be called before Open so migrations run against the restored data.
func RestoreDatabase(backupPath string) error {
	backupPath, err := filepath.Abs(backupPath)
	if err != nil {
		return fmt.Errorf("invalid backup path: %w", err)
	}

	backup, err := sql.Open("sqlite", backupPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("open backup: %w", err)
	}
	defer backup.Close()

	var result string
	if err := backup.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return fmt.Errorf("check backup integrity: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("backup integrity check failed: %s", result)
	}
	if err := backup.Close(); err != nil {
		return fmt.Errorf("close backup database: %w", err)
	}

	dir, err := DataDir()
	if err != nil {
		return fmt.Errorf("cannot determine app data directory: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("cannot create app data directory: %w", err)
	}

	destination := filepath.Join(dir, DBName)
	temp, err := os.CreateTemp(dir, DBName+".restore-*")
	if err != nil {
		return fmt.Errorf("create restore file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	source, err := os.Open(backupPath)
	if err != nil {
		temp.Close()
		return fmt.Errorf("read backup: %w", err)
	}
	if _, err := io.Copy(temp, source); err != nil {
		source.Close()
		temp.Close()
		return fmt.Errorf("copy backup: %w", err)
	}
	if err := source.Close(); err != nil {
		temp.Close()
		return fmt.Errorf("close backup: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close restore file: %w", err)
	}
	previousPath := destination + ".previous"
	_ = os.Remove(previousPath)
	if _, err := os.Stat(destination); err == nil {
		if err := os.Rename(destination, previousPath); err != nil {
			return fmt.Errorf("move existing database: %w", err)
		}
	}
	_ = os.Remove(destination + "-wal")
	_ = os.Remove(destination + "-shm")
	if err := os.Rename(tempPath, destination); err != nil {
		_ = os.Rename(previousPath, destination)
		return fmt.Errorf("install restored database: %w", err)
	}
	_ = os.Remove(previousPath)
	return nil
}
