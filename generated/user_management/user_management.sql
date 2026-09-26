-- Generated from blueprint. Review before applying.
CREATE TABLE IF NOT EXISTS users_profile (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    mobile TEXT NOT NULL,
    UNIQUE (email)
);

CREATE TABLE IF NOT EXISTS userdetails (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
	users_profile_id INTEGER NOT NULL REFERENCES users_profile(id) ON DELETE CASCADE,
    phone1 INTEGER NOT NULL,
    phone2 TEXT
);
CREATE INDEX IF NOT EXISTS ix_userdetails_parent ON userdetails(users_profile_id);
