// Command spendtui is a terminal dashboard for Claude API and Fireworks AI
// spend, backed by a local DuckDB history.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/anthropic"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/demo"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/fireworks"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/store"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/ui"
)

const day = 24 * time.Hour

func main() {
	var (
		demoMode     = flag.Bool("demo", false, "use generated data (in-memory database) instead of calling the APIs")
		dbPath       = flag.String("db", store.DefaultPath(), "DuckDB file holding spend history")
		backfill     = flag.Int("backfill-days", 180, "how far back the first sync reaches")
		claudeBudget = flag.Float64("claude-budget", envFloat("SPENDTUI_CLAUDE_BUDGET"), "monthly Claude budget in USD (env SPENDTUI_CLAUDE_BUDGET)")
		fwBudget     = flag.Float64("fireworks-budget", envFloat("SPENDTUI_FIREWORKS_BUDGET"), "monthly Fireworks budget in USD (env SPENDTUI_FIREWORKS_BUDGET)")
		every        = flag.Duration("refresh", 5*time.Minute, "auto-refresh interval (0 disables)")
		rangeNum     = flag.Int("range", 1, "initial range: 1 month to date, 2 last 7 days, 3 last 30 days, 4 last month, 5 last 90 days")
		snapshot     = flag.String("snapshot", "", "print one frame of the named view (overview, weekly, models, daily, claude, fireworks) and exit")
		width        = flag.Int("width", 120, "terminal width for --snapshot")
		height       = flag.Int("height", 40, "terminal height for --snapshot")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `spendtui — Claude + Fireworks spend in your terminal

Environment:
  ANTHROPIC_ADMIN_KEY    Anthropic Admin API key (sk-ant-admin...)
  FIREWORKS_API_KEY      Fireworks API key
  FIREWORKS_ACCOUNT_ID   Fireworks account id (as shown by `+"`firectl whoami`"+`)

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	var providers []spend.Provider
	budgets := map[string]float64{}
	if *demoMode {
		*dbPath = "" // in-memory: never mix demo rows into real history
		providers = []spend.Provider{demo.Claude(), demo.Fireworks()}
		budgets["Claude"], budgets["Fireworks"] = or(*claudeBudget, 400), or(*fwBudget, 250)
	} else {
		if k := os.Getenv("ANTHROPIC_ADMIN_KEY"); k != "" {
			providers = append(providers, anthropic.New(k))
			budgets["Claude"] = *claudeBudget
		}
		if k := os.Getenv("FIREWORKS_API_KEY"); k != "" {
			providers = append(providers, fireworks.New(k, os.Getenv("FIREWORKS_ACCOUNT_ID")))
			budgets["Fireworks"] = *fwBudget
		}
	}
	if len(providers) == 0 {
		fail(2, "no providers configured. Set ANTHROPIC_ADMIN_KEY and/or FIREWORKS_API_KEY + FIREWORKS_ACCOUNT_ID, or run with --demo.")
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		fail(1, err.Error())
	}
	defer st.Close()

	var srcs []ui.Source
	for _, p := range providers {
		sy := &store.Syncer{Store: st, Provider: p, Backfill: time.Duration(*backfill) * day, Restate: 7, Keep: 400 * day}
		srcs = append(srcs, ui.Source{Name: p.Name(), Budget: budgets[p.Name()], Cached: sy.Cached, Sync: sy.Sync})
	}

	if *snapshot != "" {
		// Piped output isn't a TTY, so termenv would downgrade colors; honor
		// an explicit truecolor request (handy for screenshots).
		if os.Getenv("COLORTERM") == "truecolor" {
			lipgloss.SetColorProfile(termenv.TrueColor)
		}
		out, err := ui.Snapshot(srcs, *snapshot, *rangeNum-1, *width, *height)
		if err != nil {
			fail(2, err.Error())
		}
		fmt.Println(out)
		return
	}
	if _, err := tea.NewProgram(ui.New(srcs, *every, *rangeNum-1), tea.WithAltScreen()).Run(); err != nil {
		fail(1, err.Error())
	}
}

func fail(code int, msg string) {
	fmt.Fprintln(os.Stderr, "spendtui:", msg)
	os.Exit(code)
}

func envFloat(key string) float64 {
	v, _ := strconv.ParseFloat(os.Getenv(key), 64)
	return v
}

func or(v, fallback float64) float64 {
	if v > 0 {
		return v
	}
	return fallback
}
