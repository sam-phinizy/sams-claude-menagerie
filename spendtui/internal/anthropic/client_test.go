package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sam-phinizy/sams-claude-menagerie/spendtui/internal/spend"
)

func fakeAPI(t *testing.T, auth func(r *http.Request) bool) (*httptest.Server, *int) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if !auth(r) || r.Header.Get("anthropic-version") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
			return
		}
		page := r.URL.Query().Get("page")
		switch r.URL.Path {
		case "/v1/organizations/cost_report":
			if r.URL.Query().Get("group_by[]") != "description" {
				t.Errorf("cost report group_by = %q", r.URL.Query().Get("group_by[]"))
			}
			if page == "" {
				w.Write([]byte(`{"data":[{"starting_at":"2026-09-01T00:00:00Z","ending_at":"2026-09-02T00:00:00Z","results":[
					{"currency":"USD","amount":"1234.5","description":"Claude Opus 5.5 Usage - Input Tokens","model":"claude-opus-5-5","cost_type":"tokens"},
					{"currency":"USD","amount":"50","description":"Web Search Usage","model":null,"cost_type":"web_search"}]}],
					"has_more":true,"next_page":"page_2"}`))
				return
			}
			w.Write([]byte(`{"data":[{"starting_at":"2026-09-02T00:00:00Z","results":[{"currency":"USD","amount":"100","model":"claude-haiku-4-5"}]}],"has_more":false,"next_page":null}`))
		case "/v1/organizations/usage_report/messages":
			if r.URL.Query().Get("group_by[]") != "model" || r.URL.Query().Get("bucket_width") != "1d" {
				t.Errorf("usage report query = %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"data":[{"starting_at":"2026-09-01T00:00:00Z","ending_at":"2026-09-02T00:00:00Z","results":[
				{"model":"claude-opus-5-5","uncached_input_tokens":1500,"cache_read_input_tokens":6000,
				 "cache_creation":{"ephemeral_1h_input_tokens":100,"ephemeral_5m_input_tokens":400},
				 "output_tokens":700,"server_tool_use":{"web_search_requests":3}}]},
				{"starting_at":"2026-09-02T00:00:00Z","ending_at":"2026-09-03T00:00:00Z","results":[]}],
				"has_more":false,"next_page":null}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	return srv, &calls
}

func fetch(t *testing.T, c *Client) []spend.Line {
	t.Helper()
	lines, err := c.Fetch(context.Background(), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

func TestFetchJoinsCostsAndTokens(t *testing.T) {
	srv, calls := fakeAPI(t, func(r *http.Request) bool { return r.Header.Get("x-api-key") == "sk-ant-admin-test" })
	defer srv.Close()
	c := New(APIKey("sk-ant-admin-test"))
	c.BaseURL = srv.URL

	lines := fetch(t, c)
	if *calls != 3 || len(lines) != 4 {
		t.Fatalf("calls=%d lines=%+v", *calls, lines)
	}
	if lines[0].USD != 12.345 || lines[0].Model != "claude-opus-5-5" {
		t.Errorf("cost line = %+v", lines[0])
	}
	if lines[1].Model != "web_search" || lines[1].USD != 0.5 {
		t.Errorf("web search line = %+v", lines[1])
	}
	want := spend.Tokens{Input: 1500, CacheRead: 6000, CacheWrite: 500, Output: 700}
	if l := lines[3]; l.Model != "claude-opus-5-5" || l.USD != 0 || l.Tokens != want {
		t.Errorf("usage line = %+v", l)
	}
	// Rolled up, the cost and usage lines land on the same model.
	r := spend.Range{Start: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)}
	for _, m := range spend.Summarize(lines, r).Models {
		if m.Model == "claude-opus-5-5" && (m.USD != 12.345 || m.Tokens != want) {
			t.Errorf("rolled up opus = %+v", m)
		}
	}
}

func TestBearerCommandAuth(t *testing.T) {
	srv, _ := fakeAPI(t, func(r *http.Request) bool { return r.Header.Get("authorization") == "Bearer tok-123" })
	defer srv.Close()
	c := New(BearerCommand("echo tok-123"))
	c.BaseURL = srv.URL
	if lines := fetch(t, c); len(lines) != 4 {
		t.Fatalf("lines = %+v", lines)
	}
}

func TestFetchExplainsAuthErrors(t *testing.T) {
	srv, _ := fakeAPI(t, func(*http.Request) bool { return false })
	defer srv.Close()
	c := New(APIKey("sk-ant-api03-workspace-key"))
	c.BaseURL = srv.URL
	_, err := c.Fetch(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err == nil || !strings.Contains(err.Error(), "401 Unauthorized: invalid x-api-key") ||
		!strings.Contains(err.Error(), "workspace keys don't work") {
		t.Fatalf("err = %v", err)
	}
}
