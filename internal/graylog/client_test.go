package graylog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestClient(handler http.HandlerFunc) (*Client, *httptest.Server) {
	srv := httptest.NewServer(handler)
	c := NewClient(&Config{URL: srv.URL, Token: "secret"})
	return c, srv
}

func TestSearchRelative(t *testing.T) {
	var gotUser, gotPass string
	var gotPath, gotQuery string
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotUser, gotPass, _ = r.BasicAuth()
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("query")
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept header = %q", r.Header.Get("Accept"))
		}
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"messages":[{"message":{"source":"h1","message":"m1"}}]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	defer srv.Close()

	params := url.Values{}
	params.Set("query", "*")
	resp, err := c.SearchRelative(context.Background(), params)
	if err != nil {
		t.Fatalf("SearchRelative error: %v", err)
	}
	if gotUser != "secret" || gotPass != "token" {
		t.Errorf("basic auth = %q/%q, want secret/token", gotUser, gotPass)
	}
	if gotPath != "/api/search/universal/relative" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "*" {
		t.Errorf("query param = %q", gotQuery)
	}
	if len(resp.Messages) != 1 || resp.Messages[0].Message["source"] != "h1" {
		t.Errorf("unexpected messages: %+v", resp.Messages)
	}
}

func TestSearchKeywordParams(t *testing.T) {
	var q url.Values
	var gotPath string
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		q = r.URL.Query()
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"messages":[]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	defer srv.Close()

	_, err := c.SearchKeyword(context.Background(), "2024-01-01 to 2024-01-02", "level:1", "streamX", "UTC", 100, 200, "timestamp:asc")
	if err != nil {
		t.Fatalf("SearchKeyword error: %v", err)
	}
	if gotPath != "/api/search/universal/keyword" {
		t.Errorf("path = %q", gotPath)
	}
	if q.Get("keyword") != "2024-01-01 to 2024-01-02" {
		t.Errorf("keyword = %q", q.Get("keyword"))
	}
	if q.Get("query") != "level:1" {
		t.Errorf("query = %q", q.Get("query"))
	}
	if q.Get("timezone") != "UTC" {
		t.Errorf("timezone = %q", q.Get("timezone"))
	}
	if q.Get("filter") != "streams:streamX" {
		t.Errorf("filter = %q", q.Get("filter"))
	}
	if q.Get("limit") != "100" || q.Get("offset") != "200" {
		t.Errorf("limit/offset = %q/%q", q.Get("limit"), q.Get("offset"))
	}
	if q.Get("sort") != "timestamp:asc" {
		t.Errorf("sort = %q", q.Get("sort"))
	}
}

func TestSearchKeywordDefaultsQueryToStar(t *testing.T) {
	var q url.Values
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		if _, err := w.Write([]byte(`{"messages":[]}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	defer srv.Close()

	if _, err := c.SearchKeyword(context.Background(), "kw", "", "", "UTC", 10, 0, ""); err != nil {
		t.Fatalf("SearchKeyword error: %v", err)
	}
	if q.Get("query") != "*" {
		t.Errorf("query default = %q, want *", q.Get("query"))
	}
	if q.Has("filter") {
		t.Errorf("filter should be omitted when streamID empty, got %q", q.Get("filter"))
	}
}

func TestStatusErrors(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		check       func(error) bool
		errContains string
	}{
		{"unauthorized", http.StatusUnauthorized, func(e error) bool { var t *AuthError; return errors.As(e, &t) }, "authentication error: 401 Unauthorized - boom\nsecond line"},
		{"forbidden", http.StatusForbidden, func(e error) bool { var t *AuthError; return errors.As(e, &t) }, "authentication error: 403 Forbidden - boom\nsecond line"},
		{"server", http.StatusInternalServerError, func(e error) bool { var t *ServerError; return errors.As(e, &t) }, "server error 500 Internal Server Error: boom"},
		{"notfound", http.StatusNotFound, func(e error) bool { var t *APIError; return errors.As(e, &t) }, "API returned non-200 status: 404 Not Found - boom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				if _, err := w.Write([]byte("boom\nsecond line")); err != nil {
					t.Errorf("write response: %v", err)
				}
			})
			defer srv.Close()

			_, err := c.SearchRelative(context.Background(), url.Values{})
			if err == nil {
				t.Fatalf("expected error for status %d", tc.status)
			}
			if !tc.check(err) {
				t.Fatalf("unexpected error type for status %d: %T (%v)", tc.status, err, err)
			}
			if err.Error() != tc.errContains {
				t.Errorf("Error() = %q, want %q", err.Error(), tc.errContains)
			}
		})
	}
}

func TestDecodeError(t *testing.T) {
	c, srv := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("not json")); err != nil {
			t.Errorf("write response: %v", err)
		}
	})
	defer srv.Close()

	_, err := c.SearchRelative(context.Background(), url.Values{})
	var de *DecodeError
	if !errors.As(err, &de) {
		t.Fatalf("expected DecodeError, got %T (%v)", err, err)
	}
	if !strings.Contains(de.Error(), "decoding JSON") {
		t.Errorf("Error() = %q, want it to mention decoding JSON", de.Error())
	}
	if de.Unwrap() == nil {
		t.Error("Unwrap() = nil, want the underlying JSON error")
	}
}
