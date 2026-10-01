// Package ui is the Bubble Tea front end.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

// Source is one provider as the UI sees it.
type Source struct {
	Name   string
	Budget float64 // monthly USD; 0 hides the budget bar
	// Cached returns stored history without touching the network, and when
	// it was last synced. Optional.
	Cached func(ctx context.Context, now time.Time) ([]spend.Line, time.Time, error)
	// Sync refreshes from the provider and returns the full history.
	Sync func(ctx context.Context, now time.Time) ([]spend.Line, error)
}

type source struct {
	Source
	color   lipgloss.TerminalColor
	lines   []spend.Line
	err     error
	loading bool
	fetched time.Time
}

type view int

const (
	viewOverview view = iota
	viewWeekly
	viewModels
	viewDaily
	numFixedViews
	// one provider view per source follows
)

var fixedViewNames = []string{"Overview", "Weekly", "Models", "Daily"}

type Model struct {
	sources  []*source
	ranges   []spend.Range
	rangeIdx int
	view     view
	scroll   int // Daily / provider tables
	cursor   int // Models view selection
	width    int
	height   int
	every    time.Duration
	spin     spinner.Model
	now      func() time.Time
}

type fetchedMsg struct {
	idx    int
	lines  []spend.Line
	err    error
	at     time.Time
	cached bool
}

type tickMsg struct{}

// New builds the model. rangeIdx picks the initial range (see spend.Ranges).
func New(srcs []Source, refreshEvery time.Duration, rangeIdx int) Model {
	palette := []lipgloss.TerminalColor{claudeColor, fireworksColor, accentColor, warnColor}
	m := Model{every: refreshEvery, now: time.Now, width: 100, height: 40}
	for i, s := range srcs {
		m.sources = append(m.sources, &source{Source: s, color: palette[i%len(palette)]})
	}
	m.ranges = spend.Ranges(m.now())
	if rangeIdx >= 0 && rangeIdx < len(m.ranges) {
		m.rangeIdx = rangeIdx
	}
	m.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(mutedStyle))
	return m
}

// ViewNames lists the tabs in order, for flags and help text.
func (m Model) ViewNames() []string {
	names := append([]string{}, fixedViewNames...)
	for _, s := range m.sources {
		names = append(names, s.Name)
	}
	return names
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.spin.Tick, m.loadCached(), m.refresh(), m.scheduleTick()}
	return tea.Batch(cmds...)
}

func (m Model) scheduleTick() tea.Cmd {
	if m.every <= 0 {
		return nil
	}
	return tea.Tick(m.every, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m Model) loadCached() tea.Cmd {
	var cmds []tea.Cmd
	now := m.now()
	for i, s := range m.sources {
		if s.Cached == nil {
			continue
		}
		i, load := i, s.Cached
		cmds = append(cmds, func() tea.Msg {
			lines, at, err := load(context.Background(), now)
			return fetchedMsg{idx: i, lines: lines, err: err, at: at, cached: true}
		})
	}
	return tea.Batch(cmds...)
}

func (m *Model) refresh() tea.Cmd {
	m.ranges = spend.Ranges(m.now())
	now := m.now()
	var cmds []tea.Cmd
	for i, s := range m.sources {
		s.loading = true
		i, sync := i, s.Sync
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			lines, err := sync(ctx, now)
			return fetchedMsg{idx: i, lines: lines, err: err, at: time.Now()}
		})
	}
	return tea.Batch(cmds...)
}

func (m Model) numViews() int { return int(numFixedViews) + len(m.sources) }

