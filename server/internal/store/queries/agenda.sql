-- name: GetAgendaItemByKey :one
SELECT id, content_hash, status 
FROM agenda_items
WHERE source = ? AND source_id = ?;

-- name: InsertAgendaItem :exec
INSERT INTO agenda_items (
    source, source_id, kind, course_id, title,
    starts_at, ends_at, all_day, source_url,
    content_hash, first_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: UpdateAgendaItem :exec
UPDATE agenda_items SET
    kind         = ?,
    course_id    = ?,
    title        = ?,
    starts_at    = ?,
    ends_at      = ?,
    all_day      = ?,
    source_url   = ?,
    content_hash = ?,
    updated_at   = ?,
    status       = CASE WHEN status = 'removed' THEN 'active' ELSE status END
WHERE id = ?;

-- name: ListActiveSince :many
SELECT id, source_id
FROM agenda_items
WHERE source = sqlc.arg(source)
  AND status = 'active'
  AND starts_at >= sqlc.arg(window_start);

-- name: MarkRemoved :exec
UPDATE agenda_items
SET status = 'removed', updated_at = ?
WHERE id = ?;