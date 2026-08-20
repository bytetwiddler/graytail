package graylog

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultStreamID is Graylog's built-in "All messages" stream, used when no
// stream is configured.
const DefaultStreamID = "000000000000000000000001"

// Config holds the settings shared by all gotail binaries. Values are populated
// from flags, environment variables, an optional .env file, and defaults, in
// that order of precedence.
type Config struct {
	URL      string
	Token    string
	StreamID string
	Timeout  time.Duration
	Insecure bool
	Limit    int
}

// LoadEnvFile loads a local .env file (key=value per line) into the process
// environment. Existing environment variables are never overwritten. Comment
// lines (starting with '#') and blank lines are ignored, and surrounding
// single or double quotes are stripped from values.
func LoadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		if k == "" {
			continue
		}
		if _, exists := os.LookupEnv(k); !exists {
			if err := os.Setenv(k, v); err != nil {
				return err
			}
		}
	}
	return s.Err()
}

// GetenvString returns the environment variable value for key, or def when it
// is unset or empty.
func GetenvString(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// GetenvDuration parses the environment variable named key as a duration,
// falling back to def when unset or invalid.
func GetenvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// GetenvBool parses the environment variable named key as a bool, falling back
// to def when unset or invalid.
func GetenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

// GetenvInt parses the environment variable named key as an int, falling back
// to def when unset or invalid.
func GetenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

// RegisterCommonFlags wires the flags shared by every gotail binary onto fs and
// returns a Config that is populated once fs.Parse has been called. Flag
// defaults are derived from the environment (which itself may have been seeded
// from a .env file via LoadEnvFile), preserving the flag → env → .env → default
// precedence.
func RegisterCommonFlags(fs *flag.FlagSet) *Config {
	cfg := &Config{}
	fs.StringVar(&cfg.URL, "url", GetenvString("GRAYLOG_URL", ""), "Graylog base URL (e.g. https://graylog.example.com:9000)")
	fs.StringVar(&cfg.Token, "token", GetenvString("GRAYLOG_API_TOKEN", ""), "Graylog API token (or set GRAYLOG_API_TOKEN)")
	fs.StringVar(&cfg.StreamID, "stream", GetenvString("GRAYLOG_STREAM_ID", ""), "Stream ID to filter (default: all messages)")
	fs.DurationVar(&cfg.Timeout, "timeout", GetenvDuration("TIMEOUT", 60*time.Second), "Per-request timeout (or set TIMEOUT)")
	fs.BoolVar(&cfg.Insecure, "insecure", GetenvBool("INSECURE", false), "Skip TLS verification (or set INSECURE=true) - not recommended")
	fs.IntVar(&cfg.Limit, "limit", GetenvInt("LIMIT", 100), "Max messages to fetch per request (or set LIMIT)")
	return cfg
}

// ApplyDefaults fills in derived defaults that depend on other values, such as
// falling back to the "All messages" stream when none is configured.
func (c *Config) ApplyDefaults() {
	if c.StreamID == "" {
		c.StreamID = DefaultStreamID
	}
}

// Validate ensures the required fields (URL and token) are present.
func (c *Config) Validate() error {
	if c.URL == "" {
		return fmt.Errorf("missing Graylog URL: set via -url or GRAYLOG_URL environment variable")
	}
	if c.Token == "" {
		return fmt.Errorf("API token must be set via -token or GRAYLOG_API_TOKEN environment variable")
	}
	return nil
}
