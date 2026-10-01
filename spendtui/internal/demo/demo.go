// Package demo provides deterministic fake providers so the TUI can be tried
// without API keys.
package demo

import (
	"context"
	"hash/fnv"
	"math"
	"time"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

type model struct {
	name       string
	dailyUSD   float64
	usdPerMTok float64 // blended, used to back out token counts
}

type Provider struct {
	name   string
	models []model
}

func Claude() *Provider {
	return &Provider{"Claude", []model{
		{"claude-opus-5-5", 9.5, 8},
		{"claude-sonnet-5-5", 4.2, 4},
		{"claude-haiku-4-5", 0.8, 2},
		{"web_search", 0.3, 0},
	}}
}

func Fireworks() *Provider {
	return &Provider{"Fireworks", []model{
		{"deepseek-v3p1", 2.6, 1.2},
		{"llama-v3p3-70b-instruct", 1.1, 0.9},
		{"qwen3-coder-480b-a35b-instruct", 1.7, 1.5},
		{"dedicated: kimi-k2-instruct", 6.0, 0},
	}}
}

func (p *Provider) Name() string { return p.name }

func (p *Provider) Fetch(ctx context.Context, start, end time.Time) ([]spend.Line, error) {
	var lines []spend.Line
	today := spend.Day(time.Now())
	for d := spend.Day(start); d.Before(end) && !d.After(today); d = d.AddDate(0, 0, 1) {
		for _, m := range p.models {
			usd := m.dailyUSD * jitter(p.name+m.name+d.Format("2006-01-02"))
			if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
				usd *= 0.35
			}
			usd = math.Round(usd*100) / 100
			var t spend.Tokens
			if m.usdPerMTok > 0 {
				tok := usd / m.usdPerMTok * 1e6
				hit := 0.25 + 0.5*(jitter("cache"+m.name)-0.3)/1.4 // stable per model, 25–75%
				t.CacheRead = int64(tok * 3 * hit)                 // cache reads are cheap, so there are many
				t.CacheWrite = int64(tok * 0.05)
				t.Input = int64(tok*3*(1-hit)) - t.CacheWrite
				t.Output = int64(tok * 0.15)
			}
			lines = append(lines, spend.Line{Day: d, Model: m.name, USD: usd, Tokens: t})
		}
	}
	return lines, nil
}

// jitter maps a key to a stable multiplier in [0.3, 1.7).
func jitter(key string) float64 {
	h := fnv.New32a()
	h.Write([]byte(key))
	return 0.3 + float64(h.Sum32()%1400)/1000
}
