package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestConfigureEmitsRequiredFields(t *testing.T) {
	var buf bytes.Buffer
	logger := Configure("INFO", &buf)
	logger.Info("hello")

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v\nraw=%s", err, buf.String())
	}
	if out["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", out["level"])
	}
	if out["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", out["msg"])
	}
	if _, ok := out["ts"]; !ok {
		t.Errorf("missing ts; raw=%s", buf.String())
	}
}

func TestConfigureIncludesAttrs(t *testing.T) {
	var buf bytes.Buffer
	logger := Configure("INFO", &buf)
	logger.Info("hello", "user", "alice")

	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if out["user"] != "alice" {
		t.Errorf("user = %v, want alice", out["user"])
	}
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{" Warn ", slog.LevelWarn},
		{"WARNING", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"INFO", slog.LevelInfo},
		{"garbage", slog.LevelInfo},
		{"", slog.LevelInfo},
	}
	for _, c := range cases {
		if got := ParseLevel(c.in); got != c.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
