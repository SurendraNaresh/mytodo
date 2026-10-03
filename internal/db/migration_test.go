package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenMigratesEraContentSchema(t *testing.T) {
	t.Setenv("MYTODO_DATA_DIR", filepath.Join(t.TempDir(), "data"))
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	if err := Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close() })

	for _, table := range []string{"era", "album", "media", "comment", "layout_frame", "artifacts"} {
		var name string
		if err := DB().QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Errorf("migrated table %q is missing: %v", table, err)
		}
	}
	var frameCount int
	if err := DB().QueryRow(`SELECT COUNT(*) FROM layout_frame`).Scan(&frameCount); err != nil || frameCount != 5 {
		t.Fatalf("default layout frame count = %d, %v; want 5", frameCount, err)
	}
	if _, err := DB().Exec(`DELETE FROM layout_frame WHERE region = 'top'`); err != nil {
		t.Fatal(err)
	}
	if _, err := DB().Exec(`INSERT INTO layout_frame (region, feature, visible, config_json) VALUES ('top', 'timeline', 1, '{}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := DB().Exec(`INSERT INTO layout_frame (region, feature, visible, config_json) VALUES ('top', 'donate_qr', 1, '{}')`); err == nil {
		t.Fatal("layout_frame accepted more than one row for a region")
	}
	if _, err := DB().Exec(`INSERT INTO layout_frame (region, feature, visible, config_json) VALUES ('invalid', 'timeline', 1, '{}')`); err == nil {
		t.Fatal("layout_frame accepted an unsupported region")
	}
}

func TestEnsureColumnAddsLegacyColumnAndIsIdempotent(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec(`CREATE TABLE voting_event (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}

	if err := ensureColumn(conn, "voting_event", "event_type", "TEXT NOT NULL DEFAULT 'Vote'"); err != nil {
		t.Fatal(err)
	}
	if err := ensureColumn(conn, "voting_event", "event_type", "TEXT NOT NULL DEFAULT 'Vote'"); err != nil {
		t.Fatalf("second migration call should be a no-op: %v", err)
	}

	if _, err := conn.Exec(`INSERT INTO voting_event (id) VALUES (1)`); err != nil {
		t.Fatal(err)
	}
	var eventType string
	if err := conn.QueryRow(`SELECT event_type FROM voting_event WHERE id = 1`).Scan(&eventType); err != nil {
		t.Fatal(err)
	}
	if eventType != "Vote" {
		t.Fatalf("migrated event type = %q, want Vote", eventType)
	}
}

func TestMigrateVotingEventTypePreservesVotesAndAllowsPersonalEvents(t *testing.T) {
	conn, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec(`
		CREATE TABLE users (id INTEGER PRIMARY KEY);
		CREATE TABLE voting_event (
			id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			description TEXT,
			event_type TEXT NOT NULL DEFAULT 'Vote' CHECK (event_type IN ('Vote', 'Internal', 'External')),
			opens_at TEXT NOT NULL,
			closes_at TEXT NOT NULL
		);
		CREATE TABLE vote (
			id INTEGER PRIMARY KEY,
			voting_event_id INTEGER NOT NULL REFERENCES voting_event(id) ON DELETE CASCADE,
			voter_user_id INTEGER NOT NULL,
			choice TEXT NOT NULL
		);
		INSERT INTO users (id) VALUES (7);
		INSERT INTO voting_event (id, title, event_type, opens_at, closes_at) VALUES (3, 'Legacy', 'Vote', '2026-01-01 09:00', '2026-01-01 10:00');
		INSERT INTO vote (id, voting_event_id, voter_user_id, choice) VALUES (11, 3, 7, 'Yes');`); err != nil {
		t.Fatal(err)
	}
	if err := ensureColumn(conn, "voting_event", "owner_user_id", "INTEGER REFERENCES users(id) ON DELETE CASCADE"); err != nil {
		t.Fatal(err)
	}
	if err := ensureColumn(conn, "voting_event", "event_class", "TEXT NOT NULL DEFAULT 'Public'"); err != nil {
		t.Fatal(err)
	}
	if err := ensureColumn(conn, "voting_event", "event_date", "TEXT NOT NULL DEFAULT ''"); err != nil {
		t.Fatal(err)
	}
	if err := ensureColumn(conn, "voting_event", "is_active", "INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1))"); err != nil {
		t.Fatal(err)
	}
	if err := migrateVotingEventType(conn); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`INSERT INTO voting_event (title, event_type, opens_at, closes_at, owner_user_id) VALUES ('Personal', 'Personal', '2026-01-01 09:00', '2026-01-01 10:00', 7)`); err != nil {
		t.Fatalf("insert Personal event after migration: %v", err)
	}
	var voteCount, ownerID int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM vote WHERE voting_event_id = 3`).Scan(&voteCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT owner_user_id FROM voting_event WHERE event_type = 'Personal'`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if voteCount != 1 || ownerID != 7 {
		t.Fatalf("migration state: preserved votes=%d, owner=%d; want 1 and 7", voteCount, ownerID)
	}
}
