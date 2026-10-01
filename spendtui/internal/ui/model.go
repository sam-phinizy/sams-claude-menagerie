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

// Source is a provider plus how the UI should present it.
type Source struct {
	Provider spend.Provider
	Budget   float64 // monthly USD; 0 disables the budget bar
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
	viewDaily
	// provider views follow, one per source
)

type Model struct {
	sources  []*source
	ranges   []spend.Range
	rangeIdx int
	view     view
	scroll   int
	width    int
	height   int
	every    time.Duration
	spin     spinner.Model
	now      func() time.Time
}

type fetchedMsg struct {
	idx   int
	lines []spend.Line
	err   error
	at    time.Time
}

type tickMsg struct{}

func New(srcs []Source, refreshEvery time.Duration) Model {
	palette := []lipgloss.TerminalColor{claudeColor, fireworksColor, accentColor, warnColor}
	m := Model{every: refreshEvery, now: time.Now, width: 100, height: 40}
	for i, s := range srcs {
		m.sources = append(m.sources, &source{Source: s, color: palette[i%len(palette)]})
	}
	m.ranges = spend.Ranges(m.now())
	m.spin = spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(mutedStyle))
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.refresh(), m.scheduleTick())
}

func (m Model) scheduleTick() tea.Cmd {
	if m.every <= 0 {
		return nil
	}
	return tea.Tick(m.every, func(time.Time) tea.Msg { return tickMsg{} })
}

// refresh fetches one window that covers every selectable range, so switching
// ranges afterwards is instant.
func (m *Model) refresh() tea.Cmd {
	m.ranges = spend.Ranges(m.now())
	start, end := m.ranges[0].Start, m.ranges[0].End
	for _, r := range m.ranges {
		if r.Start.Before(start) {
			start = r.Start
		}
		if r.End.After(end) {
			end = r.End
		}
	}
	var cmds []tea.Cmd
	for i, s := range m.sources {
		s.loading = true
		i, p := i, s.Provider
		cmds = append(cmds, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			lines, err := p.Fetch(ctx, start, end)
			return fetchedMsg{idx: i, lines: lines, err: err, at: time.Now()}
		})
	}
	return tea.Batch(cmds...)
}

func (m Model) numViews() int { return 2 + len(m.sources) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case fetchedMsg:
		s := m.sources[msg.idx]
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
			m.scroll++
		case "up", "k":
			if m.scroll > 0 {
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
	switch {
	case m.view == viewOverview:
		body = m.overview(r)
	case m.view == viewDaily:
		body = m.daily(r)
	default:
		body = m.providerView(m.sources[int(m.view)-2], r)
	}
	frame := lipgloss.JoinVertical(lipgloss.Left, m.header(r), "", body, "", m.footer())
	// Truncate rather than wrap on terminals too small for the layout.
	return lipgloss.NewStyle().MaxWidth(m.width).MaxHeight(m.height).Render(frame)
}

func (m Model) header(r spend.Range) string {
	names := []string{"Overview", "Daily"}
	for _, s := range m.sources {
		names = append(names, s.Provider.Name())
	}
	var tabs []string
	for i, n := range names {
		label := fmt.Sprintf("%d %s", i+1, n)
		if view(i) == m.view {
			tabs = append(tabs, activeTab.Render(label))
		} else {
			tabs = append(tabs, tabStyle.Render(label))
		}
	}
	last := r.End.AddDate(0, 0, -1)
	rng := fmt.Sprintf("◀ %s ▶  %s – %s UTC", r.Label, r.Start.Format("Jan 2"), last.Format("Jan 2"))
	status := ""
	for _, s := range m.sources {
		if s.loading {
			status = m.spin.View() + " refreshing"
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
			status = "updated " + newest.Local().Format("15:04")
		}
	}
	top := titleStyle.Render("spendtui") + "  " + lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
	return lipgloss.JoinVertical(lipgloss.Left, top, boldStyle.Render(rng)+"   "+mutedStyle.Render(status))
}

func (m Model) footer() string {
	return mutedStyle.Render("tab/1-9 view · ←/→ range · j/k scroll · r refresh · q quit")
}

func (m Model) contentWidth() int { return max(m.width-2, 40) }

func (m Model) overview(r spend.Range) string {
	now := m.now()
	var cards []string
	var layers []series
	total, totalProj := 0.0, 0.0
	cardW := max((m.contentWidth()-4*(len(m.sources)+1))/(len(m.sources)+1), 22)
	for _, s := range m.sources {
		sum := spend.Summarize(s.lines, r)
		total += sum.Total
		proj := spend.ProjectMonth(sum.Total, r, now)
		totalProj += proj
		layers = append(layers, series{sum.Daily, s.color})
		cards = append(cards, m.card(s, sum, r, proj, cardW))
	}
	combined := []string{
		boldStyle.Render("Combined"),
		bigNumStyle.Render(usd(total)),
	}
	if totalProj > 0 {
		combined = append(combined, mutedStyle.Render("projected ")+usd(totalProj))
	}
	if r.Days() > 0 {
		combined = append(combined, mutedStyle.Render("avg/day ")+usd(total/float64(r.Days())))
	}
	cards = append(cards, cardStyle.Width(cardW).Render(strings.Join(combined, "\n")))

	legend := make([]string, 0, len(m.sources))
	for _, s := range m.sources {
		legend = append(legend, lipgloss.NewStyle().Foreground(s.color).Render("█ ")+s.Provider.Name())
	}
	chart := columnChart(layers, max(min(m.height-18, 14), 5), m.contentWidth(),
		r.Start.Format("Jan 2"), r.End.AddDate(0, 0, -1).Format("Jan 2"))
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, cards...),
		"",
		boldStyle.Render("Daily spend")+"  "+strings.Join(legend, "  "),
		chart,
	)
}

