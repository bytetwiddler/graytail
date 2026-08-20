package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"gotail/internal/graylog"
)

// grayquery runs a bounded, one-shot Graylog search over a --begin/--end time
// window using Graylog's keyword search endpoint, and prints the matching
// messages in ascending timestamp order.

// maxTotalMessages is a safety cap on how many messages grayquery will fetch
// across all pages, preventing an unbounded query from exhausting memory.
const maxTotalMessages = 100000

func main() {
	// Load local .env if present (before reading env-derived flag defaults).
	if err := graylog.LoadEnvFile(".env"); err == nil {
		log.Println("Loaded .env file")
	} else if !os.IsNotExist(err) {
		log.Printf("error loading .env: %v\n", err)
	}

	cfg := graylog.RegisterCommonFlags(flag.CommandLine)

	defaultTimezone := graylog.GetenvString("TIMEZONE", "UTC")

	var begin, end, query, format, fieldsCSV, timezone string
	// Long and short forms share the same target variable.
	flag.StringVar(&begin, "begin", "", "Start of the time window (Graylog keyword time, e.g. \"2024-01-02 15:04:05\")")
	flag.StringVar(&begin, "b", "", "Shorthand for --begin")
	flag.StringVar(&end, "end", "", "End of the time window (Graylog keyword time)")
	flag.StringVar(&end, "e", "", "Shorthand for --end")
	flag.StringVar(&query, "query", "", "Graylog search query string (defaults to * ; may also be given positionally)")
	flag.StringVar(&query, "q", "", "Shorthand for --query")
	flag.StringVar(&format, "format", "line", "Output format: line, json, or fields")
	flag.StringVar(&format, "o", "line", "Shorthand for --format")
	flag.StringVar(&fieldsCSV, "fields", "", "Comma-separated field list for --format fields")
	flag.StringVar(&timezone, "timezone", defaultTimezone, "Timezone used to resolve --begin/--end (or set TIMEZONE)")
	flag.StringVar(&timezone, "tz", defaultTimezone, "Shorthand for --timezone")

	flag.Parse()

	if err := cfg.Validate(); err != nil {
		log.Fatalln(err)
	}
	cfg.ApplyDefaults()

	// A query given positionally (after flags) overrides an empty --query.
	if query == "" && flag.NArg() > 0 {
		query = strings.Join(flag.Args(), " ")
	}

	keyword, err := buildKeyword(begin, end)
	if err != nil {
		log.Fatalln(err)
	}

	fields := parseFields(fieldsCSV)
	if err := validateFormat(format, fields); err != nil {
		log.Fatalln(err)
	}

	client := graylog.NewClient(cfg)

	msgs, err := collectMessages(context.Background(), client, cfg, keyword, query, timezone)
	if err != nil {
		var authErr *graylog.AuthError
		if errors.As(err, &authErr) {
			log.Printf("%v\n", authErr)
			os.Exit(2)
		}
		log.Printf("%v\n", err)
		os.Exit(1)
	}

	if err := render(os.Stdout, msgs, format, fields); err != nil {
		log.Printf("rendering output: %v\n", err)
		os.Exit(1)
	}
}

// buildKeyword combines the begin/end bounds into a Graylog keyword time
// expression. At least one bound is required.
func buildKeyword(begin, end string) (string, error) {
	begin = strings.TrimSpace(begin)
	end = strings.TrimSpace(end)
	switch {
	case begin != "" && end != "":
		return begin + " to " + end, nil
	case begin != "":
		return begin, nil
	case end != "":
		return end, nil
	default:
		return "", errors.New("at least one of --begin/-b or --end/-e must be provided")
	}
}

func parseFields(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func validateFormat(format string, fields []string) error {
	switch format {
	case "line", "json":
		return nil
	case "fields":
		if len(fields) == 0 {
			return errors.New("--format fields requires a non-empty --fields list")
		}
		return nil
	default:
		return fmt.Errorf("unknown --format %q (want line, json, or fields)", format)
	}
}

// collectMessages pages through the keyword search endpoint, advancing offset by
// the configured limit until a short/empty page is returned or the safety cap is
// hit, then returns the messages sorted ascending by timestamp.
func collectMessages(ctx context.Context, client *graylog.Client, cfg *graylog.Config, keyword, query, timezone string) ([]map[string]interface{}, error) {
	limit := cfg.Limit
	if limit <= 0 {
		limit = 100
	}

	var msgs []map[string]interface{}
	for offset := 0; offset < maxTotalMessages; offset += limit {
		reqCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		resp, err := client.SearchKeyword(reqCtx, keyword, query, cfg.StreamID, timezone, limit, offset, "timestamp:asc")
		cancel()
		if err != nil {
			return nil, err
		}

		for _, env := range resp.Messages {
			msgs = append(msgs, env.Message)
			if len(msgs) >= maxTotalMessages {
				break
			}
		}

		// Last page reached when fewer than limit messages were returned.
		if len(resp.Messages) < limit || len(msgs) >= maxTotalMessages {
			break
		}
	}

	sortByTimestamp(msgs)
	return msgs, nil
}

// sortByTimestamp orders messages ascending by their timestamp field, parsing
// RFC3339 when possible and falling back to string comparison.
func sortByTimestamp(msgs []map[string]interface{}) {
	sort.SliceStable(msgs, func(i, j int) bool {
		ti, iok := msgs[i]["timestamp"].(string)
		tj, jok := msgs[j]["timestamp"].(string)
		if iok && jok {
			pi, ei := time.Parse(time.RFC3339Nano, ti)
			pj, ej := time.Parse(time.RFC3339Nano, tj)
			if ei == nil && ej == nil {
				return pi.Before(pj)
			}
			return ti < tj
		}
		return false
	})
}

// render writes each message to w in the requested format.
func render(w io.Writer, msgs []map[string]interface{}, format string, fields []string) error {
	for _, msg := range msgs {
		switch format {
		case "json":
			s, err := graylog.FormatJSON(msg)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(w, s); err != nil {
				return err
			}
		case "fields":
			if _, err := fmt.Fprintln(w, graylog.FormatFields(msg, fields)); err != nil {
				return err
			}
		default: // line
			if _, err := fmt.Fprintln(w, graylog.FormatLine(msg)); err != nil {
				return err
			}
		}
	}
	return nil
}
