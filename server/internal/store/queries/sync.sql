-- name: UpsertCourse :one
INSERT INTO courses (code, name, created_at)
VALUES (?, ?, ?)
ON CONFLICT (code) DO UPDATE SET code = excluded.code
RETURNING id;

-- name: GetSyncState :one
SELECT * FROM sync_state WHERE source = ?;

-- name: SaveSyncSuccess :exec
INSERT INTO sync_state (source, etag, last_modified, body_hash, last_success_at, last_error)
VALUES (?, ?, ?, ?, ?, NULL)
ON CONFLICT (source) DO UPDATE SET
    etag            = excluded.etag,
    last_modified   = excluded.last_modified,
    body_hash       = excluded.body_hash,
    last_success_at = excluded.last_success_at,
    last_error      = NULL;

-- name: SaveSyncError :exec
INSERT INTO sync_state (source, last_error)
VALUES (?, ?)
ON CONFLICT (source) DO UPDATE SET last_error = excluded.last_error;