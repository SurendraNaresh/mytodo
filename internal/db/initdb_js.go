//go:build js

package db

import (
	"database/sql"
	"errors"
)

var errBrowserDatabase = errors.New("browser builds use the authenticated server API; local SQLite files are not available in WebAssembly")

func InitDB() (*sql.DB, error) {
	return nil, errBrowserDatabase
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
