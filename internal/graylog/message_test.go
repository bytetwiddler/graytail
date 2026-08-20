package graylog

import (
	"encoding/json"
	"testing"
)

func TestFormatLine(t *testing.T) {
	msg := map[string]interface{}{
		"timestamp": "2024-01-02T15:04:05.000Z",
		"source":    "host1",
		"message":   "hello world",
	}
	got := FormatLine(msg)
	want := "[2024-01-02T15:04:05.000Z] host1: hello world"
	if got != want {
		t.Fatalf("FormatLine = %q, want %q", got, want)
	}
}

func TestFormatLineMissingFields(t *testing.T) {
	got := FormatLine(map[string]interface{}{})
	want := "[<nil>] <nil>: <nil>"
	if got != want {
		t.Fatalf("FormatLine (missing) = %q, want %q", got, want)
	}
}

func TestFormatJSON(t *testing.T) {
	msg := map[string]interface{}{"source": "host1", "message": "hi"}
	got, err := FormatJSON(msg)
	if err != nil {
		t.Fatalf("FormatJSON error: %v", err)
	}
	var round map[string]interface{}
	if err := json.Unmarshal([]byte(got), &round); err != nil {
		t.Fatalf("FormatJSON produced invalid JSON %q: %v", got, err)
	}
	if round["source"] != "host1" || round["message"] != "hi" {
		t.Fatalf("FormatJSON round-trip mismatch: %v", round)
	}
}

func TestFormatFields(t *testing.T) {
	msg := map[string]interface{}{
		"timestamp": "ts",
		"source":    "host1",
		"message":   "hi",
	}
	got := FormatFields(msg, []string{"source", "missing", "message"})
	want := "host1\t\thi"
	if got != want {
		t.Fatalf("FormatFields = %q, want %q", got, want)
	}
}

func TestMessageKey(t *testing.T) {
	msg := map[string]interface{}{
		"timestamp": "ts",
		"source":    "host1",
		"message":   "hi",
	}
	got := MessageKey(msg)
	want := "ts|host1|hi"
	if got != want {
		t.Fatalf("MessageKey = %q, want %q", got, want)
	}
}
