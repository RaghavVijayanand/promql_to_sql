package logging

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Logger interface following Interface Segregation Principle
type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	Fatal(msg string, fields ...Field)
	
	WithFields(fields ...Field) Logger
	SetLevel(level LogLevel)
}

// Field represents a structured log field
type Field struct {
	Key   string
	Value interface{}
}

// F creates a new field (convenience function)
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// StructuredLogger implements Logger interface with structured logging
type StructuredLogger struct {
	level  LogLevel
	output io.Writer
	fields []Field
	mu     sync.Mutex
}

// NewLogger creates a new structured logger
func NewLogger(output io.Writer, level LogLevel) Logger {
	return &StructuredLogger{
		level:  level,
		output: output,
		fields: make([]Field, 0),
	}
}

// NewDefaultLogger creates a logger with stdout and INFO level
func NewDefaultLogger() Logger {
	return NewLogger(os.Stdout, INFO)
}

// Debug logs a debug message
func (l *StructuredLogger) Debug(msg string, fields ...Field) {
	l.log(DEBUG, msg, fields...)
}

// Info logs an info message
func (l *StructuredLogger) Info(msg string, fields ...Field) {
	l.log(INFO, msg, fields...)
}

// Warn logs a warning message
func (l *StructuredLogger) Warn(msg string, fields ...Field) {
	l.log(WARN, msg, fields...)
}

// Error logs an error message
func (l *StructuredLogger) Error(msg string, fields ...Field) {
	l.log(ERROR, msg, fields...)
}

// Fatal logs a fatal message and exits
func (l *StructuredLogger) Fatal(msg string, fields ...Field) {
	l.log(FATAL, msg, fields...)
	os.Exit(1)
}

// WithFields returns a new logger with additional fields
func (l *StructuredLogger) WithFields(fields ...Field) Logger {
	newFields := make([]Field, len(l.fields)+len(fields))
	copy(newFields, l.fields)
	copy(newFields[len(l.fields):], fields)
	
	return &StructuredLogger{
		level:  l.level,
		output: l.output,
		fields: newFields,
	}
}

// SetLevel sets the log level
func (l *StructuredLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// log is the internal logging method
func (l *StructuredLogger) log(level LogLevel, msg string, fields ...Field) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	// Check level under lock to prevent race with SetLevel
	if level < l.level {
		return
	}
	
	// Build log entry
	entry := l.buildEntry(level, msg, fields)
	
	// Write to output
	fmt.Fprintln(l.output, entry)
}

// buildEntry builds a formatted log entry
func (l *StructuredLogger) buildEntry(level LogLevel, msg string, fields []Field) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")
	
	// Start with timestamp, level, and message
	entry := fmt.Sprintf("[%s] %s: %s", timestamp, level, msg)
	
	// Merge base and call-site fields without mutating the shared l.fields slice.
	// Using append(l.fields, fields...) could mutate the underlying array if
	// l.fields has spare capacity, causing a data race with other loggers
	// derived via WithFields.
	totalFields := len(l.fields) + len(fields)
	if totalFields > 0 {
		entry += " |"
		for _, field := range l.fields {
			entry += fmt.Sprintf(" %s=%v", field.Key, field.Value)
		}
		for _, field := range fields {
			entry += fmt.Sprintf(" %s=%v", field.Key, field.Value)
		}
	}
	
	return entry
}

// TranspilerLogger provides domain-specific logging for transpiler
type TranspilerLogger struct {
	logger Logger
}

// NewTranspilerLogger creates a new transpiler logger
func NewTranspilerLogger(logger Logger) *TranspilerLogger {
	return &TranspilerLogger{
		logger: logger.WithFields(F("component", "transpiler")),
	}
}

// LogQueryStart logs the start of query transpilation
func (l *TranspilerLogger) LogQueryStart(query string) {
	l.logger.Info("Starting query transpilation",
		F("query", query),
		F("query_length", len(query)))
}

// LogQueryComplete logs successful query transpilation
func (l *TranspilerLogger) LogQueryComplete(query string, sql string, duration time.Duration) {
	l.logger.Info("Query transpilation complete",
		F("query", query),
		F("sql_length", len(sql)),
		F("duration_ms", duration.Milliseconds()))
}

