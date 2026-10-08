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

func TestFormatText(t *testing.T) {
	msg := map[string]interface{}{
		"timestamp": "2024-01-02T15:04:05.000Z",
		"source":    "host1",
		"message":   "hello world",
		"level":     6,
		"facility":  "gelf",
	}
	got := FormatText(msg)
	want := "[2024-01-02T15:04:05.000Z] host1: hello world" +
		"\n    facility: gelf" +
		"\n    level: 6"
	if got != want {
		t.Fatalf("FormatText = %q, want %q", got, want)
	}
}

func TestFormatTextNoExtraFields(t *testing.T) {
	msg := map[string]interface{}{
		"timestamp": "ts",
		"source":    "host1",
		"message":   "hi",
	}
	got := FormatText(msg)
	want := "[ts] host1: hi"
	if got != want {
		t.Fatalf("FormatText = %q, want %q", got, want)
	}
}

func TestValidateFormat(t *testing.T) {
	for _, f := range []string{"line", "text", "json"} {
		if err := ValidateFormat(f, nil); err != nil {
			t.Errorf("ValidateFormat(%q, nil) = %v, want nil", f, err)
		}
	}
	if err := ValidateFormat("fields", []string{"a"}); err != nil {
		t.Errorf("ValidateFormat(fields, [a]) = %v, want nil", err)
	}
	if err := ValidateFormat("fields", nil); err == nil {
		t.Error("ValidateFormat(fields, nil) should error")
	}
	if err := ValidateFormat("bogus", nil); err == nil {
		t.Error("ValidateFormat(bogus, nil) should error")
	}
}

func TestFormatMessage(t *testing.T) {
	msg := map[string]interface{}{
		"timestamp": "ts",
		"source":    "host1",
		"message":   "hi",
	}
	cases := map[string]string{
		"line":    "[ts] host1: hi",
		"text":    "[ts] host1: hi",
		"unknown": "[ts] host1: hi",
	}
	for format, want := range cases {
		got, err := FormatMessage(msg, format, nil)
		if err != nil {
			t.Fatalf("FormatMessage(%q) error: %v", format, err)
		}
		if got != want {
			t.Errorf("FormatMessage(%q) = %q, want %q", format, got, want)
		}
	}

	got, err := FormatMessage(msg, "fields", []string{"source", "message"})
	if err != nil {
		t.Fatalf("FormatMessage(fields) error: %v", err)
	}
	if got != "host1\thi" {
		t.Errorf("FormatMessage(fields) = %q, want %q", got, "host1\thi")
	}

	js, err := FormatMessage(msg, "json", nil)
	if err != nil {
		t.Fatalf("FormatMessage(json) error: %v", err)
	}
	var round map[string]interface{}
	if err := json.Unmarshal([]byte(js), &round); err != nil {
		t.Fatalf("FormatMessage(json) produced invalid JSON %q: %v", js, err)
	}
}

func TestParseFields(t *testing.T) {
	if got := ParseFields("   "); got != nil {
		t.Errorf("ParseFields(blank) = %v, want nil", got)
	}
	got := ParseFields(" source , , message ")
	if len(got) != 2 || got[0] != "source" || got[1] != "message" {
		t.Errorf("ParseFields = %v, want [source message]", got)
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