func (m Model) card(s *source, sum spend.Summary, r spend.Range, proj float64, w int) string {
	name := lipgloss.NewStyle().Bold(true).Foreground(s.color).Render(s.Provider.Name())
	lines := []string{name}
	switch {
	case s.err != nil:
		lines = append(lines, errStyle.Width(w).Render(truncate(s.err.Error(), w*3)))
	case s.loading && s.fetched.IsZero():
		lines = append(lines, m.spin.View()+" loading")
	default:
		lines = append(lines, bigNumStyle.Render(usd(sum.Total)))
		if n := len(sum.Daily); n > 0 && r.End.Equal(spend.Day(m.now()).AddDate(0, 0, 1)) {
			lines = append(lines, mutedStyle.Render("today ")+usd(sum.Daily[n-1]))
		}
		if proj > 0 {
			lines = append(lines, mutedStyle.Render("projected ")+usd(proj))
		}
		if s.Budget > 0 && proj > 0 {
			frac := sum.Total / s.Budget
			color := s.color
			if proj > s.Budget {
				color = warnColor
			}
			lines = append(lines, hbar(frac, w-2, color),
				mutedStyle.Render(fmt.Sprintf("%.0f%% of %s budget", frac*100, usd(s.Budget))))
		}
	}
	return cardStyle.Width(w).Render(strings.Join(lines, "\n"))
}

func (m Model) providerView(s *source, r spend.Range) string {
	if s.err != nil {
		return errStyle.Width(m.contentWidth()).Render(s.err.Error())
	}
	sum := spend.Summarize(s.lines, r)
	peak, peakDay := 0.0, 0
	for i, v := range sum.Daily {
		if v > peak {
			peak, peakDay = v, i
		}
	}
	stats := []string{
		lipgloss.NewStyle().Bold(true).Foreground(s.color).Render(s.Provider.Name()) + "  " + bigNumStyle.Render(usd(sum.Total)),
	}
	if r.Days() > 0 {
		stats = append(stats, mutedStyle.Render(fmt.Sprintf("avg/day %s · peak %s on %s",
			usd(sum.Total/float64(r.Days())), usd(peak), r.Start.AddDate(0, 0, peakDay).Format("Jan 2"))))
	}
	chart := columnChart([]series{{sum.Daily, s.color}}, max(min(m.height-20, 10), 4), m.contentWidth(),
		r.Start.Format("Jan 2"), r.End.AddDate(0, 0, -1).Format("Jan 2"))

	nameW := 32
	barW := max(m.contentWidth()-nameW-40, 6)
	rows := []string{boldStyle.Render(fmt.Sprintf("%-*s %10s %6s %8s %8s  %s", nameW, "Model", "Spend", "Share", "In", "Out", ""))}
	for _, mt := range sum.Models {
		share := 0.0
		if sum.Total > 0 {
			share = mt.USD / sum.Total
		}
		rows = append(rows, fmt.Sprintf("%-*s %10s %5.1f%% %8s %8s  %s",
			nameW, truncate(mt.Model, nameW), usd(mt.USD), share*100,
			tokens(mt.InputTokens), tokens(mt.OutputTokens), hbar(share, barW, s.color)))
	}
	if len(sum.Models) == 0 {
		rows = append(rows, mutedStyle.Render("no spend in this range"))
	}
	rows = m.window(rows, 1, max(m.height-18-strings.Count(chart, "\n"), 3))
	return lipgloss.JoinVertical(lipgloss.Left, strings.Join(stats, "\n"), "", chart, "", strings.Join(rows, "\n"))
}

func (m Model) daily(r spend.Range) string {
	sums := make([]spend.Summary, len(m.sources))
	maxTotal := 0.0
	for i, s := range m.sources {
		sums[i] = spend.Summarize(s.lines, r)
	}
	n := r.Days()
	totals := make([]float64, n)
	for d := 0; d < n; d++ {
		for _, sum := range sums {
			totals[d] += sum.Daily[d]
		}
		maxTotal = max(maxTotal, totals[d])
	}
	head := fmt.Sprintf("%-12s", "Date")
	for _, s := range m.sources {
		head += fmt.Sprintf(" %11s", truncate(s.Provider.Name(), 11))
	}
	head += fmt.Sprintf(" %11s", "Total")
	barW := max(m.contentWidth()-len(head)-2, 6)
	rows := []string{boldStyle.Render(head)}
	for d := n - 1; d >= 0; d-- {
		row := fmt.Sprintf("%-12s", r.Start.AddDate(0, 0, d).Format("Mon Jan 02"))
		for _, sum := range sums {
			row += fmt.Sprintf(" %11s", usd(sum.Daily[d]))
		}
		frac := 0.0
		if maxTotal > 0 {
			frac = totals[d] / maxTotal
		}
		rows = append(rows, row+boldStyle.Render(fmt.Sprintf(" %11s", usd(totals[d])))+"  "+hbar(frac, barW, accentColor))
	}
	return strings.Join(m.window(rows, 1, max(m.height-8, 5)), "\n")
}

// window keeps the first `keep` rows (headers) and shows `size` of the rest
// starting at the scroll offset.
func (m Model) window(rows []string, keep, size int) []string {
	body := rows[keep:]
	off := min(m.scroll, max(len(body)-size, 0))
	end := min(off+size, len(body))
	out := append([]string{}, rows[:keep]...)
	out = append(out, body[off:end]...)
	if end < len(body) {
		out = append(out, mutedStyle.Render(fmt.Sprintf("… %d more (j/k to scroll)", len(body)-end)))
	}
	return out
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
