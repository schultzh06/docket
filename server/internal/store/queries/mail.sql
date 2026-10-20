-- name: GetMessageByJMAPID :one
SELECT id, status, normalizer_version, content_hash
FROM messages
WHERE jmap_id = ?;

-- name: InsertMessage :one
INSERT INTO messages (
    jmap_id, blob_id, thread_id, message_id, delivered_to, source,
    from_addr, from_name, subject, received_at,
    body_text, body_part, body_truncated, normalizer_version, content_hash,
    first_seen_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: SetMessageStatus :execrows
UPDATE messages
SET status = ?, updated_at = ?
WHERE jmap_id = ? AND status != ?;

-- name: UpdateMessageBody :exec
UPDATE messages
SET body_text = ?, body_part = ?, body_truncated = ?,
    normalizer_version = ?, content_hash = ?, updated_at = ?
WHERE id = ?;