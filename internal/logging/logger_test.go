package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.level.String(); got != tt.want {
				t.Errorf("Level.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoggerLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOutput(&buf)
	logger.SetLevel(LevelWarn)

	// Debug and Info should be filtered out
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	output := buf.String()

	if strings.Contains(output, "DEBUG") {
		t.Error("DEBUG message should be filtered out")
	}
	if strings.Contains(output, "INFO") {
		t.Error("INFO message should be filtered out")
	}
	if !strings.Contains(output, "WARN") {
		t.Error("WARN message should be present")
	}
	if !strings.Contains(output, "ERROR") {
		t.Error("ERROR message should be present")
	}
}

func TestLoggerFormatting(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOutput(&buf)
	logger.SetLevel(LevelDebug)

	logger.Info("user %s logged in", "alice")

	output := buf.String()
	if !strings.Contains(output, "[INFO]") {
		t.Error("expected [INFO] prefix")
	}
	if !strings.Contains(output, "user alice logged in") {
		t.Errorf("expected formatted message, got: %s", output)
	}
}

func TestLoggerWithField(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOutput(&buf)
	logger.SetLevel(LevelDebug)

	logger.WithField("request_id", "abc123").Info("handling request")

	output := buf.String()
	if !strings.Contains(output, "request_id=abc123") {
		t.Errorf("expected field in output, got: %s", output)
	}
}

func TestLoggerWithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOutput(&buf)
	logger.SetLevel(LevelDebug)

	fields := map[string]interface{}{
		"user_id":    42,
		"request_id": "xyz",
	}
	logger.WithFields(fields).Info("processing")

	output := buf.String()
	if !strings.Contains(output, "user_id=42") {
		t.Errorf("expected user_id field, got: %s", output)
	}
	if !strings.Contains(output, "request_id=xyz") {
		t.Errorf("expected request_id field, got: %s", output)
	}
}

func TestLoggerChainedWithField(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithOutput(&buf)
	logger.SetLevel(LevelDebug)

	logger.WithField("a", 1).WithField("b", 2).Info("test")

	output := buf.String()
	if !strings.Contains(output, "a=1") {
		t.Errorf("expected a field, got: %s", output)
	}
	if !strings.Contains(output, "b=2") {
		t.Errorf("expected b field, got: %s", output)
	}
}

func TestNopLogger(t *testing.T) {
	// NopLogger should not panic and should return itself for chaining
	nop := NopLogger{}

	nop.Debug("test")
	nop.Info("test")
	nop.Warn("test")
	nop.Error("test")

	// Should return same nop logger for chaining
	chained := nop.WithField("key", "value")
	if _, ok := chained.(NopLogger); !ok {
		t.Error("WithField should return NopLogger")
	}

	chained = nop.WithFields(map[string]interface{}{"key": "value"})
	if _, ok := chained.(NopLogger); !ok {
		t.Error("WithFields should return NopLogger")
	}

	// Should not panic
	nop.SetLevel(LevelDebug)
	nop.SetOutput(&bytes.Buffer{})
}

func TestDefaultLogger(t *testing.T) {
	// Save and restore default logger
	original := Default()
	defer SetDefault(original)

	var buf bytes.Buffer
	testLogger := NewWithOutput(&buf)
	testLogger.SetLevel(LevelDebug)
	SetDefault(testLogger)

	// Use package-level functions
	Debug("debug test")
	Info("info test")
	Warn("warn test")
	Error("error test")

	output := buf.String()
	if !strings.Contains(output, "debug test") {
		t.Error("expected debug message")
	}
	if !strings.Contains(output, "info test") {
		t.Error("expected info message")
	}
	if !strings.Contains(output, "warn test") {
		t.Error("expected warn message")
	}
	if !strings.Contains(output, "error test") {
		t.Error("expected error message")
	}
}

func TestLoggerImmutability(t *testing.T) {
	var buf bytes.Buffer
	base := NewWithOutput(&buf)
	base.SetLevel(LevelDebug)

	// Create derived logger with field
	derived := base.WithField("source", "derived")

	// Log from base - should NOT have the field
	buf.Reset()
	base.Info("base message")
	if strings.Contains(buf.String(), "source=derived") {
		t.Error("base logger should not have derived field")
	}

	// Log from derived - should have the field
	buf.Reset()
	derived.Info("derived message")
	if !strings.Contains(buf.String(), "source=derived") {
		t.Error("derived logger should have field")
	}
}
