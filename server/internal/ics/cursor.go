package ics

import "encoding/json"

// Cursor is the persisted position of one ICS feed between syncs.
type Cursor struct {
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified,omitempty"`
	BodyHash     string `json:"body_hash,omitempty"`
}

// DecodeCursor parses a cursor persisted by Encode. An empty raw string
// (the zero value of a NULL column) decodes to the zero Cursor.
func DecodeCursor(raw string) (Cursor, error) {
	var c Cursor
	if raw == "" {
		return c, nil
	}
	err := json.Unmarshal([]byte(raw), &c)
	return c, err
}

func (c Cursor) Encode() (string, error) {
	b, err := json.Marshal(c)
	return string(b), err
}
