package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

type fakeProvider struct {
	calls [][2]time.Time
	lines func(start, end time.Time) []spend.Line
}

func (f *fakeProvider) Name() string { return "Fake" }
func (f *fakeProvider) Fetch(_ context.Context, start, end time.Time) ([]spend.Line, error) {
	f.calls = append(f.calls, [2]time.Time{start, end})
	return f.lines(start, end), nil
}

func day(s string) time.Time { t, _ := time.Parse(time.DateOnly, s); return t }

func TestSyncBackfillsThenRestatesTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spend.duckdb")
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	usd := 1.0
	p := &fakeProvider{lines: func(start, end time.Time) []spend.Line {
		var out []spend.Line
		for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
			// Two lines for the same model and day must be summed on store.
			out = append(out, spend.Line{Day: d, Model: "m", USD: usd, InputTokens: 10},
				spend.Line{Day: d, Model: "m", USD: usd, OutputTokens: 5})
		}
		return out
	}}
	sy := &Syncer{Store: st, Provider: p, Backfill: 10 * 24 * time.Hour, Restate: 2, Keep: 365 * 24 * time.Hour}
	now := day("2026-09-20").Add(12 * time.Hour)

	lines, err := sy.Sync(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 11 || lines[0].USD != 2 || lines[0].InputTokens != 10 || lines[0].OutputTokens != 5 {
		t.Fatalf("after backfill: %d lines, first %+v", len(lines), lines[0])
	}
	if !p.calls[0][0].Equal(day("2026-09-10")) || !p.calls[0][1].Equal(day("2026-09-21")) {
		t.Fatalf("backfill window = %v", p.calls[0])
	}

	// Second sync only refetches the restated tail, and its new values win.
	usd = 5
	lines, err = sy.Sync(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if !p.calls[1][0].Equal(day("2026-09-18")) {
		t.Fatalf("incremental window = %v", p.calls[1])
	}
	if len(lines) != 11 || lines[7].USD != 2 || lines[8].USD != 10 {
		t.Fatalf("after restate: %+v", lines)
	}
	st.Close()

	// Data survives reopening.
	st, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	sy.Store = st
	cached, at, err := sy.Cached(context.Background(), now)
	if err != nil || len(cached) != 11 || at.IsZero() {
		t.Fatalf("cached = %d lines, at %v, err %v", len(cached), at, err)
	}
}
