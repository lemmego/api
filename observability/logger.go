// Package observability provides structured logging capabilities for the Lemmego framework
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log entry
type LogLevel int

const (
	// LevelDebug is used for detailed debugging information
	LevelDebug LogLevel = iota - 4
	// LevelInfo is used for general informational messages
	LevelInfo
	// LevelWarn is used for warning messages that don't require immediate action
	LevelWarn
	// LevelError is used for error messages that indicate a failure
	LevelError
	// LevelFatal is used for fatal error messages that indicate the application
	// cannot continue running
	LevelFatal
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", l)
	}
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     LogLevel               `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Source    string                 `json:"source,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
	UserID    string                 `json:"user_id,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Stack     string                 `json:"stack,omitempty"`
}

// Logger is a structured logger that writes JSON formatted logs
type Logger struct {
	mu              sync.RWMutex
	writer          io.Writer
	minLevel        LogLevel
	defaultFields   map[string]interface{}
	callerSkip      int
	isDiscard       bool
	outputToFile    bool
	filePath        string
	file            *os.File
}

// LoggerConfig holds configuration for the logger
type LoggerConfig struct {
	// Output is the destination for log output (os.Writer or file path)
	Output interface{}
	// MinLevel is the minimum log level to write
	MinLevel LogLevel
	// DefaultFields are fields that will be included in every log entry
	DefaultFields map[string]interface{}
	// CallerSkip is the number of stack frames to skip when getting caller info
	CallerSkip int
	// EnableConsoleColors enables colored console output (only when writing to terminal)
	EnableConsoleColors bool
}

// NewLogger creates a new structured logger with the given configuration
func NewLogger(config LoggerConfig) (*Logger, error) {
	logger := &Logger{
		minLevel:      config.MinLevel,
		defaultFields: config.DefaultFields,
		callerSkip:    config.CallerSkip + 1,
		isDiscard:      true, // Start in discard mode
	}

	if config.DefaultFields == nil {
		logger.defaultFields = make(map[string]interface{})
	}

	// Determine output destination
	switch v := config.Output.(type) {
	case io.Writer:
		logger.writer = v
	case string:
		logger.outputToFile = true
		logger.filePath = v
		if err := logger.openLogFile(); err != nil {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
	default:
		if config.Output == nil {
			logger.writer = os.Stdout
		} else {
			return nil, fmt.Errorf("invalid output type: %T", config.Output)
		}
	}

	logger.isDiscard = false
	return logger, nil
}

// openLogFile opens the log file for writing
func (l *Logger) openLogFile() error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(l.filePath), 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	file, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.file = file
	l.writer = file
	return nil
}

// Close closes the logger and any open files
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return err
		}
		l.file = nil
	}

	l.isDiscard = true
	return nil
}

// Log writes a log entry at the specified level
func (l *Logger) Log(ctx context.Context, level LogLevel, message string, fields map[string]interface{}) {
	if l.isDiscard || level < l.minLevel {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   message,
		Fields:    make(map[string]interface{}),
	}

	// Add default fields
	for k, v := range l.defaultFields {
		entry.Fields[k] = v
	}

	// Add caller info
	if pc, file, line, ok := l.getCaller(); ok {
		entry.Source = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		entry.Fields["function"] = runtime.FuncForPC(pc).Name()
	}

	// Add request context from context
	if requestID := ctx.Value("request_id"); requestID != nil {
		entry.RequestID = fmt.Sprintf("%v", requestID)
	}
	if userID := ctx.Value("user_id"); userID != nil {
		entry.UserID = fmt.Sprintf("%v", userID)
	}

	// Add custom fields
	for k, v := range fields {
		entry.Fields[k] = v
	}

	// Extract error if present
	if err, ok := entry.Fields["error"]; ok {
		if errObj, ok := err.(error); ok {
			entry.Error = errObj.Error()
			entry.Fields["error"] = errObj.Error()
		} else if str, ok := err.(string); ok {
			entry.Error = str
		}
	}

	l.writeEntry(entry)
}

// writeEntry writes a log entry to the output
func (l *Logger) writeEntry(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		// Fallback to simple text output if JSON marshaling fails
		fmt.Fprintf(l.writer, "[%s] %s %s\n", entry.Level, entry.Timestamp.Format(time.RFC3339), entry.Message)
		return
	}

	fmt.Fprintf(l.writer, "%s\n", string(data))
}

// getCaller returns the program counter, file, and line number of the caller
func (l *Logger) getCaller() (pc uintptr, file string, line int, ok bool) {
	// Skip extra frames for this function
	callerSkip := l.callerSkip + 2
	pc, file, line, ok = runtime.Caller(callerSkip)
	return pc, file, line, ok
}

// WithField returns a copy of the logger with an additional default field
func (l *Logger) WithField(key string, value interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{})
	for k, v := range l.defaultFields {
		newFields[k] = v
	}
	newFields[key] = value

	return &Logger{
		writer:        l.writer,
		minLevel:      l.minLevel,
		defaultFields: newFields,
		callerSkip:    l.callerSkip,
		isDiscard:      l.isDiscard,
		outputToFile:   l.outputToFile,
		filePath:      l.filePath,
		file:          l.file,
	}
}

// WithFields returns a copy of the logger with additional default fields
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{})
	for k, v := range l.defaultFields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &Logger{
		writer:        l.writer,
		minLevel:      l.minLevel,
		defaultFields: newFields,
		callerSkip:    l.callerSkip,
		isDiscard:      l.isDiscard,
		outputToFile:   l.outputToFile,
		filePath:      l.filePath,
		file:          l.file,
	}
}

// WithRequestID returns a copy of the logger with a request ID field
func (l *Logger) WithRequestID(requestID string) *Logger {
	return l.WithField("request_id", requestID)
}

// WithUserID returns a copy of the logger with a user ID field
func (l *Logger) WithUserID(userID string) *Logger {
	return l.WithField("user_id", userID)
}

// Debug logs a debug message
func (l *Logger) Debug(ctx context.Context, message string, fields map[string]interface{}) {
	l.Log(ctx, LevelDebug, message, fields)
}

// Info logs an info message
func (l *Logger) Info(ctx context.Context, message string, fields map[string]interface{}) {
	l.Log(ctx, LevelInfo, message, fields)
}

// Warn logs a warning message
func (l *Logger) Warn(ctx context.Context, message string, fields map[string]interface{}) {
	l.Log(ctx, LevelWarn, message, fields)
}

// Error logs an error message
func (l *Logger) Error(ctx context.Context, message string, fields map[string]interface{}) {
	l.Log(ctx, LevelError, message, fields)
}

// Fatal logs a fatal message and exits the application
func (l *Logger) Fatal(ctx context.Context, message string, fields map[string]interface{}) {
	l.Log(ctx, LevelFatal, message, fields)
	os.Exit(1)
}

// WithError logs an error and returns the logger with error field
func (l *Logger) WithError(err error) *Logger {
	return l.WithField("error", err.Error())
}

// WithStringField returns a copy of the logger with a string field
func (l *Logger) WithStringField(key, value string) *Logger {
	return l.WithField(key, value)
}

// WithIntField returns a copy of the logger with an int field
func (l *Logger) WithIntField(key string, value int) *Logger {
	return l.WithField(key, value)
}

// SetLevel changes the minimum log level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.minLevel = level
}

// GetLevel returns the current minimum log level
func (l *Logger) GetLevel() LogLevel {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.minLevel
}

// IsEnabled returns true if logging is enabled for the given level
func (l *Logger) IsEnabled(level LogLevel) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return !l.isDiscard && level >= l.minLevel
}

// Rotate rotates the log file (for file-based loggers)
func (l *Logger) Rotate() error {
	if !l.outputToFile {
		return fmt.Errorf("logger is not file-based")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return err
		}
	}

	return l.openLogFile()
}

// Flush flushes any buffered log data
func (l *Logger) Flush() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if flusher, ok := l.writer.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}

	return nil
}

// DefaultLogger returns the default structured logger
var DefaultLogger *Logger

// InitDefaultLogger initializes the default logger with configuration from environment
func InitDefaultLogger() error {
	// Get log level from environment
	logLevel := LevelInfo
	if levelStr := os.Getenv("LOG_LEVEL"); levelStr != "" {
		switch levelStr {
		case "DEBUG":
			logLevel = LevelDebug
		case "INFO":
			logLevel = LevelInfo
		case "WARN", "WARNING":
			logLevel = LevelWarn
		case "ERROR":
			logLevel = LevelError
		case "FATAL":
			logLevel = LevelFatal
		}
	}

	// Get output destination from environment
	var output any = os.Stdout
	outputPath := os.Getenv("LOG_OUTPUT")
	if outputPath != "" {
		// Assume it's a file path
		output = outputPath
	}

	config := LoggerConfig{
		Output:     output,
		MinLevel:   logLevel,
		CallerSkip: 2,
	}

	logger, err := NewLogger(config)
	if err != nil {
		return err
	}

	DefaultLogger = logger
	return nil
}

// ParseLogLevel parses a log level string into a LogLevel
func ParseLogLevel(level string) (LogLevel, error) {
	switch level {
	case "DEBUG":
		return LevelDebug, nil
	case "INFO":
		return LevelInfo, nil
	case "WARN", "WARNING":
		return LevelWarn, nil
	case "ERROR":
		return LevelError, nil
	case "FATAL":
		return LevelFatal, nil
	default:
		return LevelInfo, fmt.Errorf("unknown log level: %s", level)
	}
}

// LogToStdout writes a log entry directly to stdout (bypasses the logger)
// Useful for early logging before the logger is initialized
func LogToStdout(level LogLevel, message string, fields map[string]interface{}) {
	entry := LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   message,
		Fields:    fields,
	}

	// Add caller info
	if pc, file, line, ok := runtime.Caller(2); ok {
		entry.Source = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		entry.Fields["function"] = runtime.FuncForPC(pc).Name()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(os.Stdout, "[%s] %s %s\n", level, entry.Timestamp.Format(time.RFC3339), message)
		return
	}

	fmt.Fprintf(os.Stdout, "%s\n", string(data))
}
