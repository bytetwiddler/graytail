package graylog

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// maxBodyBytes caps how much of a response body is read, matching the original
// graytail behavior (10MB).
const maxBodyBytes = 10 * 1024 * 1024

// Client is a thin wrapper around the Graylog universal search HTTP API. It
// holds a reusable *http.Client, the base URL, and the API token, and exposes
// typed search methods for the gotail binaries.
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

// NewClient builds a Client from the shared configuration, wiring a TLS
// transport that honors cfg.Insecure and a reusable *http.Client. Per-request
// timeouts are expected to be supplied via the context passed to the search
// methods.
func NewClient(cfg *Config) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Insecure},
	}
	return &Client{
		httpClient: &http.Client{Transport: transport},
		baseURL:    strings.TrimRight(cfg.URL, "/"),
		token:      cfg.Token,
	}
}

// AuthError indicates a 401/403 response; callers should treat it as fatal.
type AuthError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("authentication error: %s - %s", e.Status, e.Body)
}

// ServerError indicates a 5xx response; callers may retry with backoff.
type ServerError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("server error %s: %s", e.Status, firstLine(e.Body))
}

// APIError indicates a non-200 response that is neither an auth nor a server
// error.
type APIError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API returned non-200 status: %s - %s", e.Status, firstLine(e.Body))
}

// DecodeError indicates the response body could not be decoded as JSON.
type DecodeError struct {
	Err error
}

func (e *DecodeError) Error() string { return fmt.Sprintf("decoding JSON: %v", e.Err) }
func (e *DecodeError) Unwrap() error { return e.Err }

// do performs a GET request against endpoint, centralizing basic auth, the
// Accept header, the 10MB body cap, and status handling. It returns a typed
// error for auth (401/403), server (5xx), other non-200 statuses, and JSON
// decode failures.
func (c *Client) do(ctx context.Context, endpoint string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.token, "token")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	bodyBuf, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))

	if resp.StatusCode != http.StatusOK {
		switch {
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, &AuthError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(bodyBuf)}
		case resp.StatusCode >= 500 && resp.StatusCode <= 599:
			return nil, &ServerError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(bodyBuf)}
		default:
			return nil, &APIError{StatusCode: resp.StatusCode, Status: resp.Status, Body: string(bodyBuf)}
		}
	}

	var result Response
	if err := json.Unmarshal(bodyBuf, &result); err != nil {
		return nil, &DecodeError{Err: err}
	}
	return &result, nil
}

// SearchRelative queries the relative search endpoint with the provided query
// parameters. It powers graytail's tailing loop.
func (c *Client) SearchRelative(ctx context.Context, params url.Values) (*Response, error) {
	endpoint := fmt.Sprintf("%s/api/search/universal/relative?%s", c.baseURL, params.Encode())
	return c.do(ctx, endpoint)
}

// SearchKeyword queries the keyword search endpoint, letting Graylog parse the
// natural-language time expression in keyword (e.g. "<begin> to <end>"). It
// powers grayquery's bounded searches. An empty query defaults to "*", and an
// empty streamID omits the stream filter. Graylog requires a non-empty
// timezone (used to resolve the keyword expression); callers should default
// it (e.g. to "UTC") rather than pass an empty string.
func (c *Client) SearchKeyword(ctx context.Context, keyword, query, streamID, timezone string, limit, offset int, sort string) (*Response, error) {
	if query == "" {
		query = "*"
	}
	params := url.Values{}
	params.Set("keyword", keyword)
	params.Set("query", query)
	params.Set("timezone", timezone)
	if streamID != "" {
		params.Set("filter", "streams:"+streamID)
	}
	params.Set("limit", strconv.Itoa(limit))
	params.Set("offset", strconv.Itoa(offset))
	if sort != "" {
		params.Set("sort", sort)
	}
	endpoint := fmt.Sprintf("%s/api/search/universal/keyword?%s", c.baseURL, params.Encode())
	return c.do(ctx, endpoint)
}

func firstLine(s string) string {
	return strings.SplitN(s, "\n", 2)[0]
}
