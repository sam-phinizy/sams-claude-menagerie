// Package spend holds the provider-neutral cost model and the aggregation
// the TUI renders from it.
package spend

import (
	"context"
	"sort"
	"time"
)

// Line is one rated cost bucket: what a provider charged for one model (or
// line item) on one UTC day.
type Line struct {
	Day          time.Time // midnight UTC
	Model        string
	USD          float64
	InputTokens  int64
	OutputTokens int64
}

// Provider fetches rated costs for the half-open UTC window [start, end).
type Provider interface {
	Name() string
	Fetch(ctx context.Context, start, end time.Time) ([]Line, error)
}

// Day truncates t to midnight UTC.
func Day(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Range is a named reporting window. End is exclusive.
type Range struct {
	Label      string
	Start, End time.Time
}

// Days reports how many whole days the range covers.
func (r Range) Days() int { return int(r.End.Sub(r.Start).Hours() / 24) }

// Ranges returns the selectable windows relative to now. The first is the
// default.
func Ranges(now time.Time) []Range {
	today := Day(now)
	tomorrow := today.AddDate(0, 0, 1)
	monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	return []Range{
		{"Month to date", monthStart, tomorrow},
		{"Last 7 days", today.AddDate(0, 0, -6), tomorrow},
		{"Last 30 days", today.AddDate(0, 0, -29), tomorrow},
		{"Last month", monthStart.AddDate(0, -1, 0), monthStart},
		{"Last 90 days", today.AddDate(0, 0, -89), tomorrow},
	}
}

// ModelTotal is a per-model rollup.
type ModelTotal struct {
	Model        string
	USD          float64
	InputTokens  int64
	OutputTokens int64
}

// Summary is everything the UI needs about one provider over one range.
type Summary struct {
	Total  float64
	Daily  []float64 // one entry per day of the range, oldest first
	Models []ModelTotal
}

// Summarize rolls lines up by day and by model. Lines outside r are ignored.
func Summarize(lines []Line, r Range) Summary {
	s := Summary{Daily: make([]float64, r.Days())}
	byModel := map[string]*ModelTotal{}
	for _, l := range lines {
		d := Day(l.Day)
		if d.Before(r.Start) || !d.Before(r.End) {
			continue
		}
		s.Daily[int(d.Sub(r.Start).Hours()/24)] += l.USD
		s.Total += l.USD
		m := byModel[l.Model]
		if m == nil {
			m = &ModelTotal{Model: l.Model}
			byModel[l.Model] = m
		}
		m.USD += l.USD
		m.InputTokens += l.InputTokens
		m.OutputTokens += l.OutputTokens
	}
	for _, m := range byModel {
		s.Models = append(s.Models, *m)
	}
	sort.Slice(s.Models, func(i, j int) bool {
		if s.Models[i].USD != s.Models[j].USD {
			return s.Models[i].USD > s.Models[j].USD
		}
		return s.Models[i].Model < s.Models[j].Model
	})
	return s
}

// ProjectMonth extrapolates month-to-date spend to the full month using the
// average daily spend so far (today counted as a full day). It returns 0 when
// r isn't a month-to-date range.
func ProjectMonth(total float64, r Range, now time.Time) float64 {
	today := Day(now)
	if r.Start.Day() != 1 || !r.End.Equal(today.AddDate(0, 0, 1)) {
		return 0
	}
	elapsed := r.Days()
	monthDays := r.Start.AddDate(0, 1, -1).Day()
	if elapsed <= 0 {
		return 0
	}
	return total / float64(elapsed) * float64(monthDays)
}
