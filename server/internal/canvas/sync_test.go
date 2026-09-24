package canvas

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/schultzh06/docket/server/internal/ics"
	"github.com/schultzh06/docket/server/internal/store"
)

type fakeFetcher struct{ body string }

func (f *fakeFetcher) Fetch(context.Context, string, ics.Validators) (ics.FetchResult, error) {
	return ics.FetchResult{Body: []byte(f.body)}, nil
}

func vevent(uid, start, summary string) string {
	return "BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTART:" + start + "\r\nSUMMARY:" + summary + "\r\nEND:VEVENT\r\n"
}

func feed(events ...string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//docket//test//EN\r\n" +
		strings.Join(events, "") + "END:VCALENDAR\r\n"
}

func TestSync(t *testing.T) {
	ctx := context.Background()
	conn, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	f := &fakeFetcher{}
	s := &Syncer{
		DB:      conn,
		Fetcher: f,
		Loc:     time.UTC,
		Now:     func() time.Time { return time.Unix(1_790_000_000, 0) },
	}
	sync := func() Stats {
		t.Helper()
		st, err := s.Sync(ctx)
		if err != nil {
			t.Fatalf("Sync: %v", err)
		}
		return st
	}
	status := func(uid string) string {
		t.Helper()
		var st string
		if err := conn.QueryRowContext(ctx,
			`SELECT status FROM agenda_items WHERE source_id = ?`, uid).Scan(&st); err != nil {
			t.Fatal(err)
		}
		return st
	}

	hw := vevent("event-assignment-1", "20261003T035900Z", "HW 3 [CS 3431]")
	quiz := vevent("event-assignment-2", "20261005T120000Z", "Quiz 1 [CS 3431]")

	// 1. First sync inserts everything.
	f.body = feed(hw, quiz)
	if st := sync(); st.Inserted != 2 {
		t.Fatalf("first sync: %+v, want 2 inserted", st)
	}

	// 2. Identical feed is short-circuited by the body hash.
	if st := sync(); !st.NotModified {
		t.Fatalf("second sync: %+v, want NotModified", st)
	}

	// 3. Mark HW done by hand, then the professor moves its deadline.
	if _, err := conn.ExecContext(ctx,
		`UPDATE agenda_items SET status = 'done' WHERE source_id = ?`, "event-assignment-1"); err != nil {
		t.Fatal(err)
	}
	hwMoved := vevent("event-assignment-1", "20261004T035900Z", "HW 3 [CS 3431]")
	f.body = feed(hwMoved, quiz)
	if st := sync(); st.Updated != 1 || st.Unchanged != 1 {
		t.Fatalf("deadline move: %+v, want 1 updated, 1 unchanged", st)
	}
	if got := status("event-assignment-1"); got != "done" {
		t.Errorf("status after update = %q, want done (manual status must survive)", got)
	}

	// 4. Quiz disappears from the feed, inside the window: removed.
	f.body = feed(hwMoved)
	if st := sync(); st.Removed != 1 {
		t.Fatalf("removal: %+v, want 1 removed", st)
	}
	if got := status("event-assignment-2"); got != "removed" {
		t.Errorf("quiz status = %q, want removed", got)
	}

	// 5. Empty feed must not remove anything.
	f.body = feed()
	if st := sync(); st.Removed != 0 {
		t.Fatalf("empty feed: %+v, want 0 removed", st)
	}
}

// TestSyncCursorRoundTrip verifies the cursor written by one sync is
// read back correctly by the next, taking the body-hash short-circuit.
func TestSyncCursorRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	f := &fakeFetcher{body: feed(vevent("event-assignment-1", "20261003T035900Z", "HW 3 [CS 3431]"))}
	s := &Syncer{
		DB:      conn,
		Fetcher: f,
		Loc:     time.UTC,
		Now:     func() time.Time { return time.Unix(1_790_000_000, 0) },
	}

	rawCursor := func() string {
		t.Helper()
		var cur sql.NullString
		if err := conn.QueryRowContext(ctx,
			`SELECT cursor FROM sync_state WHERE source = 'canvas'`).Scan(&cur); err != nil {
			t.Fatal(err)
		}
		return cur.String
	}

	if st, err := s.Sync(ctx); err != nil || st.Inserted != 1 {
		t.Fatalf("first sync: stats=%+v err=%v, want 1 inserted, no error", st, err)
	}

	sum := sha256.Sum256([]byte(f.body))
	wantHash := hex.EncodeToString(sum[:])

	c, err := ics.DecodeCursor(rawCursor())
	if err != nil {
		t.Fatalf("decode persisted cursor: %v", err)
	}
	if c.BodyHash != wantHash {
		t.Fatalf("persisted cursor body hash = %q, want %q", c.BodyHash, wantHash)
	}

	// Second sync against the same fixture must read the cursor back and
	// take the body-hash short-circuit.
	st, err := s.Sync(ctx)
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if !st.NotModified {
		t.Fatalf("second sync: %+v, want NotModified", st)
	}

	c2, err := ics.DecodeCursor(rawCursor())
	if err != nil {
		t.Fatalf("decode persisted cursor after second sync: %v", err)
	}
	if c2.BodyHash != wantHash {
		t.Fatalf("cursor body hash after second sync = %q, want %q", c2.BodyHash, wantHash)
	}
}

// TestSyncCorruptCursor verifies that an unreadable cursor degrades to a
// full fetch instead of failing the sync.
func TestSyncCorruptCursor(t *testing.T) {
	ctx := context.Background()
	conn, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	f := &fakeFetcher{body: feed(vevent("event-assignment-1", "20261003T035900Z", "HW 3 [CS 3431]"))}
	s := &Syncer{
		DB:      conn,
		Fetcher: f,
		Loc:     time.UTC,
		Now:     func() time.Time { return time.Unix(1_790_000_000, 0) },
	}

	if st, err := s.Sync(ctx); err != nil || st.Inserted != 1 {
		t.Fatalf("first sync: stats=%+v err=%v, want 1 inserted, no error", st, err)
	}

	// Valid JSON (satisfies the column's json_valid CHECK constraint) but
	// the wrong shape for ics.Cursor, so it fails to unmarshal.
	if _, err := conn.ExecContext(ctx,
		`UPDATE sync_state SET cursor = '{"etag": 123}' WHERE source = 'canvas'`); err != nil {
		t.Fatal(err)
	}

	st, err := s.Sync(ctx)
	if err != nil {
		t.Fatalf("sync with corrupt cursor: %v", err)
	}
	if st.NotModified {
		t.Fatalf("sync with corrupt cursor: %+v, want a full fetch (no short-circuit)", st)
	}

	var raw sql.NullString
	if err := conn.QueryRowContext(ctx,
		`SELECT cursor FROM sync_state WHERE source = 'canvas'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if _, err := ics.DecodeCursor(raw.String); err != nil {
		t.Fatalf("cursor after recovery is still unreadable: %v", err)
	}
}
