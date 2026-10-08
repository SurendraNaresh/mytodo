package db

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDataDirUsesWindowsLocalAppData(t *testing.T) {
	t.Setenv("MYTODO_DATA_DIR", "")
	if runtime.GOOS != "windows" {
		t.Skip("Windows data directory behavior")
	}

	base, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "mytodo")
	got, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
}

func TestDataDirHonorsOverride(t *testing.T) {
	configured := filepath.Join(t.TempDir(), "mytodo-data")
	t.Setenv("MYTODO_DATA_DIR", configured)

	got, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(configured)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
}

func TestCopyStarterIfMissing(t *testing.T) {
	dir := t.TempDir()
	starter := filepath.Join(dir, "starter.db")
	destination := filepath.Join(dir, "data", "mytodo.db")
	if err := os.WriteFile(starter, []byte("starter-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CopyStarterIfMissing(starter, destination); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != "starter-data" {
		t.Fatalf("copied starter = %q, %v", content, err)
	}
	if err := os.WriteFile(destination, []byte("existing-data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CopyStarterIfMissing(starter, destination); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(destination)
	if err != nil || string(content) != "existing-data" {
		t.Fatalf("existing database = %q, %v", content, err)
	}
}
