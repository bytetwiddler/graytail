package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gotail/internal/graylog"
)

// graytail tails Graylog via the HTTP relative search API. It supports optional
// loading of a local .env file (key=value) to set environment variables. All
// configurable values can be set via environment variables or flags, with
// reasonable defaults.

func main() {
	// Load local .env if present (do this before reading os.Getenv for flags)
	if err := graylog.LoadEnvFile(".env"); err == nil {
		log.Println("Loaded .env file")
	} else if !os.IsNotExist(err) {
		log.Printf("error loading .env: %v\n", err)
	}

	cfg := graylog.RegisterCommonFlags(flag.CommandLine)

	// graytail-specific defaults derived from environment when available.
	defaultPoll := graylog.GetenvDuration("POLL", 10*time.Second)
	defaultLookback := graylog.GetenvString("LOOKBACK", "30")
	defaultDupRetention := graylog.GetenvDuration("DUP_RETENTION", 1*time.Hour)

	var (
		poll     = flag.Duration("poll", defaultPoll, "Polling interval between requests (or set POLL)")
		lookback = flag.String("lookback", defaultLookback, "Initial lookback range in seconds (or set LOOKBACK)")
		dupRet   = flag.Duration("dup-retention", defaultDupRetention, "How long to remember seen messages to avoid duplicates (e.g., 1h)")
	)
	flag.Parse()

	if err := cfg.Validate(); err != nil {
		log.Fatalln(err)
	}
	cfg.ApplyDefaults()

	client := graylog.NewClient(cfg)

	log.Println("📋 Tailing Graylog logs... Press Ctrl+C to stop.")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var lastSeenTimestamp string
	backoff := 1 * time.Second
	maxBackoff := 32 * time.Second

	seen := make(map[string]time.Time)
	lastPrune := time.Now()

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return
		default:
		}

		// Prune seen map every 5 minutes
		if time.Since(lastPrune) > 5*time.Minute {
			cutoff := time.Now().Add(-*dupRet)
			for k, t := range seen {
				if t.Before(cutoff) {
					delete(seen, k)
				}
			}
			lastPrune = time.Now()
		}

		// Build query parameters
		params := url.Values{}
		params.Set("query", "*")
		params.Set("filter", "streams:"+cfg.StreamID)
		params.Set("limit", fmt.Sprintf("%d", cfg.Limit))
		params.Set("sort", "timestamp:asc")

		// Determine appropriate 'range' to avoid mixing timestamp queries with relative range
		if lastSeenTimestamp != "" {
			// Attempt to parse last seen timestamp as RFC3339
			if t, err := time.Parse(time.RFC3339Nano, lastSeenTimestamp); err == nil {
				// compute seconds since last seen and add a small buffer
				delta := time.Since(t).Seconds()
				rangeSecs := int(math.Ceil(delta)) + 5
				if rangeSecs < 5 {
					rangeSecs = 5
				}
				// cap range to reasonable value to avoid huge queries
				if rangeSecs > 3600 {
					rangeSecs = 3600
				}
				params.Set("range", fmt.Sprintf("%d", rangeSecs))
			} else {
				// fallback to configured lookback if parsing fails
				params.Set("range", *lookback)
				// keep the timestamp:> query as a fallback (older behavior)
				params.Set("query", fmt.Sprintf("timestamp:>%s", lastSeenTimestamp))
			}
		} else {
			params.Set("range", *lookback)
		}

		// Per-request context to enforce timeout and allow graceful shutdown
		reqCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		result, err := client.SearchRelative(reqCtx, params)
		// cancel the request context now that the request has returned so the timer/goroutine is freed
		cancel()
		if err != nil {
			// Authentication/authorization error: bail out
			var authErr *graylog.AuthError
			if errors.As(err, &authErr) {
				log.Fatalf("%v\n", authErr)
			}

			// Server error: apply exponential backoff and retry
			var serverErr *graylog.ServerError
			if errors.As(err, &serverErr) {
				log.Printf("%v\n", serverErr)
				if backoff < maxBackoff {
					backoff *= 2
				}
				time.Sleep(backoff)
				continue
			}

			// Other non-200 statuses: log and wait normal poll interval
			var apiErr *graylog.APIError
			if errors.As(err, &apiErr) {
				log.Printf("%v\n", apiErr)
				time.Sleep(*poll)
				continue
			}

			// Decode error: log and wait normal poll interval
			var decErr *graylog.DecodeError
			if errors.As(err, &decErr) {
				log.Printf("%v\n", decErr)
				time.Sleep(*poll)
				continue
			}

			// Network or request error: respect context cancellation vs retry with backoff
			select {
			case <-ctx.Done():
				log.Println("context canceled, exiting")
				return
			default:
				log.Printf("network error: %v\n", err)
				if backoff < maxBackoff {
					backoff *= 2
				}
				time.Sleep(backoff)
				continue
			}
		}

		// On success reset backoff
		backoff = 1 * time.Second

		var maxSeenTime time.Time
		for _, env := range result.Messages {
			msg := env.Message

			// Create a stable key for deduplication
			key := graylog.MessageKey(msg)
			if _, seenBefore := seen[key]; seenBefore {
				continue
			}

			fmt.Println(graylog.FormatLine(msg))
			seen[key] = time.Now()

			// Track max timestamp for range computation
			if tsStr, ok := msg["timestamp"].(string); ok {
				if t, err := time.Parse(time.RFC3339Nano, tsStr); err == nil {
					if maxSeenTime.IsZero() || t.After(maxSeenTime) {
						maxSeenTime = t
					}
				}
			}
		}

		if !maxSeenTime.IsZero() {
			lastSeenTimestamp = maxSeenTime.Format(time.RFC3339Nano)
		}

		// Wait until next poll or until signaled
		select {
		case <-time.After(*poll):
			continue
		case <-ctx.Done():
			log.Println("shutting down")
			return
		}
	}
}
