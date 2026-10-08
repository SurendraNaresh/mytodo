//go:build !js

package clientdata

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/model"
	_ "modernc.org/sqlite"
)

var (
	mu   sync.RWMutex
	conn *sql.DB
)

func ResolvePath(filename, storageRoot string) (string, error) {
	if filepath.IsAbs(filename) {
		return filepath.Clean(filename), nil
	}
	if storageRoot == "" {
		return "", fmt.Errorf("application storage directory is unavailable")
	}
	return filepath.Abs(filepath.Join(storageRoot, filename))
}

func Open(filename string) error {
	mu.Lock()
	defer mu.Unlock()
	if conn != nil {
		return fmt.Errorf("client data store is already open")
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
		return fmt.Errorf("create client data directory: %w", err)
	}
	opened, err := sql.Open("sqlite", filename)
	if err != nil {
		return fmt.Errorf("open client data database: %w", err)
	}
	if _, err := opened.Exec(`
		PRAGMA foreign_keys = ON;
		PRAGMA journal_mode = WAL;
		CREATE TABLE IF NOT EXISTS client_user (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT NOT NULL,
			role TEXT NOT NULL,
			parent_id INTEGER,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		CREATE TABLE IF NOT EXISTS client_setting (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS client_theme (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS cached_event (
			id INTEGER PRIMARY KEY,
			payload TEXT NOT NULL
		);`); err != nil {
		opened.Close()
		return fmt.Errorf("initialize client data database: %w", err)
	}
	if err := opened.Ping(); err != nil {
		opened.Close()
		return err
	}
	conn = opened
	return nil
}

func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if conn == nil {
		return nil
	}
	err := conn.Close()
	conn = nil
	return err
}

func SaveUser(user *model.User) error {
	if user == nil || user.ID <= 0 {
		return fmt.Errorf("invalid local user")
	}
	var parentID any
	if user.ParentID.Valid {
		parentID = user.ParentID.Int64
	}
	store, err := database()
	if err != nil {
		return err
	}
	_, err = store.Exec(`INSERT INTO client_user (id, name, email, role, parent_id, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP) ON CONFLICT(id) DO UPDATE SET name = excluded.name, email = excluded.email, role = excluded.role, parent_id = excluded.parent_id, updated_at = CURRENT_TIMESTAMP`, user.ID, user.Name, user.Email, string(user.Role), parentID)
	return err
}

func SaveSetting(key, value string) error {
	store, err := database()
	if err != nil {
		return err
	}
	_, err = store.Exec(`INSERT INTO client_setting (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func LoadSetting(key string) (string, error) {
	store, err := database()
	if err != nil {
		return "", err
	}
	var value string
	err = store.QueryRow(`SELECT value FROM client_setting WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func SaveTheme(key, value string) error {
	store, err := database()
	if err != nil {
		return err
	}
	_, err = store.Exec(`INSERT INTO client_theme (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func SaveEvents(events []api.Event) error {
	store, err := database()
	if err != nil {
		return err
	}
	tx, err := store.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM cached_event`); err != nil {
		return err
	}
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO cached_event (id, payload) VALUES (?, ?)`, event.ID, string(payload)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func database() (*sql.DB, error) {
	mu.RLock()
	defer mu.RUnlock()
	if conn == nil {
		return nil, fmt.Errorf("client data store is not open")
	}
	return conn, nil
}
