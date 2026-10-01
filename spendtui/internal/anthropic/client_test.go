package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchPaginatesAndConvertsCents(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("x-api-key") != "sk-ant-admin-test" || r.Header.Get("anthropic-version") == "" {
			t.Errorf("missing auth headers")
		}
		if r.URL.Path != "/v1/organizations/cost_report" || r.URL.Query().Get("group_by[]") != "description" {
			t.Errorf("unexpected request %s", r.URL)
		}
		if r.URL.Query().Get("page") == "" {
			w.Write([]byte(`{"data":[{"starting_at":"2026-09-01T00:00:00Z","ending_at":"2026-09-02T00:00:00Z","results":[
				{"currency":"USD","amount":"1234.5","description":"Claude Opus 5.5 Usage - Input Tokens","model":"claude-opus-5-5","cost_type":"tokens"},
				{"currency":"USD","amount":"50","description":"Web Search Usage","model":null,"cost_type":"web_search"}]}],
				"has_more":true,"next_page":"page_2"}`))
			return
		}
		w.Write([]byte(`{"data":[{"starting_at":"2026-09-02T00:00:00Z","results":[{"currency":"USD","amount":"100","model":"claude-haiku-4-5"}]}],"has_more":false,"next_page":null}`))
	}))
	defer srv.Close()

	c := New("sk-ant-admin-test")
	c.BaseURL = srv.URL
	lines, err := c.Fetch(context.Background(), time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(lines) != 3 {
		t.Fatalf("calls=%d lines=%d", calls, len(lines))
	}
	if lines[0].USD != 12.345 || lines[0].Model != "claude-opus-5-5" {
		t.Errorf("line0 = %+v", lines[0])
	}
	if lines[1].Model != "web_search" || lines[1].USD != 0.5 {
		t.Errorf("line1 = %+v", lines[1])
	}
}

func TestFetchSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	}))
	defer srv.Close()
	c := New("bad")
	c.BaseURL = srv.URL
	_, err := c.Fetch(context.Background(), time.Now().Add(-time.Hour), time.Now())
	if err == nil || err.Error() != "claude: 401 Unauthorized: invalid x-api-key" {
		t.Fatalf("err = %v", err)
	}
}
