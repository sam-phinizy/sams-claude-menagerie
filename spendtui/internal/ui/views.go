package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

func (m Model) overview(r spend.Range) string {
	now := m.now()
	var cards []string
	var layers []series
	total, totalProj := 0.0, 0.0
	var all []spend.Line
	cardW := max((m.contentWidth()-4*(len(m.sources)+1))/(len(m.sources)+1), 22)
	for _, s := range m.sources {
		sum := spend.Summarize(s.lines, r)
		total += sum.Total
		proj := spend.ProjectMonth(sum.Total, r, now)
		totalProj += proj
		all = append(all, s.lines...)
		layers = append(layers, series{sum.Daily, s.color})
		cards = append(cards, m.card(s, sum, r, proj, cardW))
	}
	last7, prior7 := spend.Trailing(all, now)
	combined := []string{
		boldStyle.Render("Combined"),
		bigNumStyle.Render(usd(total)),
	}
	if totalProj > 0 {
		combined = append(combined, mutedStyle.Render("projected ")+usd(totalProj))
	}
	combined = append(combined, mutedStyle.Render("7d ")+usd(last7)+" "+delta(last7, prior7))
	cards = append(cards, cardStyle.Width(cardW).Render(strings.Join(combined, "\n")))

	chart := columnChart(layers, max(min(m.height-19, 14), 4), m.contentWidth(),
		r.Start.Format("Jan 2"), r.End.AddDate(0, 0, -1).Format("Jan 2"))
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.JoinHorizontal(lipgloss.Top, cards...),
		"",
		boldStyle.Render("Daily spend")+"  "+m.legend(),
		chart,
	)
}

func (m Model) legend() string {
	parts := make([]string, 0, len(m.sources))
	for _, s := range m.sources {
		parts = append(parts, lipgloss.NewStyle().Foreground(s.color).Render("█ ")+s.Name)
	}
	return strings.Join(parts, "  ")
}

