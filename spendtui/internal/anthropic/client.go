// Package anthropic reads Claude spend and token usage from the Anthropic
// Admin API: dollars from GET /v1/organizations/cost_report and tokens from
// GET /v1/organizations/usage_report/messages.
//
// Both endpoints need an organization-level credential: an Admin API key
// (sk-ant-admin...), a personal or service-account key that isn't scoped to a
// workspace, or an OAuth token with the org:admin scope. Workspace API keys
// are rejected, and individual (non-organization) accounts have no access.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

const defaultBaseURL = "https://api.anthropic.com"

// Auth returns the header that authenticates one request.
type Auth func(ctx context.Context) (name, value string, err error)

// APIKey authenticates with x-api-key: an Admin API key, or an organization-
// scoped personal or service-account key.
func APIKey(key string) Auth {
	return func(context.Context) (string, string, error) { return "x-api-key", key, nil }
}

// Bearer authenticates with a fixed org:admin OAuth token. These are
// short-lived; prefer BearerCommand for a long-running session.
func Bearer(token string) Auth {
	return func(context.Context) (string, string, error) { return "authorization", "Bearer " + token, nil }
}

// BearerCommand runs a shell command for a fresh OAuth token on each request,
// e.g. `ant auth print-credentials --profile admin --access-token`.
func BearerCommand(command string) Auth {
	return func(ctx context.Context) (string, string, error) {
		out, err := exec.CommandContext(ctx, "sh", "-c", command).Output()
		if err != nil {
			return "", "", fmt.Errorf("claude: token command: %w", err)
		}
		tok := strings.TrimSpace(string(out))
		if tok == "" {
			return "", "", fmt.Errorf("claude: token command printed nothing")
		}
		return "authorization", "Bearer " + tok, nil
	}
}

type Client struct {
	Auth    Auth
	BaseURL string
	HTTP    *http.Client
}

func New(auth Auth) *Client {
	return &Client{Auth: auth, BaseURL: defaultBaseURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "Claude" }

type costReport struct {
	Data []struct {
		StartingAt time.Time `json:"starting_at"`
		Results    []struct {
			Amount      string  `json:"amount"` // USD cents, decimal string
			Description *string `json:"description"`
			Model       *string `json:"model"`
			CostType    *string `json:"cost_type"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool    `json:"has_more"`
	NextPage *string `json:"next_page"`
}

type usageReport struct {
	Data []struct {
		StartingAt time.Time `json:"starting_at"`
		Results    []struct {
			Model         *string `json:"model"`
			Uncached      int64   `json:"uncached_input_tokens"`
			CacheRead     int64   `json:"cache_read_input_tokens"`
			Output        int64   `json:"output_tokens"`
			CacheCreation struct {
				OneHour  int64 `json:"ephemeral_1h_input_tokens"`
				FiveMins int64 `json:"ephemeral_5m_input_tokens"`
			} `json:"cache_creation"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool    `json:"has_more"`
	NextPage *string `json:"next_page"`
}

// Fetch returns cost lines (dollars only) and usage lines (tokens only) keyed
// by the same model id; the store and rollups add them together.
func (c *Client) Fetch(ctx context.Context, start, end time.Time) ([]spend.Line, error) {
	costs, err := c.costs(ctx, start, end)
	if err != nil {
		return nil, err
	}
	usage, err := c.usage(ctx, start, end)
	if err != nil {
		return nil, err
	}
	return append(costs, usage...), nil
}

func (c *Client) costs(ctx context.Context, start, end time.Time) ([]spend.Line, error) {
	var lines []spend.Line
	err := c.paginate(ctx, "/v1/organizations/cost_report", start, end, "description", func(body []byte) (*string, bool, error) {
		var rep costReport
		if err := json.Unmarshal(body, &rep); err != nil {
			return nil, false, fmt.Errorf("claude: decoding cost report: %w", err)
		}
		for _, b := range rep.Data {
			for _, r := range b.Results {
				cents, err := strconv.ParseFloat(r.Amount, 64)
				if err != nil {
					return nil, false, fmt.Errorf("claude: bad amount %q: %w", r.Amount, err)
				}
				lines = append(lines, spend.Line{
					Day:   spend.Day(b.StartingAt),
					Model: label(r.Model, r.CostType, r.Description),
					USD:   cents / 100,
				})
			}
		}
		return rep.NextPage, rep.HasMore, nil
	})
	return lines, err
}

func (c *Client) usage(ctx context.Context, start, end time.Time) ([]spend.Line, error) {
	var lines []spend.Line
	err := c.paginate(ctx, "/v1/organizations/usage_report/messages", start, end, "model", func(body []byte) (*string, bool, error) {
		var rep usageReport
		if err := json.Unmarshal(body, &rep); err != nil {
			return nil, false, fmt.Errorf("claude: decoding usage report: %w", err)
		}
		for _, b := range rep.Data {
			for _, r := range b.Results {
				t := spend.Tokens{
					Input:      r.Uncached,
					CacheRead:  r.CacheRead,
					CacheWrite: r.CacheCreation.OneHour + r.CacheCreation.FiveMins,
					Output:     r.Output,
				}
				if t == (spend.Tokens{}) {
					continue
				}
				lines = append(lines, spend.Line{Day: spend.Day(b.StartingAt), Model: label(r.Model, nil, nil), Tokens: t})
			}
		}
		return rep.NextPage, rep.HasMore, nil
	})
	return lines, err
}

// paginate walks a report in daily buckets, 31 per page (the API maximum).
func (c *Client) paginate(ctx context.Context, path string, start, end time.Time, groupBy string,
	page func(body []byte) (next *string, more bool, err error)) error {
	cursor := ""
	for {
		q := url.Values{}
		q.Set("starting_at", start.UTC().Format(time.RFC3339))
		q.Set("ending_at", end.UTC().Format(time.RFC3339))
		q.Set("bucket_width", "1d")
		q.Add("group_by[]", groupBy)
		q.Set("limit", "31")
		if cursor != "" {
			q.Set("page", cursor)
		}
		body, err := c.get(ctx, path+"?"+q.Encode())
		if err != nil {
			return err
		}
		next, more, err := page(body)
		if err != nil {
			return err
		}
		if !more || next == nil || *next == "" {
			return nil
		}
		cursor = *next
	}
}

// label picks the grouping key for a line: the model when there is one,
// otherwise the cost type (web search, code execution) or the raw description.
func label(model, costType, desc *string) string {
	for _, s := range []*string{model, costType, desc} {
		if s != nil && *s != "" {
			return *s
		}
	}
	return "other"
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	name, value, err := c.Auth(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set(name, value)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "spendtui/0.2 (https://github.com/sam-phinizy/sams-claude-menagerie)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("claude: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode == http.StatusOK {
		return body, nil
	}
	var e struct {
		Error struct{ Message string } `json:"error"`
	}
	msg := resp.Status
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		msg += ": " + e.Error.Message
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		msg += " (cost and usage reports need an Admin API key, an organization-scoped personal key, or an org:admin OAuth token; workspace keys don't work)"
	}
	return nil, fmt.Errorf("claude: %s", msg)
}
