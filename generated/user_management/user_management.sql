-- Generated from blueprint. Review before applying.
CREATE TABLE IF NOT EXISTS user (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL,
    email TEXT NOT NULL,
    UNIQUE (email, username)
);

CREATE TABLE IF NOT EXISTS userdetails (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES user(id) ON DELETE CASCADE,
    phone1 INTEGER NOT NULL,
    phone2 TEXT
);
CREATE INDEX IF NOT EXISTS ix_userdetails_parent ON userdetails(user_id);