func (m Model) card(s *source, sum spend.Summary, r spend.Range, proj float64, w int) string {
	name := lipgloss.NewStyle().Bold(true).Foreground(s.color).Render(s.Name)
	lines := []string{name}
	if s.loading && s.fetched.IsZero() {
		lines = append(lines, m.spin.View()+" loading")
	} else if !s.fetched.IsZero() {
		lines = append(lines, bigNumStyle.Render(usd(sum.Total)))
		if n := len(sum.Daily); n > 0 && r.End.Equal(spend.Day(m.now()).AddDate(0, 0, 1)) {
			lines = append(lines, mutedStyle.Render("today ")+usd(sum.Daily[n-1]))
		}
		if proj > 0 {
			lines = append(lines, mutedStyle.Render("projected ")+usd(proj))
		}
		last7, prior7 := spend.Trailing(s.lines, m.now())
		lines = append(lines, mutedStyle.Render("7d ")+usd(last7)+" "+delta(last7, prior7))
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
	if s.err != nil {
		lines = append(lines, errStyle.Width(w-2).Render(truncate(s.err.Error(), w*2)))
	}
	return cardStyle.Width(w).Render(strings.Join(lines, "\n"))
}

// weekly compares calendar weeks per provider.
func (m Model) weekly() string {
	now := m.now()
	weeks := 12
	var all []spend.Line
	perSource := make([][]float64, len(m.sources))
	var cards []string
	cardW := max((m.contentWidth()-4*(len(m.sources)+1))/(len(m.sources)+1), 22)
	for i, s := range m.sources {
		all = append(all, s.lines...)
		perSource[i] = spend.Weekly(s.lines, weeks, now)
		last7, prior7 := spend.Trailing(s.lines, now)
		cards = append(cards, cardStyle.Width(cardW).Render(strings.Join([]string{
			lipgloss.NewStyle().Bold(true).Foreground(s.color).Render(s.Name),
			bigNumStyle.Render(usd(last7)) + " " + delta(last7, prior7),
			mutedStyle.Render("last 7d vs prior ") + usd(prior7),
			sparkline(perSource[i], s.color),
		}, "\n")))
	}
	last7, prior7 := spend.Trailing(all, now)
	totals := spend.Weekly(all, weeks, now)
	cards = append(cards, cardStyle.Width(cardW).Render(strings.Join([]string{
		boldStyle.Render("Combined"),
		bigNumStyle.Render(usd(last7)) + " " + delta(last7, prior7),
		mutedStyle.Render("last 7d vs prior ") + usd(prior7),
		sparkline(totals, accentColor),
	}, "\n")))

	head := fmt.Sprintf("%-18s", "Week of")
	for _, s := range m.sources {
		head += fmt.Sprintf(" %11s", truncate(s.Name, 11))
	}
	head += fmt.Sprintf(" %11s %8s", "Total", "WoW")
	barW := max(m.contentWidth()-len(head)-2, 6)
	maxT := 0.0
	for _, t := range totals {
		maxT = max(maxT, t)
	}
	rows := []string{boldStyle.Render(head)}
	ws := spend.WeekStart(now)
	for w := weeks - 1; w >= 0; w-- {
		label := ws.AddDate(0, 0, -7*(weeks-1-w)).Format("Mon Jan 02")
		if w == weeks-1 {
			label += " (wtd)"
		}
		row := fmt.Sprintf("%-18s", label)
		for i := range m.sources {
			row += fmt.Sprintf(" %11s", usd(perSource[i][w]))
		}
		d := mutedStyle.Render(fmt.Sprintf("%8s", "—"))
		if w > 0 && w < weeks-1 { // the partial week isn't comparable to a full one
			d = padLeft(delta(totals[w], totals[w-1]), 8)
		}
		frac := 0.0
		if maxT > 0 {
			frac = totals[w] / maxT
		}
		rows = append(rows, row+boldStyle.Render(fmt.Sprintf(" %11s", usd(totals[w])))+" "+d+"  "+hbar(frac, barW, accentColor))
	}
	cardsBlock := lipgloss.JoinHorizontal(lipgloss.Top, cards...)
	avail := m.height - 8 - lipgloss.Height(cardsBlock)
	return lipgloss.JoinVertical(lipgloss.Left, cardsBlock, "", strings.Join(m.window(rows, 1, max(avail, 3)), "\n"))
}

type modelRow struct {
	src           *source
	model         string
	lines         []spend.Line
	inRange       float64
	last7, prior7 float64
	weekly        []float64
}

// models lists every model across providers with week-over-week movement,
// and charts the selected one.
func (m Model) models(r spend.Range) string {
	now := m.now()
	var rows []modelRow
	for _, s := range m.sources {
		for _, name := range spend.Models(s.lines) {
			ls := spend.ForModel(s.lines, name)
			mr := modelRow{src: s, model: name, lines: ls, weekly: spend.Weekly(ls, 12, now)}
			mr.inRange = spend.Between(ls, r.Start, r.End)
			mr.last7, mr.prior7 = spend.Trailing(ls, now)
			if mr.inRange == 0 && mr.last7 == 0 && mr.prior7 == 0 {
				continue
			}
			rows = append(rows, mr)
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].inRange != rows[j].inRange {
			return rows[i].inRange > rows[j].inRange
		}
		return rows[i].last7 > rows[j].last7
	})
	if len(rows) == 0 {
		return mutedStyle.Render("no spend recorded yet")
	}
	cursor := min(m.cursor, len(rows)-1)

	nameW := max(min(m.contentWidth()-71, 34), 16)
	// Four leading columns: cursor mark (2), provider dot, space.
	lines := []string{boldStyle.Render(fmt.Sprintf("    %-*s %12s %10s %10s %8s  %s",
		nameW, "Model", truncate(r.Label, 12), "Last 7d", "Prior 7d", "WoW", "12 weeks"))}
	for i, mr := range rows {
		mark := "  "
		name := fmt.Sprintf("%-*s", nameW, truncate(mr.model, nameW))
		if i == cursor {
			mark = lipgloss.NewStyle().Foreground(accentColor).Render("▶ ")
			name = boldStyle.Render(name)
		}
		dot := lipgloss.NewStyle().Foreground(mr.src.color).Render("●")
		lines = append(lines, fmt.Sprintf("%s%s %s %12s %10s %10s %s  %s",
			mark, dot, name, usd(mr.inRange), usd(mr.last7), usd(mr.prior7),
			padLeft(delta(mr.last7, mr.prior7), 8), sparkline(mr.weekly, mr.src.color)))
	}

	sel := rows[cursor]
	sum := spend.Summarize(sel.lines, r)
	detailTitle := lipgloss.NewStyle().Bold(true).Foreground(sel.src.color).Render(sel.model) +
		mutedStyle.Render(fmt.Sprintf("  %s · %s in range", sel.src.Name, usd(sum.Total)))
	if ts := tokenSummary(sum.Tokens); ts != "" {
		detailTitle += mutedStyle.Render(" · " + ts)
	}
	chartH := max(min(m.height-len(rows)-14, 8), 3)
	chart := columnChart([]series{{sum.Daily, sel.src.color}}, chartH, m.contentWidth(),
		r.Start.Format("Jan 2"), r.End.AddDate(0, 0, -1).Format("Jan 2"))

	tableRows := max(m.height-10-chartH-3, 3)
	// Keep the cursor in view.
	off := max(0, cursor-tableRows+1)
	mm := m
	mm.scroll = off
	return lipgloss.JoinVertical(lipgloss.Left,
		strings.Join(mm.window(lines, 1, tableRows), "\n"), "", detailTitle, chart)
}

