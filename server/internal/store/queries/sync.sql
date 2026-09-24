-- name: UpsertCourse :one
INSERT INTO courses (code, name, created_at)
VALUES (?, ?, ?)
ON CONFLICT (code) DO UPDATE SET code = excluded.code
RETURNING id;

-- name: GetSyncState :one
SELECT * FROM sync_state WHERE source = ?;

-- name: SaveSyncSuccess :exec
INSERT INTO sync_state (source, cursor, last_attempt_at, last_success_at, last_error, consecutive_failures)
VALUES (?, ?, ?, ?, NULL, 0)
ON CONFLICT (source) DO UPDATE SET
    cursor               = excluded.cursor,
    last_attempt_at      = excluded.last_attempt_at,
    last_success_at      = excluded.last_success_at,
    last_error           = NULL,
    consecutive_failures = 0;

-- name: SaveSyncError :exec
INSERT INTO sync_state (source, last_attempt_at, last_error, consecutive_failures)
VALUES (?, ?, ?, 1)
ON CONFLICT (source) DO UPDATE SET
    last_attempt_at      = excluded.last_attempt_at,
    last_error           = excluded.last_error,
    consecutive_failures = consecutive_failures + 1;