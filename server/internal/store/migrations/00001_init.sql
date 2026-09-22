-- +goose Up
CREATE TABLE courses (
    id         INTEGER PRIMARY KEY,
    code       TEXT    NOT NULL UNIQUE,
    name       TEXT    NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

CREATE TABLE agenda_items (
    id            INTEGER PRIMARY KEY,
    source        TEXT    NOT NULL CHECK (source IN ('canvas', 'calendar', 'email', 'manual')),
    source_id     TEXT    NOT NULL,
    kind          TEXT    NOT NULL CHECK (kind IN ('event', 'due', 'proposed')),
    course_id     INTEGER REFERENCES courses (id),
    title         TEXT    NOT NULL,
    starts_at     INTEGER NOT NULL,
    ends_at       INTEGER,
    all_day       INTEGER NOT NULL DEFAULT 0 CHECK (all_day IN (0, 1)),
    source_url    TEXT,
    status        TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'done', 'removed')),
    content_hash  TEXT    NOT NULL,
    first_seen_at INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    UNIQUE (source, source_id)
) STRICT;

CREATE INDEX agenda_items_starts_at ON agenda_items (starts_at);

CREATE TABLE sync_state (
    source          TEXT PRIMARY KEY,
    etag            TEXT,
    last_modified   TEXT,
    body_hash       TEXT,
    last_success_at INTEGER,
    last_error      TEXT
) STRICT;

-- +goose Down
DROP TABLE sync_state;
DROP TABLE agenda_items;
DROP TABLE courses;