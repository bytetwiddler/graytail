package graylog

import (
	"encoding/json"
	"fmt"
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
