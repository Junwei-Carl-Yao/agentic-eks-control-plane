package logging

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestConfigureEmitsRequiredFields(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := Configure(&logBuffer)
	logger.Info("hello")

	var logEntry map[string]any
	if err := json.Unmarshal(logBuffer.Bytes(), &logEntry); err != nil {
		t.Fatalf("invalid json: %v\nraw=%s", err, logBuffer.String())
	}
	if logEntry["level"] != "INFO" {
		t.Errorf("level = %v, want INFO", logEntry["level"])
	}
	if logEntry["msg"] != "hello" {
		t.Errorf("msg = %v, want hello", logEntry["msg"])
	}
	if _, hasTimestamp := logEntry["ts"]; !hasTimestamp {
		t.Errorf("missing ts; raw=%s", logBuffer.String())
	}
}

func TestConfigureIncludesAttrs(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := Configure(&logBuffer)
	logger.Info("hello", "user", "alice")

	var logEntry map[string]any
	if err := json.Unmarshal(logBuffer.Bytes(), &logEntry); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if logEntry["user"] != "alice" {
		t.Errorf("user = %v, want alice", logEntry["user"])
	}
}