// LogQueryError logs query transpilation error
func (l *TranspilerLogger) LogQueryError(query string, err error, duration time.Duration) {
	l.logger.Error("Query transpilation failed",
		F("query", query),
		F("error", err.Error()),
		F("duration_ms", duration.Milliseconds()))
}

// LogStrategySelection logs strategy selection
func (l *TranspilerLogger) LogStrategySelection(strategyName string, priority int) {
	l.logger.Debug("Strategy selected",
		F("strategy", strategyName),
		F("priority", priority))
}

// LogFunctionHandler logs function handler execution
func (l *TranspilerLogger) LogFunctionHandler(funcName string, args int) {
	l.logger.Debug("Executing function handler",
		F("function", funcName),
		F("args", args))
}

// LogOptimization logs query optimization
func (l *TranspilerLogger) LogOptimization(optimization string, before int, after int) {
	var reduction float64
	if before > 0 {
		reduction = float64(before-after) / float64(before) * 100
	}
	l.logger.Info("Applied optimization",
		F("optimization", optimization),
		F("before_size", before),
		F("after_size", after),
		F("reduction_percent", fmt.Sprintf("%.2f", reduction)))
}

// LogCardinality logs cardinality estimation
func (l *TranspilerLogger) LogCardinality(metric string, estimated int) {
	l.logger.Info("Cardinality estimated",
		F("metric", metric),
		F("estimated_cardinality", estimated))
}

// PerformanceLogger logs performance metrics
type PerformanceLogger struct {
	logger Logger
}

// NewPerformanceLogger creates a new performance logger
func NewPerformanceLogger(logger Logger) *PerformanceLogger {
	return &PerformanceLogger{
		logger: logger.WithFields(F("component", "performance")),
	}
}

// LogQueryExecution logs query execution time
func (l *PerformanceLogger) LogQueryExecution(sql string, duration time.Duration, rowCount int) {
	l.logger.Info("Query executed",
		F("sql_length", len(sql)),
		F("duration_ms", duration.Milliseconds()),
		F("rows", rowCount))
}

// LogCacheHit logs cache hit
func (l *PerformanceLogger) LogCacheHit(key string) {
	l.logger.Debug("Cache hit", F("key", key))
}

// LogCacheMiss logs cache miss
func (l *PerformanceLogger) LogCacheMiss(key string) {
	l.logger.Debug("Cache miss", F("key", key))
}

// NullLogger implements Logger but does nothing
type NullLogger struct{}

func (l *NullLogger) Debug(msg string, fields ...Field)         {}
func (l *NullLogger) Info(msg string, fields ...Field)          {}
func (l *NullLogger) Warn(msg string, fields ...Field)          {}
func (l *NullLogger) Error(msg string, fields ...Field)         {}
func (l *NullLogger) Fatal(msg string, fields ...Field)         {}
func (l *NullLogger) WithFields(fields ...Field) Logger         { return l }
func (l *NullLogger) SetLevel(level LogLevel)                   {}

// Global logger instance (protected by globalMu)
var (
	globalLogger Logger = NewDefaultLogger()
	globalMu     sync.RWMutex
)

// SetGlobalLogger sets the global logger (thread-safe)
func SetGlobalLogger(logger Logger) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalLogger = logger
}

// GetGlobalLogger returns the global logger (thread-safe)
func GetGlobalLogger() Logger {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalLogger
}

// Convenience functions using global logger
func Debug(msg string, fields ...Field) {
	GetGlobalLogger().Debug(msg, fields...)
}

func Info(msg string, fields ...Field) {
	GetGlobalLogger().Info(msg, fields...)
}

func Warn(msg string, fields ...Field) {
	GetGlobalLogger().Warn(msg, fields...)
}

func Error(msg string, fields ...Field) {
	GetGlobalLogger().Error(msg, fields...)
}

func Fatal(msg string, fields ...Field) {
	GetGlobalLogger().Fatal(msg, fields...)
}
