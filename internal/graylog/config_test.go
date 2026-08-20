package graylog

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\n\nPLAIN=value1\nQUOTED=\"value2\"\nSINGLE='value3'\nPRESET=fromfile\nnoequals\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp env: %v", err)
	}

	// PRESET is already set in the environment and must not be overwritten.
	t.Setenv("PRESET", "fromenv")
	t.Setenv("PLAIN", "")
	_ = os.Unsetenv("PLAIN")
	t.Setenv("QUOTED", "")
	_ = os.Unsetenv("QUOTED")
	t.Setenv("SINGLE", "")
	_ = os.Unsetenv("SINGLE")

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile error: %v", err)
	}

	if got := os.Getenv("PLAIN"); got != "value1" {
		t.Errorf("PLAIN = %q, want %q", got, "value1")
	}
	if got := os.Getenv("QUOTED"); got != "value2" {
		t.Errorf("QUOTED = %q, want %q", got, "value2")
	}
	if got := os.Getenv("SINGLE"); got != "value3" {
		t.Errorf("SINGLE = %q, want %q", got, "value3")
	}
	if got := os.Getenv("PRESET"); got != "fromenv" {
		t.Errorf("PRESET = %q, want preserved %q", got, "fromenv")
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	err := LoadEnvFile(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected IsNotExist error, got %v", err)
	}
}

func TestRegisterCommonFlagsDefaults(t *testing.T) {
	for _, k := range []string{"GRAYLOG_URL", "GRAYLOG_API_TOKEN", "GRAYLOG_STREAM_ID", "TIMEOUT", "INSECURE", "LIMIT"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := RegisterCommonFlags(fs)
	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if cfg.URL != "" || cfg.Token != "" || cfg.StreamID != "" {
		t.Errorf("expected empty URL/Token/StreamID, got %+v", cfg)
	}
	if cfg.Timeout != 60*time.Second {
		t.Errorf("Timeout = %v, want 60s", cfg.Timeout)
	}
	if cfg.Insecure {
		t.Error("Insecure = true, want false")
	}
	if cfg.Limit != 100 {
		t.Errorf("Limit = %d, want 100", cfg.Limit)
	}
}

func TestRegisterCommonFlagsEnv(t *testing.T) {
	t.Setenv("GRAYLOG_URL", "https://env.example:9000")
	t.Setenv("GRAYLOG_API_TOKEN", "env-token")
	t.Setenv("LIMIT", "250")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := RegisterCommonFlags(fs)
	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if cfg.URL != "https://env.example:9000" {
		t.Errorf("URL = %q, want env value", cfg.URL)
	}
	if cfg.Token != "env-token" {
		t.Errorf("Token = %q, want env value", cfg.Token)
	}
	if cfg.Limit != 250 {
		t.Errorf("Limit = %d, want 250", cfg.Limit)
	}
}

func TestRegisterCommonFlagsFlagOverridesEnv(t *testing.T) {
	t.Setenv("GRAYLOG_URL", "https://env.example:9000")
	t.Setenv("GRAYLOG_API_TOKEN", "env-token")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := RegisterCommonFlags(fs)
	if err := fs.Parse([]string{"-url", "https://flag.example:9000"}); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if cfg.URL != "https://flag.example:9000" {
		t.Errorf("URL = %q, want flag override", cfg.URL)
	}
	if cfg.Token != "env-token" {
		t.Errorf("Token = %q, want env fallback", cfg.Token)
	}
}

func TestValidate(t *testing.T) {
	if err := (&Config{URL: "u", Token: "t"}).Validate(); err != nil {
		t.Errorf("valid config returned error: %v", err)
	}
	if err := (&Config{Token: "t"}).Validate(); err == nil {
		t.Error("expected error for missing URL")
	}
	if err := (&Config{URL: "u"}).Validate(); err == nil {
		t.Error("expected error for missing token")
	}
}

func TestApplyDefaults(t *testing.T) {
	c := &Config{}
	c.ApplyDefaults()
	if c.StreamID != DefaultStreamID {
		t.Errorf("StreamID = %q, want %q", c.StreamID, DefaultStreamID)
	}

	c = &Config{StreamID: "custom"}
	c.ApplyDefaults()
	if c.StreamID != "custom" {
		t.Errorf("StreamID = %q, want preserved custom", c.StreamID)
	}
}
