package local

import (
	"testing"
	"time"
)

func TestNewParser(t *testing.T) {
	tests := []struct {
		name     string
		format   Format
		wantType string
	}{
		{"plain", FormatPlain, "*local.PlainParser"},
		{"json", FormatJSON, "*local.JSONParser"},
		{"syslog", FormatSyslog, "*local.SyslogParser"},
		{"java", FormatJava, "*local.JavaParser"},
		{"unknown", Format(99), "*local.PlainParser"}, // unknown format defaults to plain
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewParser(tt.format)
			gotType := getTypeName(p)
			if gotType != tt.wantType {
				t.Errorf("NewParser(%v) = %s, want %s", tt.format, gotType, tt.wantType)
			}
		})
	}
}

func getTypeName(p Parser) string {
	switch p.(type) {
	case *PlainParser:
		return "*local.PlainParser"
	case *JSONParser:
		return "*local.JSONParser"
	case *SyslogParser:
		return "*local.SyslogParser"
	case *JavaParser:
		return "*local.JavaParser"
	default:
		return "unknown"
	}
}

// PlainParser tests

func TestPlainParser_ParseLine(t *testing.T) {
	p := &PlainParser{}

	tests := []struct {
		name     string
		line     string
		lineNum  int
		filePath string
		wantNil  bool
		wantMsg  string
	}{
		{
			name:     "basic line",
			line:     "Hello, World!",
			lineNum:  1,
			filePath: "/var/log/app.log",
			wantNil:  false,
			wantMsg:  "Hello, World!",
		},
		{
			name:     "empty line",
			line:     "",
			lineNum:  1,
			filePath: "/var/log/app.log",
			wantNil:  true,
		},
		{
			name:     "line with special chars",
			line:     "Error: connection failed @ 10.0.0.1:8080",
			lineNum:  42,
			filePath: "/logs/server.log",
			wantNil:  false,
			wantMsg:  "Error: connection failed @ 10.0.0.1:8080",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := p.ParseLine(tt.line, tt.lineNum, tt.filePath)
			if tt.wantNil {
				if entry != nil {
					t.Errorf("expected nil, got %v", entry)
				}
				return
			}
			if entry == nil {
				t.Fatal("expected non-nil entry")
			}
			if entry.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", entry.Message, tt.wantMsg)
			}
			if entry.Timestamp.IsZero() == false {
				t.Error("expected zero timestamp for plain parser")
			}
			if entry.Stream != "app.log" && entry.Stream != "server.log" {
				t.Errorf("Stream = %q, want basename of filepath", entry.Stream)
			}
			if entry.Ptr == "" {
				t.Error("expected non-empty Ptr")
			}
		})
	}
}

func TestPlainParser_Multiline(t *testing.T) {
	p := &PlainParser{}
	if p.IsMultiline() {
		t.Error("PlainParser should not be multiline")
	}
	if p.ShouldJoin("any line") {
		t.Error("PlainParser should never join lines")
	}
}

// JSONParser tests

