package spend

import (
	"math"
	"testing"
	"time"
)

func TestSummarize(t *testing.T) {
	r := Range{Start: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)}
	lines := []Line{
		{Day: r.Start, Model: "a", USD: 1},
		{Day: r.Start.AddDate(0, 0, 2), Model: "b", USD: 5, InputTokens: 10},
		{Day: r.Start.AddDate(0, 0, 2), Model: "a", USD: 2},
		{Day: r.End, Model: "a", USD: 100}, // outside: end is exclusive
	}
	s := Summarize(lines, r)
	if s.Total != 8 {
		t.Fatalf("total = %v, want 8", s.Total)
	}
	if want := []float64{1, 0, 7}; len(s.Daily) != 3 || s.Daily[0] != want[0] || s.Daily[2] != want[2] {
		t.Fatalf("daily = %v, want %v", s.Daily, want)
	}
	if s.Models[0].Model != "b" || s.Models[1].USD != 3 || s.Models[0].InputTokens != 10 {
		t.Fatalf("models = %+v", s.Models)
	}
}

func TestProjectMonth(t *testing.T) {
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	rs := Ranges(now)
	if got := ProjectMonth(100, rs[0], now); math.Abs(got-300) > 1e-9 { // 10 days → 30-day month
		t.Fatalf("projection = %v, want 300", got)
	}
	if got := ProjectMonth(100, rs[1], now); got != 0 {
		t.Fatalf("non-MTD projection = %v, want 0", got)
	}
}

func TestWeeklyAndTrailing(t *testing.T) {
	now := time.Date(2026, 9, 17, 9, 0, 0, 0, time.UTC) // Thursday
	if ws := WeekStart(now); ws.Weekday() != time.Monday || ws.Day() != 14 {
		t.Fatalf("week start = %v", ws)
	}
	if ws := WeekStart(time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)); ws.Day() != 14 { // Sunday
		t.Fatalf("sunday week start = %v", ws)
	}
	var lines []Line
	for d := 0; d < 21; d++ {
		lines = append(lines, Line{Day: now.AddDate(0, 0, -d), Model: "m", USD: float64(d)})
	}
	w := Weekly(lines, 3, now)
	// Current week Mon 14–Thu 17 is d=3..0 → 6; prior week d=4..10 → 49.
	if w[2] != 6 || w[1] != 49 {
		t.Fatalf("weekly = %v", w)
	}
	last, prior := Trailing(lines, now)
	if last != 21 || prior != 70 { // 0..6, 7..13
		t.Fatalf("trailing = %v, %v", last, prior)
	}
}
