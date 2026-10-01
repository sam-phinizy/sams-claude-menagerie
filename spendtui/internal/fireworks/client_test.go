package fireworks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchChunksWindowsAndParsesBuckets(t *testing.T) {
	var windows [][2]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/accounts/my-acct/billingUsage" || r.Header.Get("Authorization") != "Bearer fw_test" {
			t.Errorf("unexpected request %s", r.URL)
		}
		q := r.URL.Query()
		windows = append(windows, [2]string{q.Get("startTime"), q.Get("endTime")})
		if len(windows) > 1 {
			w.Write([]byte(`{}`))
			return
		}
		w.Write([]byte(`{
			"serverlessCosts":[{"startTime":"2026-08-01T00:00:00Z","endTime":"2026-08-02T00:00:00Z","modelName":"accounts/fireworks/models/deepseek-v3p1","promptTokens":"1000","completionTokens":"200","costNanoUsd":"2500000000"}],
			"dedicatedCosts":[{"startTime":"2026-08-01T00:00:00Z","deploymentId":"dep-1","costNanoUsd":1000000000}],
			"trainingCosts":null}`))
	}))
	defer srv.Close()

	c := New("fw_test", "accounts/my-acct")
	c.BaseURL = srv.URL
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	lines, err := c.Fetch(context.Background(), start, start.AddDate(0, 0, 45))
	if err != nil {
		t.Fatal(err)
	}
	if len(windows) != 2 || windows[0][1] != windows[1][0] || windows[1][1] != "2026-09-15T00:00:00Z" {
		t.Fatalf("windows = %v", windows)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	if l := lines[0]; l.Model != "deepseek-v3p1" || l.USD != 2.5 || l.InputTokens != 1000 || l.OutputTokens != 200 {
		t.Errorf("serverless = %+v", l)
	}
	if l := lines[1]; l.Model != "dedicated: dep-1" || l.USD != 1 {
		t.Errorf("dedicated = %+v", l)
	}
}