func TestJSONParser_ParseLine(t *testing.T) {
	p := &JSONParser{}

	tests := []struct {
		name       string
		line       string
		wantNil    bool
		wantMsg    string
		wantFields map[string]string
		checkTime  bool
	}{
		{
			name:    "empty line",
			line:    "",
			wantNil: true,
		},
		{
			name:    "non-JSON line",
			line:    "plain text log",
			wantNil: true,
		},
		{
			name:    "basic JSON with message",
			line:    `{"message": "test message", "level": "info"}`,
			wantNil: false,
			wantMsg: "test message",
			wantFields: map[string]string{
				"level": "info",
			},
		},
		{
			name:    "JSON with msg field",
			line:    `{"msg": "test message", "service": "api"}`,
			wantNil: false,
			wantMsg: "test message",
			wantFields: map[string]string{
				"service": "api",
			},
		},
		{
			name:      "JSON with RFC3339 timestamp",
			line:      `{"timestamp": "2025-01-15T10:30:45Z", "message": "event"}`,
			wantNil:   false,
			wantMsg:   "event",
			checkTime: true,
		},
		{
			name:      "JSON with Unix timestamp seconds",
			line:      `{"ts": 1705315845, "message": "event"}`,
			wantNil:   false,
			wantMsg:   "event",
			checkTime: true,
		},
		{
			name:      "JSON with Unix timestamp milliseconds",
			line:      `{"time": 1705315845000, "message": "event"}`,
			wantNil:   false,
			wantMsg:   "event",
			checkTime: true,
		},
		{
			name:    "JSON with numeric fields",
			line:    `{"message": "test", "count": 42, "ratio": 3.14}`,
			wantNil: false,
			wantMsg: "test",
			wantFields: map[string]string{
				"count": "42",
				"ratio": "3.14",
			},
		},
		{
			name:    "JSON with boolean field",
			line:    `{"message": "test", "enabled": true}`,
			wantNil: false,
			wantMsg: "test",
			wantFields: map[string]string{
				"enabled": "true",
			},
		},
		{
			name:    "JSON with nested object",
			line:    `{"message": "test", "meta": {"key": "value"}}`,
			wantNil: false,
			wantMsg: "test",
			wantFields: map[string]string{
				"meta": `{"key":"value"}`,
			},
		},
		{
			name:    "JSON without message field",
			line:    `{"event": "startup", "pid": 1234}`,
			wantNil: false,
			wantMsg: `{"event": "startup", "pid": 1234}`, // entire JSON as message
		},
		{
			name:    "invalid JSON",
			line:    `{"broken": json`,
			wantNil: false,
			wantMsg: `{"broken": json`, // treat as plain text
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := p.ParseLine(tt.line, 1, "/var/log/app.log")
			if tt.wantNil {
				if entry != nil {
					t.Errorf("expected nil, got %v", entry)
				}
				return
			}
			if entry == nil {
				t.Fatal("expected non-nil entry")
			}
			if entry.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", entry.Message, tt.wantMsg)
			}
			if tt.checkTime && entry.Timestamp.IsZero() {
				t.Error("expected non-zero timestamp")
			}
			for k, v := range tt.wantFields {
				if entry.Fields[k] != v {
					t.Errorf("Fields[%q] = %q, want %q", k, entry.Fields[k], v)
				}
			}
		})
	}
}

func TestJSONParser_Multiline(t *testing.T) {
	p := &JSONParser{}
	if p.IsMultiline() {
		t.Error("JSONParser should not be multiline")
	}
	if p.ShouldJoin("any line") {
		t.Error("JSONParser should never join lines")
	}
}

// SyslogParser tests