func (m Model) providerView(s *source, r spend.Range) string {
	sum := spend.Summarize(s.lines, r)
	peak, peakDay := 0.0, 0
	for i, v := range sum.Daily {
		if v > peak {
			peak, peakDay = v, i
		}
	}
	stats := []string{
		lipgloss.NewStyle().Bold(true).Foreground(s.color).Render(s.Name) + "  " + bigNumStyle.Render(usd(sum.Total)),
	}
	if r.Days() > 0 {
		stats = append(stats, mutedStyle.Render(fmt.Sprintf("avg/day %s · peak %s on %s",
			usd(sum.Total/float64(r.Days())), usd(peak), r.Start.AddDate(0, 0, peakDay).Format("Jan 2"))))
	}
	if ts := tokenSummary(sum.Tokens); ts != "" {
		stats = append(stats, mutedStyle.Render("tokens: "+ts))
	}
	if s.err != nil {
		stats = append(stats, errStyle.Width(m.contentWidth()).Render(s.err.Error()))
	}
	chart := columnChart([]series{{sum.Daily, s.color}}, max(min(m.height-21, 10), 4), m.contentWidth(),
		r.Start.Format("Jan 2"), r.End.AddDate(0, 0, -1).Format("Jan 2"))

	nameW := 30
	barW := max(m.contentWidth()-nameW-62, 6)
	rows := []string{boldStyle.Render(fmt.Sprintf("%-*s %10s %6s %8s %8s %8s %8s %5s  %s",
		nameW, "Model", "Spend", "Share", "Input", "Cache R", "Cache W", "Output", "Hit", ""))}
	for _, mt := range sum.Models {
		share := 0.0
		if sum.Total > 0 {
			share = mt.USD / sum.Total
		}
		rows = append(rows, fmt.Sprintf("%-*s %10s %5.1f%% %8s %8s %8s %8s %5s  %s",
			nameW, truncate(mt.Model, nameW), usd(mt.USD), share*100,
			tokens(mt.Input), tokens(mt.CacheRead), tokens(mt.CacheWrite), tokens(mt.Output),
			hitRate(mt.Tokens), hbar(share, barW, s.color)))
	}
	if len(sum.Models) == 0 {
		rows = append(rows, mutedStyle.Render("no spend in this range"))
	}
	rows = m.window(rows, 1, max(m.height-19-strings.Count(chart, "\n"), 3))
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
		head += fmt.Sprintf(" %11s", truncate(s.Name, 11))
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

func padLeft(s string, w int) string {
	if pad := w - lipgloss.Width(s); pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}
