# spendtui

A terminal dashboard for your **Claude API** and **Fireworks AI** spend, built
with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

```
spendtui   1 Overview  2 Daily  3 Claude  4 Fireworks
◀ Month to date ▶  Sep 1 – Sep 18 UTC   updated 12:00

╭──────────────────────────╮╭──────────────────────────╮╭──────────────────────────╮
│ Claude                   ││ Fireworks                ││ Combined                 │
│ $225.57                  ││ $129.88                  ││ $355.45                  │
│ today $14.69             ││ today $8.07              ││ projected $592.42        │
│ projected $375.95        ││ projected $216.47        ││ avg/day $19.75           │
│ ██████████████▋········· │╰──────────────────────────╯╰──────────────────────────╯
│ 56% of $400.00 budget    │
╰──────────────────────────╯

Daily spend  █ Claude  █ Fireworks
$25.10 │███ ███ ███ ███         ███
       │███ ███ ███ ███         ███     ███                 ███ ███ ███ ███ ███
$12.55 │███ ███ ███ ███ ███ ███ ███ ███ ███ ███ ███ ███ ███ ███ ███ ███ ███ ███
       └────────────────────────────────────────────────────────────────────────
        Sep 1                                                             Sep 18
```

## Views

| Key | View | What it shows |
|---|---|---|
| `1` | Overview | One card per provider (total, today, month-end projection, budget bar) and a combined card, over a stacked daily column chart |
| `2` | Daily | Day-by-day table, newest first, with per-provider columns and a total bar |
| `3` | Claude | Daily chart, plus a per-model breakdown with spend, share and token counts |
| `4` | Fireworks | The same, per model / dedicated deployment / training job |

Other keys: `←`/`→` (or `h`/`l`) cycle the range (Month to date, Last 7 days,
Last 30 days, Last month, Last 90 days), `j`/`k` scroll, `r` refresh, `q` quit.

All dates are UTC days, which is how both providers bucket billing.

## Install

```fish
go install github.com/sam-phinizy/sams-claude-menagerie/spendtui@latest
```

or from a checkout: `cd spendtui; go build .`

Try it without keys: `spendtui --demo`

## Configure

```fish
# Claude: needs an *Admin* API key (Console → Settings → Admin keys).
# Regular sk-ant-api keys are rejected by the cost endpoint.
set -Ux ANTHROPIC_ADMIN_KEY sk-ant-admin01-...

# Fireworks: an API key plus your account id (`firectl whoami` shows it).
set -Ux FIREWORKS_API_KEY fw_...
set -Ux FIREWORKS_ACCOUNT_ID my-account

# Optional monthly budgets in USD; they drive the budget bars on the overview.
set -Ux SPENDTUI_CLAUDE_BUDGET 400
set -Ux SPENDTUI_FIREWORKS_BUDGET 250
```

Configure either provider or both; whichever has credentials shows up.

Flags: `--claude-budget`, `--fireworks-budget` (override the env vars),
`--refresh 5m` (auto-refresh interval; `0` disables), `--demo`.

## Where the numbers come from

| Provider | Endpoint | Notes |
|---|---|---|
| Claude | `GET /v1/organizations/cost_report` (Anthropic Admin API) | Daily buckets grouped by `description`, so each line carries its model. Amounts are USD cents. Web search and code execution show up as their own rows. Priority Tier costs are **not** included in this endpoint. The Admin API isn't available to individual (non-organization) accounts. |
| Fireworks | `GET /v1/accounts/{account}/billingUsage` | Daily buckets across `serverlessCosts`, `dedicatedCosts` and `trainingCosts`; cost is `costNanoUsd`. Windows are capped at 31 days, so longer ranges are fetched in chunks. Fireworks reports `0` cost when a line has no authoritative rate yet, which doesn't mean it was free. |

On each refresh, spendtui fetches one window that covers every selectable range
(back to the start of last month or 90 days ago, whichever is earlier), so
switching ranges doesn't trigger a refetch. Both APIs lag real usage by a few
minutes.

The month-end projection is month-to-date spend ÷ days elapsed (today counts
as a full day) × days in the month. It only appears on the Month to date range.

## Layout

```
main.go                     flags, env, provider wiring
internal/spend              provider-neutral Line type, ranges, rollups, projection
internal/anthropic          Admin API cost_report client
internal/fireworks          billingUsage client
internal/demo               deterministic fake providers for --demo
internal/ui                 Bubble Tea model, views, charts
```

Adding another provider means implementing `spend.Provider`
(`Name()` + `Fetch(ctx, start, end) ([]spend.Line, error)`) and appending it
in `main.go`.
