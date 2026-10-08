package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromReadsDefaultsAndCaseInsensitiveKeys(t *testing.T) {
	for _, key := range []string{"PORT", "DB_FILENAME", "LOCAL_DATA_FILE", "API_URL", "CORS_ORIGINS"} {
		t.Setenv(key, "")
	}
	filename := filepath.Join(t.TempDir(), ".config")
	if err := os.WriteFile(filename, []byte("PORT=9876\nDB_FILENAME=./data/custom.db\nLocal_Data_file=local_data.db\n"), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := LoadFrom(filename)
	if err != nil {
		t.Fatal(err)
	}
	if config.Port != "9876" || config.DBFilename != "./data/custom.db" || config.LocalDataFile != "local_data.db" {
		t.Fatalf("loaded config = %#v", config)
	}
}

func TestLoadFromUsesDefaultsAndEnvironmentOverrides(t *testing.T) {
	t.Setenv("PORT", "9999")
	t.Setenv("DB_FILENAME", "")
	t.Setenv("LOCAL_DATA_FILE", "")
	t.Setenv("API_URL", "")

	config, err := LoadFrom(filepath.Join(t.TempDir(), "missing.config"))
	if err != nil {
		t.Fatal(err)
	}
	if config.Port != "9999" || config.DBFilename != "./data/mytodo.db" || config.LocalDataFile != "local_data.db" || config.APIURL != "http://127.0.0.1:9876/api/v1" {
		t.Fatalf("loaded config = %#v", config)
	}
}
