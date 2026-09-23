package canvas

import (
	"context"
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
