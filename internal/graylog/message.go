package graylog

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Response models the subset of the Graylog universal search API response that
// gotail consumes: a list of message envelopes each wrapping the raw message
// fields.
type Response struct {
	Messages []struct {
		Message map[string]interface{} `json:"message"`
	} `json:"messages"`
}

// FormatLine renders a message in the classic gotail form
// "[timestamp] source: message". Missing fields render as "<nil>", preserving
// the original behavior.
func FormatLine(msg map[string]interface{}) string {
	return fmt.Sprintf("[%v] %v: %v", msg["timestamp"], msg["source"], msg["message"])
}

// textPrimaryFields are rendered first, in this order, on the header line of
// FormatText; every other field follows, sorted, indented beneath it.
var textPrimaryFields = []string{"timestamp", "source", "message"}

// FormatText renders a message in full: the classic "[timestamp] source:
// message" header line followed by every remaining field, sorted, indented
// beneath it. This is the full-fidelity text view of a GELF message.
func FormatText(msg map[string]interface{}) string {
	var b strings.Builder
	b.WriteString(FormatLine(msg))

	primary := make(map[string]bool, len(textPrimaryFields))
	for _, f := range textPrimaryFields {
		primary[f] = true
	}

	rest := make([]string, 0, len(msg))
	for k := range msg {
		if !primary[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)

	for _, k := range rest {
		fmt.Fprintf(&b, "\n    %s: %v", k, msg[k])
	}
	return b.String()
}

// FormatJSON renders the raw message map as compact JSON.
func FormatJSON(msg map[string]interface{}) (string, error) {
	b, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// FormatFields renders the requested fields, in order, as tab-separated values.
// Missing fields render as an empty column.
func FormatFields(msg map[string]interface{}, fields []string) string {
	cols := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if v, ok := msg[f]; ok {
			cols = append(cols, fmt.Sprintf("%v", v))
		} else {
			cols = append(cols, "")
		}
	}
	return strings.Join(cols, "\t")
}

// MessageKey builds a stable deduplication key from the identifying fields of a
// message.
func MessageKey(msg map[string]interface{}) string {
	return fmt.Sprintf("%v|%v|%v", msg["timestamp"], msg["source"], msg["message"])
}

// ParseFields splits a comma-separated field list, trimming surrounding
// whitespace from each entry and dropping empty ones. It returns nil for an
// empty or whitespace-only input.
func ParseFields(csv string) []string {
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

// ValidateFormat reports whether format is one of the supported output formats
// ("line", "text", "json", "fields"). The "fields" format additionally requires
// a non-empty fields list.
func ValidateFormat(format string, fields []string) error {
	switch format {
	case "line", "text", "json":
		return nil
	case "fields":
		if len(fields) == 0 {
			return errors.New("--format fields requires a non-empty --fields list")
		}
		return nil
	default:
		return fmt.Errorf("unknown --format %q (want line, text, json, or fields)", format)
	}
}

// FormatMessage renders a single message in the given format ("line", "text",
// "json", or "fields"); for "fields", fields selects and orders the columns. An
// unknown format falls back to "line".
func FormatMessage(msg map[string]interface{}, format string, fields []string) (string, error) {
	switch format {
	case "text":
		return FormatText(msg), nil
	case "json":
		return FormatJSON(msg)
	case "fields":
		return FormatFields(msg, fields), nil
	default: // line
		return FormatLine(msg), nil
	}
}
