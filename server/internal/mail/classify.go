package jmap

/*
import (
	"context"
	"database/sql"
	"errors"

	"github.com/schultzh06/docket/server/internal/store/db"
)

// classify decides what to do with each id, using local state + current location.
func classify(ctx context.Context, q *db.Queries, mb mailboxSet,
	lite []jmap.Email) (needFull []string, statusChanges map[string]string, err error) {

	statusChanges = make(map[string]string)
	for _, e := range lite {
		included := mb.isIncluded(e.MailboxIDs)
		row, err := q.GetMessageByJMAPID(ctx, e.ID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if included {
				needFull = append(needFull, e.ID)
			} // else: never seen, still excluded → skip
		case err != nil:
			return nil, nil, err
		default:
			want := "removed"
			if included {
				want = "active"
			}
			if row.Status != want {
				statusChanges[e.ID] = want
			}
		}
	}
	return needFull, statusChanges, nil
}
*/
