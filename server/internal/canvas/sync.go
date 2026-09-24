package canvas

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/schultzh06/docket/server/internal/ics"
	"github.com/schultzh06/docket/server/internal/store/db"
)

const source = "canvas"

type fetcher interface {
	Fetch(ctx context.Context, feedURL string, prev ics.Validators) (ics.FetchResult, error)
}

type Syncer struct {
	DB      *sql.DB
	Fetcher fetcher
	FeedURL string
	Loc     *time.Location
	Now     func() time.Time
}

type Stats struct {
	NotModified bool
	Inserted    int
	Updated     int
	Unchanged   int
	Removed     int
	Skipped     int
}

func (s *Syncer) Sync(ctx context.Context) (Stats, error) {
	q := db.New(s.DB)
	now := s.Now().Unix()

	state, err := q.GetSyncState(ctx, source)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Stats{}, fmt.Errorf("load sync state: %w", err)
	}
	cur, err := ics.DecodeCursor(state.Cursor.String)
	if err != nil {
		slog.Warn("discarding unreadable sync cursor", "source", source, "err", err)
		cur = ics.Cursor{}
	}
	prev := ics.Validators{ETag: cur.ETag, LastModified: cur.LastModified}

	res, err := s.Fetcher.Fetch(ctx, s.FeedURL, prev)
	if err != nil {
		return Stats{}, s.fail(ctx, q, err, now)
	}
	if res.NotModified {
		return Stats{NotModified: true}, s.succeed(ctx, q, res.Validators, cur.BodyHash, now)
	}

	sum := sha256.Sum256(res.Body)
	bodyHash := hex.EncodeToString(sum[:])
	if bodyHash == cur.BodyHash {
		return Stats{NotModified: true}, s.succeed(ctx, q, res.Validators, bodyHash, now)
	}

	parsed, err := ics.Parse(bytes.NewReader(res.Body), s.Loc)
	if err != nil {
		return Stats{}, s.fail(ctx, q, err, now)
	}

	st, err := s.apply(ctx, parsed.Events, now)
	if err != nil {
		return Stats{}, s.fail(ctx, q, err, now)
	}
	st.Skipped = len(parsed.Skipped)
	return st, s.succeed(ctx, q, res.Validators, bodyHash, now)
}

// Apply: writes one feed's events in a single transaction.
func (s *Syncer) apply(ctx context.Context, events []ics.Event, now int64) (Stats, error) {
	var st Stats

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return st, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op once committed

	q := db.New(tx)
	courses := map[string]int64{}
	seen := make(map[string]struct{}, len(events))
	var windowStart int64

	// Upserts
	for i, ev := range events {
		it := toItem(ev)
		seen[ev.UID] = struct{}{}
		if i == 0 || it.StartsAt < windowStart {
			windowStart = it.StartsAt
		}

		courseID, err := resolveCourse(ctx, q, courses, it.Course, now)
		if err != nil {
			return st, err
		}
		hash := it.hash()

		existing, err := q.GetAgendaItemByKey(ctx, db.GetAgendaItemByKeyParams{Source: source, SourceID: ev.UID})
		switch {
		case errors.Is(err, sql.ErrNoRows):
			// New item / Insert
			if err := q.InsertAgendaItem(ctx, db.InsertAgendaItemParams{
				Source:      source,
				SourceID:    ev.UID,
				Kind:        it.Kind,
				CourseID:    courseID,
				Title:       it.Title,
				StartsAt:    it.StartsAt,
				EndsAt:      nullInt(it.EndsAt),
				AllDay:      boolInt(it.AllDay),
				SourceUrl:   nullString(it.URL),
				ContentHash: hash,
				FirstSeenAt: now,
				UpdatedAt:   now,
			}); err != nil {
				return st, fmt.Errorf("insert %s: %w", ev.UID, err)
			}
			st.Inserted++

		case err != nil:
			return st, fmt.Errorf("lookup %s: %w", ev.UID, err)

		case existing.ContentHash == hash && existing.Status != "removed":
			st.Unchanged++

		default:
			// Update
			if err := q.UpdateAgendaItem(ctx, db.UpdateAgendaItemParams{
				Kind:        it.Kind,
				CourseID:    courseID,
				Title:       it.Title,
				StartsAt:    it.StartsAt,
				EndsAt:      nullInt(it.EndsAt),
				AllDay:      boolInt(it.AllDay),
				SourceUrl:   nullString(it.URL),
				ContentHash: hash,
				UpdatedAt:   now,
				ID:          existing.ID,
			}); err != nil {
				return st, fmt.Errorf("update %s: %w", ev.UID, err)
			}
			st.Updated++
		}
	}

	// Removal pass, skipped for empty feed
	if len(events) > 0 {
		active, err := q.ListActiveSince(ctx, db.ListActiveSinceParams{Source: source, WindowStart: windowStart})
		if err != nil {
			return st, fmt.Errorf("list active: %w", err)
		}
		for _, row := range active {
			if _, ok := seen[row.SourceID]; ok {
				continue
			}
			if err := q.MarkRemoved(ctx, db.MarkRemovedParams{UpdatedAt: now, ID: row.ID}); err != nil {
				return st, fmt.Errorf("remove %s: %w", row.SourceID, err)
			}
			st.Removed++
		}
	}

	if err := tx.Commit(); err != nil {
		return st, fmt.Errorf("commit: %w", err)
	}
	return st, nil
}

