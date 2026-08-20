package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"gotail/internal/graylog"
)

func TestBuildKeyword(t *testing.T) {
	cases := []struct {
		begin, end, want string
		wantErr          bool
	}{
		{"2024-01-01", "2024-01-02", "2024-01-01 to 2024-01-02", false},
		{"2024-01-01", "", "2024-01-01", false},
		{"", "2024-01-02", "2024-01-02", false},
		{"", "", "", true},
	}
	for _, tc := range cases {
		got, err := buildKeyword(tc.begin, tc.end)
		if tc.wantErr {
			if err == nil {
				t.Errorf("buildKeyword(%q,%q) expected error", tc.begin, tc.end)
			}
			continue
		}
		if err != nil {
			t.Errorf("buildKeyword(%q,%q) error: %v", tc.begin, tc.end, err)
		}
		if got != tc.want {
			t.Errorf("buildKeyword(%q,%q) = %q, want %q", tc.begin, tc.end, got, tc.want)
		}
	}
}

func TestValidateFormat(t *testing.T) {
	if err := validateFormat("line", nil); err != nil {
		t.Errorf("line: %v", err)
	}
	if err := validateFormat("json", nil); err != nil {
		t.Errorf("json: %v", err)
	}
	if err := validateFormat("fields", []string{"a"}); err != nil {
		t.Errorf("fields with list: %v", err)
	}
	if err := validateFormat("fields", nil); err == nil {
		t.Error("fields without list should error")
	}
	if err := validateFormat("bogus", nil); err == nil {
		t.Error("bogus format should error")
	}
}

// buildDataset returns messages ordered DESCENDING by timestamp, so a correct
// implementation must sort them back to ascending order.
func buildDataset() []map[string]interface{} {
	return []map[string]interface{}{
		{"timestamp": "2024-01-05T00:00:00.000Z", "source": "h5", "message": "m5"},
		{"timestamp": "2024-01-04T00:00:00.000Z", "source": "h4", "message": "m4"},
		{"timestamp": "2024-01-03T00:00:00.000Z", "source": "h3", "message": "m3"},
		{"timestamp": "2024-01-02T00:00:00.000Z", "source": "h2", "message": "m2"},
		{"timestamp": "2024-01-01T00:00:00.000Z", "source": "h1", "message": "m1"},
	}
}

func newKeywordServer(t *testing.T, dataset []map[string]interface{}, reqCount *int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reqCount != nil {
			*reqCount++
		}
		if r.URL.Path != "/api/search/universal/keyword" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("keyword") == "" {
			t.Error("missing keyword param")
		}
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

		end := offset + limit
		if end > len(dataset) {
			end = len(dataset)
		}
		var page []map[string]interface{}
		if offset < len(dataset) {
			page = dataset[offset:end]
		}

		envs := make([]map[string]interface{}, 0, len(page))
		for _, m := range page {
			envs = append(envs, map[string]interface{}{"message": m})
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{"messages": envs}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
}

func TestCollectMessagesPaginatesAndSorts(t *testing.T) {
	dataset := buildDataset()
	var reqCount int
	srv := newKeywordServer(t, dataset, &reqCount)
	defer srv.Close()

	cfg := &graylog.Config{URL: srv.URL, Token: "t", Limit: 2, Timeout: 5 * time.Second}
	client := graylog.NewClient(cfg)

	msgs, err := collectMessages(context.Background(), client, cfg, "2024-01-01 to 2024-01-06", "*", "UTC")
	if err != nil {
		t.Fatalf("collectMessages error: %v", err)
	}
	if len(msgs) != 5 {
		t.Fatalf("collected %d messages, want 5", len(msgs))
	}
	// limit=2 over 5 messages => pages at offset 0,2,4 (last page short) = 3 requests
	if reqCount != 3 {
		t.Errorf("made %d requests, want 3", reqCount)
	}
	// Ascending by timestamp: m1..m5
	for i, want := range []string{"m1", "m2", "m3", "m4", "m5"} {
		if msgs[i]["message"] != want {
			t.Errorf("msgs[%d] = %v, want %v", i, msgs[i]["message"], want)
		}
	}
}

func TestCollectMessagesEmpty(t *testing.T) {
	srv := newKeywordServer(t, nil, nil)
	defer srv.Close()

	cfg := &graylog.Config{URL: srv.URL, Token: "t", Limit: 2, Timeout: 5 * time.Second}
	client := graylog.NewClient(cfg)

	msgs, err := collectMessages(context.Background(), client, cfg, "kw", "*", "UTC")
	if err != nil {
		t.Fatalf("collectMessages error: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("expected 0 messages, got %d", len(msgs))
	}
}

func TestRenderFormats(t *testing.T) {
	msgs := []map[string]interface{}{
		{"timestamp": "2024-01-01T00:00:00.000Z", "source": "h1", "message": "hello"},
	}

	var line bytes.Buffer
	if err := render(&line, msgs, "line", nil); err != nil {
		t.Fatalf("line render: %v", err)
	}
	if got := strings.TrimSpace(line.String()); got != "[2024-01-01T00:00:00.000Z] h1: hello" {
		t.Errorf("line output = %q", got)
	}

	var js bytes.Buffer
	if err := render(&js, msgs, "json", nil); err != nil {
		t.Fatalf("json render: %v", err)
	}
	var round map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(js.String())), &round); err != nil {
		t.Fatalf("json output invalid: %v (%q)", err, js.String())
	}
	if round["source"] != "h1" {
		t.Errorf("json source = %v", round["source"])
	}

	var fld bytes.Buffer
	if err := render(&fld, msgs, "fields", []string{"source", "message"}); err != nil {
		t.Fatalf("fields render: %v", err)
	}
	if got := strings.TrimSpace(fld.String()); got != "h1\thello" {
		t.Errorf("fields output = %q, want %q", got, "h1\thello")
	}
}
