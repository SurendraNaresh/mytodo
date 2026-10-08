//go:build js

package db

import (
	"database/sql"
	"errors"
)

var errBrowserDatabase = errors.New("browser builds use the authenticated server API; local SQLite files are not available in WebAssembly")

const DBName = "mytodo.db"

func InitDB() (*sql.DB, error) {
	return nil, errBrowserDatabase
}

func InitDBAt(string) (*sql.DB, error) {
	return nil, errBrowserDatabase
}

func CopyStarterIfMissing(string, string) error {
	return errBrowserDatabase
}

func DataDir() (string, error) {
	return "", errBrowserDatabase
}

func DatabaseExists() (bool, error) {
	return true, nil
}

func RestoreDatabase(string) error {
	return errBrowserDatabase
}

func RestoreDatabaseAt(string, string) error {
	return errBrowserDatabase
}
