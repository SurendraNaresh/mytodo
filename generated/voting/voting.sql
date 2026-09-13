-- Generated from blueprint. Review before applying.
CREATE TABLE IF NOT EXISTS voting_event (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL,
    description TEXT,
    opens_at TEXT NOT NULL,
    closes_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS vote (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
	voting_event_id INTEGER NOT NULL REFERENCES voting_event(id) ON DELETE CASCADE,
    voter_user_id INTEGER NOT NULL,
    choice TEXT NOT NULL,
    UNIQUE (voting_event_id, voter_user_id)
);
CREATE INDEX IF NOT EXISTS ix_vote_parent ON vote(voting_event_id);
