package ics

import (
	"os"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	f, err := os.Open("testdata/sample.ics")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}

	res, err := Parse(f, ny)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(res.Events) != 2 {
		t.Fatalf("got %d events, want 2", len(res.Events))
	}
	if len(res.Skipped) != 1 {
		t.Errorf("got %d skipped, want 1", len(res.Skipped))
	}

	hw := res.Events[0]
	if hw.UID != "event-assignment-101" || hw.AllDay {
		t.Errorf("unexpected first event: %+v", hw)
	}
	if want := time.Date(2026, 10, 3, 3, 59, 0, 0, time.UTC); !hw.Start.Equal(want) {
		t.Errorf("start = %v, want %v", hw.Start, want)
	}

	proj := res.Events[1]
	if !proj.AllDay {
		t.Error("second event should be all-day")
	}
	// Oct 10 in New York, not Oct 9 at 8pm.
	if y, m, d := proj.Start.In(ny).Date(); y != 2026 || m != time.October || d != 10 {
		t.Errorf("all-day date = %d-%d-%d, want 2026-10-10", y, m, d)
	}
}
