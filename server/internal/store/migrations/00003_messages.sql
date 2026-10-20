-- +goose Up
CREATE TABLE messages (
    id                 INTEGER PRIMARY KEY,
    jmap_id            TEXT    NOT NULL UNIQUE,
    blob_id            TEXT    NOT NULL,
    thread_id          TEXT    NOT NULL,
    message_id         TEXT,
    delivered_to       TEXT,
    source             TEXT    NOT NULL CHECK (source IN ('wpi', 'gmail', 'personal', 'other')),
    from_addr          TEXT    NOT NULL DEFAULT '',
    from_name          TEXT,
    subject            TEXT    NOT NULL DEFAULT '',
    received_at        INTEGER NOT NULL,
    body_text          TEXT    NOT NULL DEFAULT '',
    body_part          TEXT    NOT NULL CHECK (body_part IN ('html', 'text', 'none')),
    body_truncated     INTEGER NOT NULL DEFAULT 0 CHECK (body_truncated IN (0, 1)),
    normalizer_version INTEGER NOT NULL,
    content_hash       TEXT    NOT NULL,
    status             TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'removed', 'destroyed')),
    first_seen_at      INTEGER NOT NULL,
    updated_at         INTEGER NOT NULL
) STRICT;

CREATE INDEX messages_recent ON messages (received_at DESC) WHERE status = 'active';

-- +goose Down
DROP TABLE messages;