// normalized, hashable form of a feed event
type item struct {
	Kind     string `json:"kind"`
	Course   string `json:"course"`
	Title    string `json:"title"`
	StartsAt int64  `json:"starts_at"`
	EndsAt   int64  `json:"ends_at"`
	AllDay   bool   `json:"all_day"`
	URL      string `json:"url"`
}

func toItem(ev ics.Event) item {
	title, course := SplitSummary(ev.Summary)
	kind := "event"
	if IsAssignment(ev.UID) {
		kind = "due"
	}
	it := item{
		Kind:     kind,
		Course:   course,
		Title:    title,
		StartsAt: ev.Start.Unix(),
		AllDay:   ev.AllDay,
		URL:      ev.URL,
	}
	if !ev.End.IsZero() {
		it.EndsAt = ev.End.Unix()
	}
	return it
}

func (it item) hash() string {
	b, _ := json.Marshal(it) // cannot fail: only strings, ints, bools
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func resolveCourse(ctx context.Context, q *db.Queries, cache map[string]int64, code string, now int64) (sql.NullInt64, error) {
	if code == "" {
		return sql.NullInt64{}, nil
	}
	if id, ok := cache[code]; ok {
		return sql.NullInt64{Int64: id, Valid: true}, nil
	}
	id, err := q.UpsertCourse(ctx, db.UpsertCourseParams{Code: code, Name: code, CreatedAt: now})
	if err != nil {
		return sql.NullInt64{}, fmt.Errorf("upsert course %q: %w", code, err)
	}
	cache[code] = id
	return sql.NullInt64{Int64: id, Valid: true}, nil
}

func (s *Syncer) succeed(ctx context.Context, q *db.Queries, v ics.Validators, bodyHash string, now int64) error {
	enc, err := ics.Cursor{ETag: v.ETag, LastModified: v.LastModified, BodyHash: bodyHash}.Encode()
	if err != nil {
		return fmt.Errorf("encode cursor: %w", err)
	}
	if err := q.SaveSyncSuccess(ctx, db.SaveSyncSuccessParams{
		Source:        source,
		Cursor:        sql.NullString{String: enc, Valid: true},
		LastAttemptAt: sql.NullInt64{Int64: now, Valid: true},
		LastSuccessAt: sql.NullInt64{Int64: now, Valid: true},
	}); err != nil {
		return fmt.Errorf("save sync state: %w", err)
	}
	return nil
}

// fail records the error for the status panel and returns it unchanged.
func (s *Syncer) fail(ctx context.Context, q *db.Queries, cause error, now int64) error {
	return errors.Join(cause, q.SaveSyncError(ctx, db.SaveSyncErrorParams{
		Source:        source,
		LastAttemptAt: sql.NullInt64{Int64: now, Valid: true},
		LastError:     nullString(cause.Error()),
	}))
}

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
func nullInt(n int64) sql.NullInt64      { return sql.NullInt64{Int64: n, Valid: n != 0} }

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
