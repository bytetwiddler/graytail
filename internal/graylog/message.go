package graylog

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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

	for _, k := range sortedKeysExcept(msg, textPrimaryFields) {
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

// tsvEscaper backslash-escapes the characters that would otherwise split a
// tab-separated record across columns or lines.
var tsvEscaper = strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`)

// FormatFields renders the requested fields, in order, as tab-separated values.
// Missing fields render as an empty column. Backslashes, newlines, carriage
// returns and tabs within values are escaped as \\, \n, \r and \t so each
// message stays on one line with a fixed number of columns.
func FormatFields(msg map[string]interface{}, fields []string) string {
	cols := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if v, ok := msg[f]; ok {
			cols = append(cols, tsvEscaper.Replace(fmt.Sprintf("%v", v)))
		} else {
			cols = append(cols, "")
		}
	}
	return strings.Join(cols, "\t")
}

// FormatLogfmt renders a message as space-separated key=value pairs. With a
// fields list, exactly those keys are rendered in order, a missing field
// rendering as "key=". Without one, timestamp, source and message come first,
// followed by every remaining field, sorted.
func FormatLogfmt(msg map[string]interface{}, fields []string) string {
	keys := fields
	if len(keys) == 0 {
		keys = make([]string, 0, len(msg))
		for _, f := range textPrimaryFields {
			if _, ok := msg[f]; ok {
				keys = append(keys, f)
			}
		}
		keys = append(keys, sortedKeysExcept(msg, textPrimaryFields)...)
	}

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		v := ""
		if raw, ok := msg[k]; ok && raw != nil {
			v = fmt.Sprintf("%v", raw)
		}
		pairs = append(pairs, k+"="+logfmtValue(v))
	}
	return strings.Join(pairs, " ")
}

// logfmtValue returns v unchanged when it can stand bare, and as a Go-quoted
// string (escaping quotes, backslashes and control characters) when it
// contains a space, '=', '"' or a non-printable character.
func logfmtValue(v string) string {
	for _, r := range v {
		if r == ' ' || r == '=' || r == '"' || r == '\\' || !strconv.IsPrint(r) {
			return strconv.Quote(v)
		}
	}
	return v
}

// sortedKeysExcept returns the keys of msg not in exclude, sorted.
func sortedKeysExcept(msg map[string]interface{}, exclude []string) []string {
	skip := make(map[string]bool, len(exclude))
	for _, f := range exclude {
		skip[f] = true
	}
	keys := make([]string, 0, len(msg))
	for k := range msg {
		if !skip[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
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

// ResolveFormat returns the effective output format. A non-empty fields list
// implies "fields" unless a format was explicitly given on the command line
// (explicit), in which case format is returned unchanged.
func ResolveFormat(format string, explicit bool, fields []string) string {
	if !explicit && len(fields) > 0 {
		return "fields"
	}
	return format
}

// ValidateFormat reports whether format is one of the supported output formats
// ("line", "text", "json", "fields", "logfmt"). The "fields" format
// additionally requires a non-empty fields list, which "logfmt" also accepts;
// the other formats reject one, since it would be silently ignored.
func ValidateFormat(format string, fields []string) error {
	switch format {
	case "line", "text", "json":
		if len(fields) > 0 {
			return fmt.Errorf("--fields requires --format fields or logfmt (got --format %s)", format)
		}
		return nil
	case "fields":
		if len(fields) == 0 {
			return errors.New("--format fields requires a non-empty --fields list")
		}
		return nil
	case "logfmt":
		return nil
	default:
		return fmt.Errorf("unknown --format %q (want line, text, json, fields, or logfmt)", format)
	}
}

// FormatMessage renders a single message in the given format ("line", "text",
// "json", "fields", or "logfmt"); for "fields" and "logfmt", fields selects and
// orders the output. An unknown format falls back to "line".
func FormatMessage(msg map[string]interface{}, format string, fields []string) (string, error) {
	switch format {
	case "text":
		return FormatText(msg), nil
	case "json":
		return FormatJSON(msg)
	case "fields":
		return FormatFields(msg, fields), nil
	case "logfmt":
		return FormatLogfmt(msg, fields), nil
	default: // line
		return FormatLine(msg), nil
	}
}
