// Package logging provides a structured logging abstraction for clew.
// It provides a simple interface that can be backed by the standard log
// package initially, and extended to use slog or other structured loggers later.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Level represents a log level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Logger is the interface for structured logging.
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})

	// WithField returns a new logger with the given field added.
	WithField(key string, value interface{}) Logger

	// WithFields returns a new logger with the given fields added.
	WithFields(fields map[string]interface{}) Logger

	// SetLevel sets the minimum log level.
	SetLevel(level Level)

	// SetOutput sets the output writer.
	SetOutput(w io.Writer)
}

// defaultLogger is the package-level default logger.
var (
	defaultLogger Logger
	defaultMu     sync.RWMutex
)

func init() {
	defaultLogger = New()
}

// Default returns the default logger.
func Default() Logger {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultLogger
}

// SetDefault sets the default logger.
func SetDefault(l Logger) {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	defaultLogger = l
}

// Debug logs a debug message using the default logger.
func Debug(msg string, args ...interface{}) {
	Default().Debug(msg, args...)
}

// Info logs an info message using the default logger.
func Info(msg string, args ...interface{}) {
	Default().Info(msg, args...)
}

// Warn logs a warning message using the default logger.
func Warn(msg string, args ...interface{}) {
	Default().Warn(msg, args...)
}

// Error logs an error message using the default logger.
func Error(msg string, args ...interface{}) {
	Default().Error(msg, args...)
}

// stdLogger implements Logger using the standard library log package.
type stdLogger struct {
	logger *log.Logger
	level  Level
	fields map[string]interface{}
	mu     sync.RWMutex
}

// New creates a new standard library-based logger.
func New() Logger {
	return &stdLogger{
		logger: log.New(os.Stderr, "", log.LstdFlags),
		level:  LevelInfo, // Default to Info level
		fields: make(map[string]interface{}),
	}
}

// NewWithOutput creates a new logger with the specified output.
func NewWithOutput(w io.Writer) Logger {
	return &stdLogger{
		logger: log.New(w, "", log.LstdFlags),
		level:  LevelInfo,
		fields: make(map[string]interface{}),
	}
}

func (l *stdLogger) shouldLog(level Level) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return level >= l.level
}

func (l *stdLogger) log(level Level, msg string, args ...interface{}) {
	if !l.shouldLog(level) {
		return
	}

	// Format message
	formatted := msg
	if len(args) > 0 {
		formatted = fmt.Sprintf(msg, args...)
	}

	// Build log line
	prefix := fmt.Sprintf("[%s] ", level.String())

	// Add fields if any
	l.mu.RLock()
	fields := l.fields
	l.mu.RUnlock()

	if len(fields) > 0 {
		fieldStr := ""
		for k, v := range fields {
			if fieldStr != "" {
				fieldStr += " "
			}
			fieldStr += fmt.Sprintf("%s=%v", k, v)
		}
		l.logger.Printf("%s%s [%s]", prefix, formatted, fieldStr)
	} else {
		l.logger.Printf("%s%s", prefix, formatted)
	}
}

func (l *stdLogger) Debug(msg string, args ...interface{}) {
	l.log(LevelDebug, msg, args...)
}

func (l *stdLogger) Info(msg string, args ...interface{}) {
	l.log(LevelInfo, msg, args...)
}

func (l *stdLogger) Warn(msg string, args ...interface{}) {
	l.log(LevelWarn, msg, args...)
}

func (l *stdLogger) Error(msg string, args ...interface{}) {
	l.log(LevelError, msg, args...)
}

func (l *stdLogger) WithField(key string, value interface{}) Logger {
	l.mu.RLock()
	newFields := make(map[string]interface{}, len(l.fields)+1)
	for k, v := range l.fields {
		newFields[k] = v
	}
	l.mu.RUnlock()

	newFields[key] = value

	return &stdLogger{
		logger: l.logger,
		level:  l.level,
		fields: newFields,
	}
}

func (l *stdLogger) WithFields(fields map[string]interface{}) Logger {
	l.mu.RLock()
	newFields := make(map[string]interface{}, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}
	l.mu.RUnlock()

	for k, v := range fields {
		newFields[k] = v
	}

	return &stdLogger{
		logger: l.logger,
		level:  l.level,
		fields: newFields,
	}
}

func (l *stdLogger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *stdLogger) SetOutput(w io.Writer) {
	l.logger.SetOutput(w)
}

// NopLogger is a logger that discards all output.
// Useful for testing or when logging should be disabled.
type NopLogger struct{}

func (NopLogger) Debug(msg string, args ...interface{})                {}
func (NopLogger) Info(msg string, args ...interface{})                 {}
func (NopLogger) Warn(msg string, args ...interface{})                 {}
func (NopLogger) Error(msg string, args ...interface{})                {}
func (n NopLogger) WithField(key string, value interface{}) Logger     { return n }
func (n NopLogger) WithFields(fields map[string]interface{}) Logger    { return n }
func (NopLogger) SetLevel(level Level)                                 {}
func (NopLogger) SetOutput(w io.Writer)                                {}