func TestSyslogParser_RFC5424(t *testing.T) {
	p := &SyslogParser{}

	line := "<134>1 2025-01-15T10:30:45.123456Z myhost myapp 1234 ID47 - Application started"
	entry := p.ParseLine(line, 1, "/var/log/syslog")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Message != "- Application started" {
		t.Errorf("Message = %q, want %q", entry.Message, "- Application started")
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if entry.Fields["hostname"] != "myhost" {
		t.Errorf("hostname = %q, want %q", entry.Fields["hostname"], "myhost")
	}
	if entry.Fields["app"] != "myapp" {
		t.Errorf("app = %q, want %q", entry.Fields["app"], "myapp")
	}
	if entry.Fields["facility"] != "16" { // 134/8 = 16
		t.Errorf("facility = %q, want %q", entry.Fields["facility"], "16")
	}
	if entry.Fields["severity"] != "6" { // 134%8 = 6
		t.Errorf("severity = %q, want %q", entry.Fields["severity"], "6")
	}
}

func TestSyslogParser_RFC3164(t *testing.T) {
	p := &SyslogParser{}

	line := "Jan 15 10:30:45 myhost sshd[1234]: Accepted password for user"
	entry := p.ParseLine(line, 1, "/var/log/auth.log")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Message != "Accepted password for user" {
		t.Errorf("Message = %q, want %q", entry.Message, "Accepted password for user")
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if entry.Timestamp.Month() != time.January || entry.Timestamp.Day() != 15 {
		t.Errorf("timestamp date wrong: got %v", entry.Timestamp)
	}
	if entry.Fields["hostname"] != "myhost" {
		t.Errorf("hostname = %q, want %q", entry.Fields["hostname"], "myhost")
	}
	if entry.Fields["program"] != "sshd" {
		t.Errorf("program = %q, want %q", entry.Fields["program"], "sshd")
	}
	if entry.Fields["pid"] != "1234" {
		t.Errorf("pid = %q, want %q", entry.Fields["pid"], "1234")
	}
}

func TestSyslogParser_RFC3164_NoPID(t *testing.T) {
	p := &SyslogParser{}

	line := "Dec  5 08:15:30 server kernel: CPU0: Temperature above threshold"
	entry := p.ParseLine(line, 1, "/var/log/messages")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Fields["program"] != "kernel" {
		t.Errorf("program = %q, want %q", entry.Fields["program"], "kernel")
	}
	if entry.Fields["pid"] != "" {
		t.Errorf("pid should be empty, got %q", entry.Fields["pid"])
	}
}

func TestSyslogParser_Fallback(t *testing.T) {
	p := &SyslogParser{}

	line := "This is not a syslog message"
	entry := p.ParseLine(line, 1, "/var/log/app.log")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Message != line {
		t.Errorf("Message = %q, want %q", entry.Message, line)
	}
}

func TestSyslogParser_EmptyLine(t *testing.T) {
	p := &SyslogParser{}
	entry := p.ParseLine("", 1, "/var/log/syslog")
	if entry != nil {
		t.Error("expected nil for empty line")
	}
}

func TestSyslogParser_Multiline(t *testing.T) {
	p := &SyslogParser{}
	if p.IsMultiline() {
		t.Error("SyslogParser should not be multiline")
	}
}

// JavaParser tests

func TestJavaParser_Log4jFormat(t *testing.T) {
	p := &JavaParser{}

	line := "2025-01-15 10:30:45,123 INFO [main] com.example.App - Application started"
	entry := p.ParseLine(line, 1, "/var/log/app.log")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Message != "Application started" {
		t.Errorf("Message = %q, want %q", entry.Message, "Application started")
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if entry.Fields["level"] != "INFO" {
		t.Errorf("level = %q, want %q", entry.Fields["level"], "INFO")
	}
	if entry.Fields["thread"] != "main" {
		t.Errorf("thread = %q, want %q", entry.Fields["thread"], "main")
	}
	if entry.Fields["logger"] != "com.example.App" {
		t.Errorf("logger = %q, want %q", entry.Fields["logger"], "com.example.App")
	}
}

func TestJavaParser_Log4j2Format(t *testing.T) {
	p := &JavaParser{}

	line := "2025-01-15 10:30:45.123 [worker-1] ERROR com.example.Service - Connection failed"
	entry := p.ParseLine(line, 1, "/var/log/app.log")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Message != "Connection failed" {
		t.Errorf("Message = %q, want %q", entry.Message, "Connection failed")
	}
	if entry.Fields["level"] != "ERROR" {
		t.Errorf("level = %q, want %q", entry.Fields["level"], "ERROR")
	}
	if entry.Fields["thread"] != "worker-1" {
		t.Errorf("thread = %q, want %q", entry.Fields["thread"], "worker-1")
	}
}

func TestJavaParser_SimpleFormat(t *testing.T) {
	p := &JavaParser{}

	line := "2025-01-15 10:30:45,123 WARN Configuration not found"
	entry := p.ParseLine(line, 1, "/var/log/app.log")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Message != "Configuration not found" {
		t.Errorf("Message = %q, want %q", entry.Message, "Configuration not found")
	}
	if entry.Fields["level"] != "WARN" {
		t.Errorf("level = %q, want %q", entry.Fields["level"], "WARN")
	}
}

func TestJavaParser_ISOFormat(t *testing.T) {
	p := &JavaParser{}

	line := "2025-01-15T10:30:45.123Z DEBUG Processing request"
	entry := p.ParseLine(line, 1, "/var/log/app.log")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Message != "Processing request" {
		t.Errorf("Message = %q, want %q", entry.Message, "Processing request")
	}
	if entry.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	if entry.Fields["level"] != "DEBUG" {
		t.Errorf("level = %q, want %q", entry.Fields["level"], "DEBUG")
	}
}

func TestJavaParser_Fallback(t *testing.T) {
	p := &JavaParser{}

	line := "Some unstructured log message"
	entry := p.ParseLine(line, 1, "/var/log/app.log")

	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Message != line {
		t.Errorf("Message = %q, want %q", entry.Message, line)
	}
}

func TestJavaParser_EmptyLine(t *testing.T) {
	p := &JavaParser{}
	entry := p.ParseLine("", 1, "/var/log/app.log")
	if entry != nil {
		t.Error("expected nil for empty line")
	}
}

func TestJavaParser_IsMultiline(t *testing.T) {
	p := &JavaParser{}
	if !p.IsMultiline() {
		t.Error("JavaParser should be multiline")
	}
}

func TestJavaParser_ShouldJoin(t *testing.T) {
	p := &JavaParser{}

	tests := []struct {
		name     string
		line     string
		wantJoin bool
	}{
		{
			name:     "empty line",
			line:     "",
			wantJoin: true, // Blank lines preserved in multiline messages
		},
		{
			name:     "stack frame with tab",
			line:     "\tat com.example.App.main(App.java:10)",
			wantJoin: true,
		},
		{
			name:     "stack frame with spaces",
			line:     "    at com.example.App.main(App.java:10)",
			wantJoin: true,
		},
		{
			name:     "suppressed frames",
			line:     "\t... 15 more",
			wantJoin: true,
		},
		{
			name:     "caused by",
			line:     "Caused by: java.io.IOException: File not found",
			wantJoin: true,
		},
		{
			name:     "suppressed exception",
			line:     "Suppressed: java.lang.RuntimeException: cleanup failed",
			wantJoin: true,
		},
		{
			name:     "exception class name",
			line:     "java.lang.NullPointerException: value was null",
			wantJoin: true,
		},
		{
			name:     "exception class with Error suffix",
			line:     "java.lang.OutOfMemoryError: Java heap space",
			wantJoin: true,
		},
		{
			name:     "custom exception",
			line:     "com.example.app.ValidationException: Invalid input",
			wantJoin: true,
		},
		{
			name:     "regular log line",
			line:     "2025-01-15 10:30:45,123 INFO [main] App - Starting",
			wantJoin: false,
		},
		{
			name:     "plain text",
			line:     "Some regular message",
			wantJoin: true, // Non-timestamp lines are continuations
		},
		{
			name:     "at without indent",
			line:     "at the store I bought something",
			wantJoin: true, // Non-timestamp lines are continuations
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.ShouldJoin(tt.line)
			if got != tt.wantJoin {
				t.Errorf("ShouldJoin(%q) = %v, want %v", tt.line, got, tt.wantJoin)
			}
		})
	}
}

// parseJSONTimestamp tests

func TestParseJSONTimestamp(t *testing.T) {
	tests := []struct {
		name      string
		val       interface{}
		wantZero  bool
		wantYear  int
		wantMonth time.Month
		wantDay   int
	}{
		{
			name:      "RFC3339",
			val:       "2025-01-15T10:30:45Z",
			wantZero:  false,
			wantYear:  2025,
			wantMonth: time.January,
			wantDay:   15,
		},
		{
			name:      "RFC3339Nano",
			val:       "2025-01-15T10:30:45.123456789Z",
			wantZero:  false,
			wantYear:  2025,
			wantMonth: time.January,
			wantDay:   15,
		},
		{
			name:      "date space time",
			val:       "2025-01-15 10:30:45",
			wantZero:  false,
			wantYear:  2025,
			wantMonth: time.January,
			wantDay:   15,
		},
		{
			name:      "Unix seconds",
			val:       float64(1705315845),
			wantZero:  false,
			wantYear:  2024, // Unix timestamp for Jan 15, 2024
			wantMonth: time.January,
			wantDay:   15,
		},
		{
			name:      "Unix milliseconds",
			val:       float64(1705315845000),
			wantZero:  false,
			wantYear:  2024,
			wantMonth: time.January,
			wantDay:   15,
		},
		{
			name:     "invalid string",
			val:      "not a timestamp",
			wantZero: true,
		},
		{
			name:     "nil",
			val:      nil,
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := parseJSONTimestamp(tt.val)
			if tt.wantZero {
				if !ts.IsZero() {
					t.Errorf("expected zero time, got %v", ts)
				}
				return
			}
			if ts.IsZero() {
				t.Error("expected non-zero time")
				return
			}
			if ts.Year() != tt.wantYear {
				t.Errorf("year = %d, want %d", ts.Year(), tt.wantYear)
			}
			if ts.Month() != tt.wantMonth {
				t.Errorf("month = %v, want %v", ts.Month(), tt.wantMonth)
			}
			if ts.Day() != tt.wantDay {
				t.Errorf("day = %d, want %d", ts.Day(), tt.wantDay)
			}
		})
	}
}

// parseJavaTimestamp tests

func TestParseJavaTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		datePart string
		timePart string
		wantZero bool
		wantYear int
	}{
		{
			name:     "comma separator",
			datePart: "2025-01-15",
			timePart: "10:30:45,123",
			wantZero: false,
			wantYear: 2025,
		},
		{
			name:     "dot separator",
			datePart: "2025-01-15",
			timePart: "10:30:45.123",
			wantZero: false,
			wantYear: 2025,
		},
		{
			name:     "invalid format",
			datePart: "invalid",
			timePart: "invalid",
			wantZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := parseJavaTimestamp(tt.datePart, tt.timePart)
			if tt.wantZero {
				if !ts.IsZero() {
					t.Errorf("expected zero time, got %v", ts)
				}
				return
			}
			if ts.IsZero() {
				t.Error("expected non-zero time")
				return
			}
			if ts.Year() != tt.wantYear {
				t.Errorf("year = %d, want %d", ts.Year(), tt.wantYear)
			}
		})
	}
}
