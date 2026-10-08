package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port          string
	DBFilename    string
	LocalDataFile string
	APIURL        string
	CORSOrigins   string
}

func Load() (Config, error) {
	return LoadFrom(".config")
}

func LoadFrom(filename string) (Config, error) {
	values, err := readFile(filename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if values == nil {
		values = make(map[string]string)
	}
	for key, value := range map[string]string{
		"PORT":            os.Getenv("PORT"),
		"DB_FILENAME":     os.Getenv("DB_FILENAME"),
		"LOCAL_DATA_FILE": os.Getenv("LOCAL_DATA_FILE"),
		"API_URL":         os.Getenv("API_URL"),
		"CORS_ORIGINS":    os.Getenv("CORS_ORIGINS"),
	} {
		if value != "" {
			values[key] = value
		}
	}
	config := Config{
		Port:          valueOr(values, "PORT", "9876"),
		DBFilename:    valueOr(values, "DB_FILENAME", "./data/mytodo.db"),
		LocalDataFile: valueOr(values, "LOCAL_DATA_FILE", "local_data.db"),
		APIURL:        valueOr(values, "API_URL", "http://127.0.0.1:9876/api/v1"),
		CORSOrigins:   values["CORS_ORIGINS"],
	}
	return config, nil
}

func readFile(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid config entry at line %d", lineNumber)
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		value = strings.Trim(strings.TrimSpace(value), "\"'")
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func valueOr(values map[string]string, key, fallback string) string {
	if value := strings.TrimSpace(values[key]); value != "" {
		return value
	}
	return fallback
}
