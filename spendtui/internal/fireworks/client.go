// Package fireworks reads spend from the Fireworks AI billing usage report
// (GET /v1/accounts/{account}/billingUsage). The report answers in daily
// buckets split across serverless, dedicated and training arrays, and rejects
// windows wider than 31 days, so longer ranges are fetched in chunks.
package fireworks

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

const (
	defaultBaseURL = "https://api.fireworks.ai"
	maxWindow      = 31 * 24 * time.Hour
)

type Client struct {
	APIKey    string
	AccountID string
	BaseURL   string
	HTTP      *http.Client
}

func New(apiKey, accountID string) *Client {
	// Accept the "accounts/<id>" form firectl prints.
	accountID = strings.TrimPrefix(strings.Trim(accountID, "/ "), "accounts/")
	return &Client{APIKey: apiKey, AccountID: accountID, BaseURL: defaultBaseURL, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Name() string { return "Fireworks" }

// int64s are proto-JSON encoded, which means they usually arrive as strings.
type num int64

func (n *num) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		f, ferr := strconv.ParseFloat(s, 64)
		if ferr != nil {
			return err
		}
		v = int64(f)
	}
	*n = num(v)
	return nil
}

type bucket struct {
	StartTime        time.Time `json:"startTime"`
	ModelName        string    `json:"modelName"`
	DeploymentID     string    `json:"deploymentId"`
	BaseModel        string    `json:"baseModel"`
	JobType          string    `json:"jobType"`
	PromptTokens     num       `json:"promptTokens"`
	CompletionTokens num       `json:"completionTokens"`
	CostNanoUSD      num       `json:"costNanoUsd"`
}

type usageReport struct {
	ServerlessCosts []bucket `json:"serverlessCosts"`
	DedicatedCosts  []bucket `json:"dedicatedCosts"`
	TrainingCosts   []bucket `json:"trainingCosts"`
}

func (c *Client) Fetch(ctx context.Context, start, end time.Time) ([]spend.Line, error) {
	if c.AccountID == "" {
		return nil, fmt.Errorf("fireworks: account id is required")
	}
	var lines []spend.Line
	for ws := start; ws.Before(end); ws = ws.Add(maxWindow) {
		we := ws.Add(maxWindow)
		if we.After(end) {
			we = end
		}
		var rep usageReport
		if err := c.get(ctx, ws, we, &rep); err != nil {
			return nil, err
		}
		for _, b := range rep.ServerlessCosts {
			lines = append(lines, line(b, shortModel(b.ModelName)))
		}
		for _, b := range rep.DedicatedCosts {
			name := b.DeploymentID
			if b.BaseModel != "" {
				name = shortModel(b.BaseModel)
			}
			lines = append(lines, line(b, "dedicated: "+name))
		}
		for _, b := range rep.TrainingCosts {
			name := b.JobType
			if b.BaseModel != "" {
				name = strings.TrimSpace(name + " " + shortModel(b.BaseModel))
			}
			lines = append(lines, line(b, "training: "+name))
		}
	}
	return lines, nil
}

func line(b bucket, model string) spend.Line {
	return spend.Line{
		Day:          spend.Day(b.StartTime),
		Model:        model,
		USD:          float64(b.CostNanoUSD) / 1e9,
		InputTokens:  int64(b.PromptTokens),
		OutputTokens: int64(b.CompletionTokens),
	}
}

// shortModel turns "accounts/fireworks/models/llama-v3p1-8b" into
// "llama-v3p1-8b".
func shortModel(name string) string {
	if i := strings.LastIndex(name, "/models/"); i >= 0 {
		return name[i+len("/models/"):]
	}
	if name == "" {
		return "unknown"
	}
	return name
}

func (c *Client) get(ctx context.Context, start, end time.Time, out any) error {
	q := url.Values{}
	q.Set("startTime", start.UTC().Format(time.RFC3339))
	q.Set("endTime", end.UTC().Format(time.RFC3339))
	u := fmt.Sprintf("%s/v1/accounts/%s/billingUsage?%s", c.BaseURL, url.PathEscape(c.AccountID), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("fireworks: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Message string `json:"message"`
			Error   struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		msg := e.Message
		if msg == "" {
			msg = e.Error.Message
		}
		if msg != "" {
			return fmt.Errorf("fireworks: %s: %s", resp.Status, msg)
		}
		return fmt.Errorf("fireworks: %s", resp.Status)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("fireworks: decoding billing usage: %w", err)
	}
	return nil
}
