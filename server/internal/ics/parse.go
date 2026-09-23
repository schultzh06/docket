package ics

import (
	"fmt"
	"io"
	"time"

	"github.com/emersion/go-ical"
)

type Event struct {
	UID     string
	Summary string
	Start   time.Time
	End     time.Time
	AllDay  bool
	URL     string
}

type ParseResult struct {
	Events  []Event
	Skipped []error // Per-event errors store here so the rest of feed is usable
}

func Parse(r io.Reader, loc *time.Location) (ParseResult, error) {
	cal, err := ical.NewDecoder(r).Decode()
	if err != nil {
		return ParseResult{}, fmt.Errorf("decide ics: %w", err)
	}

	var res ParseResult
	for _, ev := range cal.Events() {
		e, err := convert(ev, loc)
		if err != nil {
			res.Skipped = append(res.Skipped, err)
			continue
		}
		res.Events = append(res.Events, e)
	}
	return res, nil
}

func convert(ev ical.Event, loc *time.Location) (Event, error) {
	uid, err := ev.Props.Text(ical.PropUID)
	if err != nil || uid == "" {
		return Event{}, fmt.Errorf("event missing UID")
	}

	start, err := ev.DateTimeStart(loc)
	if err != nil {
		return Event{}, fmt.Errorf("event %s: bad DTSTART: %w", uid, err)
	}

	e := Event{UID: uid, Start: start}

	if p := ev.Props.Get(ical.PropDateTimeStart); p != nil && p.ValueType() == ical.ValueDate {
		e.AllDay = true
	}
	if end, err := ev.DateTimeEnd(loc); err == nil {
		e.End = end
	}
	e.Summary, _ = ev.Props.Text(ical.PropSummary)
	e.URL, _ = ev.Props.Text(ical.PropURL)

	return e, nil
}
