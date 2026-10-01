// Package anthropic reads rated spend from the Anthropic Admin API cost
// report (GET /v1/organizations/cost_report). It needs an Admin API key
// (sk-ant-admin...); regular API keys are rejected.
package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

const defaultBaseURL = "https://api.anthropic.com"

type Client struct {
	AdminKey string
	BaseURL  string
	HTTP     *http.Client
}

func New(adminKey string) *Client {
	return &Client{AdminKey: adminKey, BaseURL: defaultBaseURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "Claude" }

type costReport struct {
	Data []struct {
		StartingAt time.Time `json:"starting_at"`
		Results    []struct {
			Amount      string  `json:"amount"` // USD cents, decimal string
			Currency    string  `json:"currency"`
			Description *string `json:"description"`
			Model       *string `json:"model"`
			CostType    *string `json:"cost_type"`
		} `json:"results"`
	} `json:"data"`
	HasMore  bool    `json:"has_more"`
	NextPage *string `json:"next_page"`
}

func (c *Client) Fetch(ctx context.Context, start, end time.Time) ([]spend.Line, error) {
	var lines []spend.Line
	page := ""
	for {
		q := url.Values{}
		q.Set("starting_at", start.UTC().Format(time.RFC3339))
		q.Set("ending_at", end.UTC().Format(time.RFC3339))
		q.Set("bucket_width", "1d")
		q.Add("group_by[]", "description")
		q.Set("limit", "31")
		if page != "" {
			q.Set("page", page)
		}
		var rep costReport
		if err := c.get(ctx, "/v1/organizations/cost_report?"+q.Encode(), &rep); err != nil {
			return nil, err
		}
		for _, b := range rep.Data {
			for _, r := range b.Results {
				cents, err := strconv.ParseFloat(r.Amount, 64)
				if err != nil {
					return nil, fmt.Errorf("claude: bad amount %q: %w", r.Amount, err)
				}
				lines = append(lines, spend.Line{
					Day:   spend.Day(b.StartingAt),
					Model: label(r.Model, r.CostType, r.Description),
					USD:   cents / 100,
				})
			}
		}
		if !rep.HasMore || rep.NextPage == nil || *rep.NextPage == "" {
			return lines, nil
		}
		page = *rep.NextPage
	}
}

// label picks the grouping key for a cost line: the model when there is one,
// otherwise the cost type (web search, code execution) or the raw description.
func label(model, costType, desc *string) string {
	for _, s := range []*string{model, costType, desc} {
		if s != nil && *s != "" {
			return *s
		}
	}
	return "other"
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", c.AdminKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("User-Agent", "spendtui/0.1 (https://github.com/sam-phinizy/sams-claude-menagerie)")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("claude: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct{ Message string } `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
			return fmt.Errorf("claude: %s: %s", resp.Status, e.Error.Message)
		}
		return fmt.Errorf("claude: %s", resp.Status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("claude: decoding cost report: %w", err)
	}
	return nil
}
