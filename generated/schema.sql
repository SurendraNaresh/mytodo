-- Generated from blueprint. Review before applying.
CREATE TABLE IF NOT EXISTS era (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    slug TEXT NOT NULL,
    title TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    theme_json TEXT NOT NULL,
    UNIQUE (slug)
);

CREATE TABLE IF NOT EXISTS album (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    era_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    sort_order INTEGER NOT NULL,
    FOREIGN KEY (era_id) REFERENCES era(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS media (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    album_id INTEGER NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('photo', 'short')),
    content_hash TEXT NOT NULL,
    duration_s INTEGER,
    caption TEXT,
    sort_order INTEGER NOT NULL,
    UNIQUE (content_hash),
    FOREIGN KEY (album_id) REFERENCES album(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS comment (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    body TEXT NOT NULL,
    created_at TEXT NOT NULL,
    FOREIGN KEY (media_id) REFERENCES media(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS layout_frame (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    region TEXT NOT NULL CHECK (region IN ('top', 'bottom', 'left', 'right', 'center')),
    feature TEXT NOT NULL,
    visible INTEGER NOT NULL,
    config_json TEXT NOT NULL,
    UNIQUE (region)
);