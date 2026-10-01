// Command spendtui is a terminal dashboard for Claude API and Fireworks AI
// spend.
package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/anthropic"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/demo"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/fireworks"
	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/ui"
)

func main() {
	var (
		demoMode     = flag.Bool("demo", false, "use generated data instead of calling the APIs")
		claudeBudget = flag.Float64("claude-budget", envFloat("SPENDTUI_CLAUDE_BUDGET"), "monthly Claude budget in USD (env SPENDTUI_CLAUDE_BUDGET)")
		fwBudget     = flag.Float64("fireworks-budget", envFloat("SPENDTUI_FIREWORKS_BUDGET"), "monthly Fireworks budget in USD (env SPENDTUI_FIREWORKS_BUDGET)")
		every        = flag.Duration("refresh", 5*time.Minute, "auto-refresh interval (0 disables)")
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

	var srcs []ui.Source
	if *demoMode {
		srcs = []ui.Source{
			{Provider: demo.Claude(), Budget: or(*claudeBudget, 400)},
			{Provider: demo.Fireworks(), Budget: or(*fwBudget, 250)},
		}
	} else {
		if k := os.Getenv("ANTHROPIC_ADMIN_KEY"); k != "" {
			srcs = append(srcs, ui.Source{Provider: anthropic.New(k), Budget: *claudeBudget})
		}
		if k := os.Getenv("FIREWORKS_API_KEY"); k != "" {
			srcs = append(srcs, ui.Source{Provider: fireworks.New(k, os.Getenv("FIREWORKS_ACCOUNT_ID")), Budget: *fwBudget})
		}
	}
	if len(srcs) == 0 {
		fmt.Fprintln(os.Stderr, "spendtui: no providers configured. Set ANTHROPIC_ADMIN_KEY and/or FIREWORKS_API_KEY + FIREWORKS_ACCOUNT_ID, or run with --demo.")
		os.Exit(2)
	}

	if _, err := tea.NewProgram(ui.New(srcs, *every), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "spendtui:", err)
		os.Exit(1)
	}
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
