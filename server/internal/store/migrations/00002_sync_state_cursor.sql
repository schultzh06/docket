-- +goose Up
CREATE TABLE sync_state_new (
    source               TEXT    PRIMARY KEY,
    cursor               TEXT    CHECK (cursor IS NULL OR json_valid(cursor)),
    last_attempt_at      INTEGER,
    last_success_at      INTEGER,
    last_error           TEXT,
    consecutive_failures INTEGER NOT NULL DEFAULT 0
) STRICT;

INSERT INTO sync_state_new (source, cursor, last_success_at, last_error)
SELECT source,
       json_object('etag', etag, 'last_modified', last_modified, 'body_hash', body_hash),
       last_success_at,
       last_error
FROM sync_state;

DROP TABLE sync_state;
ALTER TABLE sync_state_new RENAME TO sync_state;

-- +goose Down
CREATE TABLE sync_state_old (
    source          TEXT PRIMARY KEY,
    etag            TEXT,
    last_modified   TEXT,
    body_hash       TEXT,
    last_success_at INTEGER,
    last_error      TEXT
) STRICT;

INSERT INTO sync_state_old
SELECT source,
       json_extract(cursor, '$.etag'),
       json_extract(cursor, '$.last_modified'),
       json_extract(cursor, '$.body_hash'),
       last_success_at,
       last_error
FROM sync_state;

DROP TABLE sync_state;
ALTER TABLE sync_state_old RENAME TO sync_state;