package ui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/demo"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

func demoSource(p spend.Provider, budget float64) Source {
	return Source{Name: p.Name(), Budget: budget, Sync: func(ctx context.Context, now time.Time) ([]spend.Line, error) {
		return p.Fetch(ctx, spend.Day(now).AddDate(0, 0, -120), spend.Day(now).AddDate(0, 0, 1))
	}}
}

// TestViewsRender drives every view and range at a few terminal sizes, makes
// sure nothing overflows the terminal, and walks the Models cursor. Set
// SPENDTUI_DUMP=1 to print the frames.
func TestViewsRender(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	for _, size := range [][2]int{{120, 40}, {80, 24}, {50, 20}} {
		m := New([]Source{demoSource(demo.Claude(), 400), demoSource(demo.Fireworks(), 0)}, 0, 0)
		m.now = func() time.Time { return now }
		m.ranges = spend.Ranges(now)
		for i, s := range m.sources {
			lines, _ := s.Sync(context.Background(), now)
			next, _ := m.Update(fetchedMsg{idx: i, lines: lines, at: now})
			m = next.(Model)
		}
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		for v := 0; v < m.numViews(); v++ {
			for ri := range m.ranges {
				m.view, m.rangeIdx = view(v), ri
				for _, cur := range []int{0, 5, 100} {
					m.cursor = cur
					out := m.View()
					if !strings.Contains(out, "spendtui") {
						t.Fatalf("view %d range %d missing header", v, ri)
					}
					for _, line := range strings.Split(out, "\n") {
						if w := lipgloss.Width(line); w > size[0] {
							t.Errorf("view %d range %d at %dx%d: line is %d wide: %q", v, ri, size[0], size[1], w, line)
							break
						}
					}
					if os.Getenv("SPENDTUI_DUMP") != "" && ri == 0 && cur == 0 && size[0] == 120 {
						t.Logf("\n%s", out)
					}
					if v != int(viewModels) {
						break
					}
				}
			}
		}
	}
}

func TestStaleCacheDoesNotOverwriteSync(t *testing.T) {
	m := New([]Source{demoSource(demo.Claude(), 0)}, 0, 0)
	live := []spend.Line{{Day: time.Now(), Model: "live", USD: 1}}
	next, _ := m.Update(fetchedMsg{idx: 0, lines: live, at: time.Now()})
	next, _ = next.(Model).Update(fetchedMsg{idx: 0, lines: nil, at: time.Now().Add(-time.Hour), cached: true})
	if got := next.(Model).sources[0].lines; len(got) != 1 || got[0].Model != "live" {
		t.Fatalf("cache overwrote live data: %+v", got)
	}
}
