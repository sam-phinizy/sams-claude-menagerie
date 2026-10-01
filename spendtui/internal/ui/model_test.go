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
)

// TestViewsRender drives every view and range at a few terminal sizes to make
// sure nothing panics. Set SPENDTUI_DUMP=1 to print the frames.
func TestViewsRender(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {50, 20}} {
		m := New([]Source{{Provider: demo.Claude(), Budget: 400}, {Provider: demo.Fireworks()}}, 0)
		m.now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }
		m.ranges = nil
		_ = m.refresh() // recompute ranges against the fixed clock
		for i, s := range m.sources {
			r := m.ranges[4]
			lines, _ := s.Provider.Fetch(context.Background(), r.Start, m.ranges[0].End)
			next, _ := m.Update(fetchedMsg{idx: i, lines: lines, at: m.now()})
			m = next.(Model)
		}
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = next.(Model)
		for v := 0; v < m.numViews(); v++ {
			for ri := range m.ranges {
				m.view, m.rangeIdx = view(v), ri
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
				if os.Getenv("SPENDTUI_DUMP") != "" && ri == 0 && size[0] == 120 {
					t.Logf("\n%s", out)
				}
			}
		}
	}
}
