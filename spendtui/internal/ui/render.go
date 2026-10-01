package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	claudeColor    = lipgloss.AdaptiveColor{Light: "#C15F3C", Dark: "#D97757"}
	fireworksColor = lipgloss.AdaptiveColor{Light: "#6D28D9", Dark: "#A78BFA"}
	mutedColor     = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9CA3AF"}
	accentColor    = lipgloss.AdaptiveColor{Light: "#0F766E", Dark: "#5EEAD4"}
	warnColor      = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FBBF24"}
	errColor       = lipgloss.AdaptiveColor{Light: "#B91C1C", Dark: "#F87171"}

	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	mutedStyle  = lipgloss.NewStyle().Foreground(mutedColor)
	errStyle    = lipgloss.NewStyle().Foreground(errColor)
	boldStyle   = lipgloss.NewStyle().Bold(true)
	tabStyle    = lipgloss.NewStyle().Padding(0, 1).Foreground(mutedColor)
	activeTab   = lipgloss.NewStyle().Padding(0, 1).Bold(true).Foreground(accentColor).Underline(true)
	cardStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(mutedColor).Padding(0, 1)
	bigNumStyle = lipgloss.NewStyle().Bold(true)
)

func usd(v float64) string {
	switch {
	case v >= 10000:
		return fmt.Sprintf("$%s", commas(int64(math.Round(v))))
	default:
		return fmt.Sprintf("$%s.%02d", commas(int64(v)), int64(math.Round(v*100))%100)
	}
}

func commas(n int64) string {
	s := fmt.Sprint(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if neg {
		return "-" + s
	}
	return s
}

func tokens(n int64) string {
	switch {
	case n == 0:
		return "—"
	case n >= 1e9:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1e6:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1e3:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

// hbar renders a horizontal bar of the given width filled to frac.
func hbar(frac float64, width int, color lipgloss.TerminalColor) string {
	if width <= 0 {
		return ""
	}
	frac = math.Max(0, math.Min(1, frac))
	eighths := int(math.Round(frac * float64(width) * 8))
	full, rem := eighths/8, eighths%8
	partials := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	bar := strings.Repeat("█", full)
	used := full
	if rem > 0 && full < width {
		bar += partials[rem]
		used++
	}
	return lipgloss.NewStyle().Foreground(color).Render(bar) +
		mutedStyle.Render(strings.Repeat("·", width-used))
}

// series is one stacked layer of a column chart.
type series struct {
	values []float64
	color  lipgloss.TerminalColor
}

// columnChart draws stacked daily columns, oldest on the left. labels are
// placed under the first and last column.
func columnChart(layers []series, height, width int, firstLabel, lastLabel string) string {
	if len(layers) == 0 || len(layers[0].values) == 0 {
		return ""
	}
	n := len(layers[0].values)
	totals := make([]float64, n)
	maxV := 0.0
	for _, l := range layers {
		for i, v := range l.values {
			totals[i] += v
		}
	}
	for _, t := range totals {
		maxV = math.Max(maxV, t)
	}
	axisW := len(usd(maxV)) + 1
	colW, gap := 1, 0
	avail := width - axisW - 1
	for _, cw := range []struct{ w, g int }{{3, 1}, {2, 1}, {1, 1}, {1, 0}} {
		if n*(cw.w+cw.g) <= avail {
			colW, gap = cw.w, cw.g
			break
		}
	}
	if n*(colW+gap) > avail { // too many days for the terminal: keep the newest
		drop := n - avail/(colW+gap)
		for i := range layers {
			layers[i].values = layers[i].values[drop:]
		}
		totals = totals[drop:]
		n -= drop
		firstLabel = ""
	}

	// Per column, the cell row (from the bottom) each layer occupies up to.
	cells := make([][]int, len(layers))
	for li := range layers {
		cells[li] = make([]int, n)
	}
	for i := 0; i < n; i++ {
		if maxV == 0 {
			break
		}
		acc := 0.0
		prev := 0
		for li, l := range layers {
			acc += l.values[i]
			h := int(math.Round(acc / maxV * float64(height)))
			if l.values[i] > 0 && h == prev { // keep tiny but nonzero days visible
				h = prev + 1
			}
			cells[li][i] = h
			prev = h
		}
	}

	var b strings.Builder
	for row := height; row >= 1; row-- {
		label := ""
		switch row {
		case height:
			label = usd(maxV)
		case (height + 1) / 2:
			label = usd(maxV / 2)
		}
		b.WriteString(mutedStyle.Render(fmt.Sprintf("%*s ", axisW-1, label)))
		b.WriteString(mutedStyle.Render("│"))
		for i := 0; i < n; i++ {
			cell := strings.Repeat(" ", colW)
			for li := range layers {
				if row <= cells[li][i] {
					cell = lipgloss.NewStyle().Foreground(layers[li].color).Render(strings.Repeat("█", colW))
					break
				}
			}
			b.WriteString(cell + strings.Repeat(" ", gap))
		}
		b.WriteString("\n")
	}
	plotW := n * (colW + gap)
	b.WriteString(strings.Repeat(" ", axisW) + mutedStyle.Render("└"+strings.Repeat("─", plotW)) + "\n")
	pad := plotW - len(firstLabel) - len(lastLabel)
	if pad < 1 {
		firstLabel, pad = "", plotW-len(lastLabel)
	}
	b.WriteString(strings.Repeat(" ", axisW+1) + mutedStyle.Render(firstLabel+strings.Repeat(" ", max(pad, 0))+lastLabel))
	return b.String()
}
