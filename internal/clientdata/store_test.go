//go:build !js

package clientdata

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/SurendraNaresh/mytodo/internal/api"
	"github.com/SurendraNaresh/mytodo/internal/model"
)

func TestClientDataPersistsProfileSettingsThemeAndEvents(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "local_data.db")
	if err := Open(filename); err != nil {
		t.Fatal(err)
	}
	if err := SaveUser(&model.User{ID: 5, Name: "Member", Email: "member@example.test", Role: model.RoleMember}); err != nil {
		t.Fatal(err)
	}
	if err := SaveSetting("api_url", "https://todo.example.test/api/v1"); err != nil {
		t.Fatal(err)
	}
	if err := SaveTheme("app", `{"variant":"light"}`); err != nil {
		t.Fatal(err)
	}
	if err := SaveEvents([]api.Event{{ID: 8, Title: "Cached event"}}); err != nil {
		t.Fatal(err)
	}
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if err := Open(filename); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close() })
	apiURL, err := LoadSetting("api_url")
	if err != nil || apiURL != "https://todo.example.test/api/v1" {
		t.Fatalf("LoadSetting(api_url) = %q, %v", apiURL, err)
	}
	for _, check := range []struct {
		query string
		want  string
	}{
		{`SELECT email FROM client_user WHERE id = 5`, "member@example.test"},
		{`SELECT value FROM client_setting WHERE key = 'api_url'`, "https://todo.example.test/api/v1"},
		{`SELECT value FROM client_theme WHERE key = 'app'`, `{"variant":"light"}`},
	} {
		var got string
		if err := conn.QueryRow(check.query).Scan(&got); err != nil {
			t.Fatalf("query %q: %v", check.query, err)
		}
		if got != check.want {
			t.Errorf("query %q = %q, want %q", check.query, got, check.want)
		}
	}
	var payload string
	if err := conn.QueryRow(`SELECT payload FROM cached_event WHERE id = 8`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var event api.Event
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatal(err)
	}
	if event.ID != 8 || event.Title != "Cached event" {
		t.Fatalf("cached event = %#v", event)
	}
}

func TestResolvePathUsesAppStorageForRelativeFilename(t *testing.T) {
	root := t.TempDir()
	got, err := ResolvePath("local_data.db", root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "local_data.db")
	if got != want {
		t.Fatalf("ResolvePath() = %q, want %q", got, want)
	}
}
