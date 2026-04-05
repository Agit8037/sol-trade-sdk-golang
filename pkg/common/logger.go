package common

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// LogLevel represents the logging level
type LogLevel int32

const (
	// LogLevelDebug is the most verbose level
	LogLevelDebug LogLevel = iota
	// LogLevelInfo is for informational messages
	LogLevelInfo
	// LogLevelWarn is for warning messages
	LogLevelWarn
	// LogLevelError is for error messages
	LogLevelError
	// LogLevelFatal is for fatal errors
	LogLevelFatal
	// LogLevelNone disables all logging
	LogLevelNone
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case LogLevelDebug:
		return "DEBUG"
	case LogLevelInfo:
		return "INFO"
	case LogLevelWarn:
		return "WARN"
	case LogLevelError:
		return "ERROR"
	case LogLevelFatal:
		return "FATAL"
	case LogLevelNone:
		return "NONE"
	default:
		return "UNKNOWN"
	}
}

// ParseLogLevel parses a log level from string
func ParseLogLevel(s string) LogLevel {
	switch s {
	case "DEBUG":
		return LogLevelDebug
	case "INFO":
		return LogLevelInfo
	case "WARN", "WARNING":
		return LogLevelWarn
	case "ERROR":
		return LogLevelError
	case "FATAL":
		return LogLevelFatal
	case "NONE":
		return LogLevelNone
	default:
		return LogLevelInfo
	}
}

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     LogLevel               `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string                 `json:"caller,omitempty"`
	TraceID   string                 `json:"trace_id,omitempty"`
}

// MarshalJSON implements custom JSON marshaling
func (e LogEntry) MarshalJSON() ([]byte, error) {
	type Alias LogEntry
	return json.Marshal(&struct {
		Timestamp string `json:"timestamp"`
		Level     string `json:"level"`
		*Alias
	}{
		Timestamp: e.Timestamp.Format(time.RFC3339Nano),
		Level:     e.Level.String(),
		Alias:     (*Alias)(&e),
	})
}

// LogFormatter formats log entries
type LogFormatter interface {
	Format(entry *LogEntry) ([]byte, error)
}

// JSONFormatter formats logs as JSON
type JSONFormatter struct {
	PrettyPrint bool
}

// Format formats a log entry as JSON
func (f *JSONFormatter) Format(entry *LogEntry) ([]byte, error) {
	if f.PrettyPrint {
		return json.MarshalIndent(entry, "", "  ")
	}
	return json.Marshal(entry)
}

// TextFormatter formats logs as text
type TextFormatter struct {
	DisableColors bool
	FullTimestamp bool
}

// Format formats a log entry as text
func (f *TextFormatter) Format(entry *LogEntry) ([]byte, error) {
	timestamp := ""
	if f.FullTimestamp {
		timestamp = entry.Timestamp.Format(time.RFC3339) + " "
	} else {
		timestamp = entry.Timestamp.Format("15:04:05") + " "
	}

	msg := fmt.Sprintf("%s[%s] %s", timestamp, entry.Level.String(), entry.Message)

	if len(entry.Fields) > 0 {
		fields, _ := json.Marshal(entry.Fields)
		msg += " " + string(fields)
	}

	if entry.Caller != "" {
		msg += fmt.Sprintf(" (%s)", entry.Caller)
	}

	if entry.TraceID != "" {
		msg += fmt.Sprintf(" [trace=%s]", entry.TraceID)
	}

	return []byte(msg + "\n"), nil
}

// SDKLogger is a structured logger for the SDK
type SDKLogger struct {
	level     atomic.Int32
	formatter LogFormatter
	output    io.Writer
	mu        sync.RWMutex
	fields    map[string]interface{}
	caller    bool
	traceID   string
}

// LoggerOption is a functional option for the logger
type LoggerOption func(*SDKLogger)

// WithLevel sets the log level
func WithLevel(level LogLevel) LoggerOption {
	return func(l *SDKLogger) {
		l.level.Store(int32(level))
	}
}

// WithFormatter sets the log formatter
func WithFormatter(formatter LogFormatter) LoggerOption {
	return func(l *SDKLogger) {
		l.formatter = formatter
	}
}

// WithOutput sets the output writer
func WithOutput(output io.Writer) LoggerOption {
	return func(l *SDKLogger) {
		l.output = output
	}
}

// WithFields sets default fields
func WithFields(fields map[string]interface{}) LoggerOption {
	return func(l *SDKLogger) {
		l.fields = fields
	}
}

// WithCaller enables caller information
func WithCaller(enabled bool) LoggerOption {
	return func(l *SDKLogger) {
		l.caller = enabled
	}
}

// WithTraceID sets the trace ID
func WithTraceID(traceID string) LoggerOption {
	return func(l *SDKLogger) {
		l.traceID = traceID
	}
}

// NewSDKLogger creates a new SDK logger
func NewSDKLogger(opts ...LoggerOption) *SDKLogger {
	logger := &SDKLogger{
		formatter: &TextFormatter{FullTimestamp: true},
		output:    os.Stdout,
		fields:    make(map[string]interface{}),
	}
	logger.level.Store(int32(LogLevelInfo))

	for _, opt := range opts {
		opt(logger)
	}

	return logger
}

// SetLevel sets the log level
func (l *SDKLogger) SetLevel(level LogLevel) {
	l.level.Store(int32(level))
}

// GetLevel returns the current log level
func (l *SDKLogger) GetLevel() LogLevel {
	return LogLevel(l.level.Load())
}

// IsLevelEnabled checks if a level is enabled
func (l *SDKLogger) IsLevelEnabled(level LogLevel) bool {
	return level >= l.GetLevel()
}

// WithField adds a field to the logger
func (l *SDKLogger) WithField(key string, value interface{}) *SDKLogger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{}, len(l.fields)+1)
	for k, v := range l.fields {
		newFields[k] = v
	}
	newFields[key] = value

	return &SDKLogger{
		level:     atomic.Int32{},
		formatter: l.formatter,
		output:    l.output,
		fields:    newFields,
		caller:    l.caller,
		traceID:   l.traceID,
	}
}

// WithFields adds multiple fields to the logger
func (l *SDKLogger) WithFields(fields map[string]interface{}) *SDKLogger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newFields := make(map[string]interface{}, len(l.fields)+len(fields))
	for k, v := range l.fields {
		newFields[k] = v
	}
	for k, v := range fields {
		newFields[k] = v
	}

	return &SDKLogger{
		level:     atomic.Int32{},
		formatter: l.formatter,
		output:    l.output,
		fields:    newFields,
		caller:    l.caller,
		traceID:   l.traceID,
	}
}

// WithTraceID adds a trace ID to the logger
func (l *SDKLogger) WithTraceID(traceID string) *SDKLogger {
	return &SDKLogger{
		level:     atomic.Int32{},
		formatter: l.formatter,
		output:    l.output,
		fields:    l.fields,
		caller:    l.caller,
		traceID:   traceID,
	}
}

// log performs the actual logging
func (l *SDKLogger) log(level LogLevel, msg string, fields map[string]interface{}) {
	if !l.IsLevelEnabled(level) {
		return
	}

	entry := &LogEntry{
		Timestamp: time.Now().UTC(),
		Level:     level,
		Message:   msg,
		Fields:    make(map[string]interface{}),
		TraceID:   l.traceID,
	}

	// Add default fields
	l.mu.RLock()
	for k, v := range l.fields {
		entry.Fields[k] = v
	}
	l.mu.RUnlock()

	// Add call-specific fields
	for k, v := range fields {
		entry.Fields[k] = v
	}

	// Add caller info if enabled
	if l.caller {
		entry.Caller = getCaller(3)
	}

	// Format and output
	data, err := l.formatter.Format(entry)
	if err != nil {
		return
	}

	l.mu.Lock()
	l.output.Write(data)
	l.mu.Unlock()
}

// Debug logs a debug message
func (l *SDKLogger) Debug(msg string) {
	l.log(LogLevelDebug, msg, nil)
}

// Debugf logs a formatted debug message
func (l *SDKLogger) Debugf(format string, args ...interface{}) {
	l.log(LogLevelDebug, fmt.Sprintf(format, args...), nil)
}

// Debugw logs a debug message with fields
func (l *SDKLogger) Debugw(msg string, fields map[string]interface{}) {
	l.log(LogLevelDebug, msg, fields)
}

// Info logs an info message
func (l *SDKLogger) Info(msg string) {
	l.log(LogLevelInfo, msg, nil)
}

// Infof logs a formatted info message
func (l *SDKLogger) Infof(format string, args ...interface{}) {
	l.log(LogLevelInfo, fmt.Sprintf(format, args...), nil)
}

// Infow logs an info message with fields
func (l *SDKLogger) Infow(msg string, fields map[string]interface{}) {
	l.log(LogLevelInfo, msg, fields)
}

// Warn logs a warning message
func (l *SDKLogger) Warn(msg string) {
	l.log(LogLevelWarn, msg, nil)
}

// Warnf logs a formatted warning message
func (l *SDKLogger) Warnf(format string, args ...interface{}) {
	l.log(LogLevelWarn, fmt.Sprintf(format, args...), nil)
}

// Warnw logs a warning message with fields
func (l *SDKLogger) Warnw(msg string, fields map[string]interface{}) {
	l.log(LogLevelWarn, msg, fields)
}

// Error logs an error message
func (l *SDKLogger) Error(msg string) {
	l.log(LogLevelError, msg, nil)
}

// Errorf logs a formatted error message
func (l *SDKLogger) Errorf(format string, args ...interface{}) {
	l.log(LogLevelError, fmt.Sprintf(format, args...), nil)
}

// Errorw logs an error message with fields
func (l *SDKLogger) Errorw(msg string, fields map[string]interface{}) {
	l.log(LogLevelError, msg, fields)
}

// Fatal logs a fatal message
func (l *SDKLogger) Fatal(msg string) {
	l.log(LogLevelFatal, msg, nil)
}

// Fatalf logs a formatted fatal message
func (l *SDKLogger) Fatalf(format string, args ...interface{}) {
	l.log(LogLevelFatal, fmt.Sprintf(format, args...), nil)
}

// Fatalw logs a fatal message with fields
func (l *SDKLogger) Fatalw(msg string, fields map[string]interface{}) {
	l.log(LogLevelFatal, msg, fields)
}

// getCaller returns the caller information (simplified)
func getCaller(skip int) string {
	// In a real implementation, use runtime.Caller
	return ""
}

// ===== Global Logger =====

var (
	globalLogger     *SDKLogger
	globalLoggerOnce sync.Once
)

// GetLogger returns the global logger instance
func GetLogger() *SDKLogger {
	globalLoggerOnce.Do(func() {
		globalLogger = NewSDKLogger()
	})
	return globalLogger
}

// SetGlobalLogger sets the global logger
func SetGlobalLogger(logger *SDKLogger) {
	globalLogger = logger
}

// InitGlobalLogger initializes the global logger with options
func InitGlobalLogger(opts ...LoggerOption) {
	globalLogger = NewSDKLogger(opts...)
}

// Debug uses the global logger
func Debug(msg string) {
	GetLogger().Debug(msg)
}

// Debugf uses the global logger
func Debugf(format string, args ...interface{}) {
	GetLogger().Debugf(format, args...)
}

// Info uses the global logger
func Info(msg string) {
	GetLogger().Info(msg)
}

// Infof uses the global logger
func Infof(format string, args ...interface{}) {
	GetLogger().Infof(format, args...)
}

// Warn uses the global logger
func Warn(msg string) {
	GetLogger().Warn(msg)
}

// Warnf uses the global logger
func Warnf(format string, args ...interface{}) {
	GetLogger().Warnf(format, args...)
}

// Error uses the global logger
func Error(msg string) {
	GetLogger().Error(msg)
}

// Errorf uses the global logger
func Errorf(format string, args ...interface{}) {
	GetLogger().Errorf(format, args...)
}

// Fatal uses the global logger
func Fatal(msg string) {
	GetLogger().Fatal(msg)
}

// Fatalf uses the global logger
func Fatalf(format string, args ...interface{}) {
	GetLogger().Fatalf(format, args...)
}

// ===== Log Adapter =====

// LogAdapter adapts SDKLogger to standard logger interface
type LogAdapter struct {
	logger *SDKLogger
}

// NewLogAdapter creates a new log adapter
func NewLogAdapter(logger *SDKLogger) *LogAdapter {
	return &LogAdapter{logger: logger}
}

// Print implements the standard logger interface
func (a *LogAdapter) Print(v ...interface{}) {
	a.logger.Info(fmt.Sprint(v...))
}

// Printf implements the standard logger interface
func (a *LogAdapter) Printf(format string, v ...interface{}) {
	a.logger.Infof(format, v...)
}

// Println implements the standard logger interface
func (a *LogAdapter) Println(v ...interface{}) {
	a.logger.Info(fmt.Sprintln(v...))
}