// SetView selects a tab by (case-insensitive) name.
func (m *Model) SetView(name string) error {
	for i, n := range m.ViewNames() {
		if strings.EqualFold(n, name) {
			m.view = view(i)
			return nil
		}
	}
	return fmt.Errorf("unknown view %q (have %s)", name, strings.Join(m.ViewNames(), ", "))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case fetchedMsg:
		s := m.sources[msg.idx]
		if msg.cached {
			// A cache read that lands after a live sync is stale; drop it.
			if s.fetched.IsZero() && msg.err == nil {
				s.lines, s.fetched = msg.lines, msg.at
			}
			break
		}
		s.loading = false
		s.err = msg.err
		if msg.err == nil {
			s.lines, s.fetched = msg.lines, msg.at
		}
	case tickMsg:
		return m, tea.Batch(m.refresh(), m.scheduleTick())
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "tab":
			m.view = (m.view + 1) % view(m.numViews())
			m.scroll = 0
		case "shift+tab":
			m.view = (m.view + view(m.numViews()) - 1) % view(m.numViews())
			m.scroll = 0
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if v := int(msg.String()[0] - '1'); v < m.numViews() {
				m.view, m.scroll = view(v), 0
			}
		case "right", "l", "]":
			m.rangeIdx = (m.rangeIdx + 1) % len(m.ranges)
			m.scroll = 0
		case "left", "h", "[":
			m.rangeIdx = (m.rangeIdx + len(m.ranges) - 1) % len(m.ranges)
			m.scroll = 0
		case "down", "j":
			if m.view == viewModels {
				m.cursor++ // clamped when rendering
			} else {
				m.scroll++
			}
		case "up", "k":
			if m.view == viewModels {
				m.cursor = max(m.cursor-1, 0)
			} else if m.scroll > 0 {
				m.scroll--
			}
		case "r":
			return m, m.refresh()
		}
	}
	return m, nil
}

func (m Model) View() string {
	r := m.ranges[m.rangeIdx]
	var body string
	switch m.view {
	case viewOverview:
		body = m.overview(r)
	case viewWeekly:
		body = m.weekly()
	case viewModels:
		body = m.models(r)
	case viewDaily:
		body = m.daily(r)
	default:
		body = m.providerView(m.sources[int(m.view-numFixedViews)], r)
	}
	frame := lipgloss.JoinVertical(lipgloss.Left, m.header(r), "", body, "", m.footer())
	// Truncate rather than wrap on terminals too small for the layout.
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(frame)
}

func (m Model) header(r spend.Range) string {
	var tabs []string
	for i, n := range m.ViewNames() {
		label := fmt.Sprintf("%d %s", i+1, n)
		if view(i) == m.view {
			tabs = append(tabs, activeTab.Render(label))
		} else {
			tabs = append(tabs, tabStyle.Render(label))
		}
	}
	last := r.End.AddDate(0, 0, -1)
	rng := fmt.Sprintf("◀ %s ▶  %s – %s UTC", r.Label, r.Start.Format("Jan 2"), last.Format("Jan 2"))
	if m.view == viewWeekly {
		rng = "Week over week · weeks start Monday, UTC"
	}
	status := ""
	for _, s := range m.sources {
		if s.loading {
			status = m.spin.View() + " syncing"
			break
		}
	}
	if status == "" {
		var newest time.Time
		for _, s := range m.sources {
			if s.fetched.After(newest) {
				newest = s.fetched
			}
		}
		if !newest.IsZero() {
			status = "synced " + newest.Local().Format("Jan 2 15:04")
		}
	}
	top := titleStyle.Render("spendtui") + "  " + lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return lipgloss.JoinVertical(lipgloss.Left, top, boldStyle.Render(rng)+"   "+mutedStyle.Render(status))
}

func (m Model) footer() string {
	return mutedStyle.Render("tab/1-9 view · ←/→ range · j/k move · r refresh · q quit")
}

func (m Model) contentWidth() int { return max(m.width-2, 40) }

// Snapshot syncs every source synchronously and renders one frame, for
// non-interactive use (screenshots, status scripts).
func Snapshot(srcs []Source, viewName string, rangeIdx, width, height int) (string, error) {
	m := New(srcs, 0, rangeIdx)
	if err := m.SetView(viewName); err != nil {
		return "", err
	}
	m.width, m.height = width, height
	for i, s := range m.sources {
		lines, err := s.Sync(context.Background(), m.now())
		next, _ := m.Update(fetchedMsg{idx: i, lines: lines, err: err, at: time.Now()})
		m = next.(Model)
	}
	return m.View(), nil
}
