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
