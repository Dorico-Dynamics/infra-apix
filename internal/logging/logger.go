package logging

import (
	"log/slog"
	"os"
	"sync"
)

// Logger wraps slog.Logger with apix-specific context
type Logger struct {
	*slog.Logger
}

var (
	globalLogger *Logger
	// configured is the logger passed to apix.SetLogger, nil until then.
	configured *slog.Logger
	mu         sync.RWMutex
)

func init() {
	// Initialize with default logger (disabled by default)
	SetLogger(NewNopLogger())
}

// NewLogger creates a new Logger with the given handler
func NewLogger(handler slog.Handler) *Logger {
	return &Logger{
		Logger: slog.New(handler),
	}
}

// NewJSONLogger creates a new JSON logger with the given level
func NewJSONLogger(level slog.Level) *Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return NewLogger(handler)
}

// NewTextLogger creates a new text logger with the given level
func NewTextLogger(level slog.Level) *Logger {
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	return NewLogger(handler)
}

// NewNopLogger creates a logger that discards all output
func NewNopLogger() *Logger {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError + 1, // Higher than any level, effectively disabling
	})
	return NewLogger(handler)
}

// SetLogger sets the global logger
func SetLogger(logger *Logger) {
	mu.Lock()
	defer mu.Unlock()
	globalLogger = logger
}

// GetLogger returns the global logger
func GetLogger() *Logger {
	mu.RLock()
	defer mu.RUnlock()
	return globalLogger
}

// Configure installs the logger passed to apix.SetLogger. Library events go to
// it instead of being dropped, and so do the errors the adapters answer with a
// generic 500. nil restores both defaults.
func Configure(logger *slog.Logger) {
	mu.Lock()
	defer mu.Unlock()
	configured = logger
	if logger == nil {
		globalLogger = NewNopLogger()
		return
	}
	globalLogger = &Logger{Logger: logger}
}

// ErrorLogger returns the logger for the errors the adapters answer with a
// generic 500: the one passed to apix.SetLogger, or else slog.Default(), so
// those errors are printed even when no logger was set.
func ErrorLogger() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if configured != nil {
		return configured
	}
	return slog.Default()
}

// WithFields returns a logger with the given fields
func (l *Logger) WithFields(fields ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(fields...),
	}
}

// RouteRegistered logs route registration events
func (l *Logger) RouteRegistered(method, path string, fields ...any) {
	l.Info("route registered",
		append([]any{"method", method, "path", path}, fields...)...)
}

// SchemaGenerated logs schema generation events
func (l *Logger) SchemaGenerated(typeName string, fields ...any) {
	l.Debug("schema generated",
		append([]any{"type", typeName}, fields...)...)
}

// SpecBuilt logs OpenAPI spec build events
func (l *Logger) SpecBuilt(routeCount, schemaCount int, fields ...any) {
	l.Info("OpenAPI spec built",
		append([]any{"routes", routeCount, "schemas", schemaCount}, fields...)...)
}

// HandlerExecuted logs handler execution events
func (l *Logger) HandlerExecuted(method, path string, statusCode int, fields ...any) {
	l.Debug("handler executed",
		append([]any{"method", method, "path", path, "status", statusCode}, fields...)...)
}

// ErrorOccurred logs error events
func (l *Logger) ErrorOccurred(method, path string, err error, fields ...any) {
	l.Error("error occurred",
		append([]any{"method", method, "path", path, "error", err}, fields...)...)
}

// PluginRegistered logs plugin registration events
func (l *Logger) PluginRegistered(pluginName string, fields ...any) {
	l.Info("plugin registered",
		append([]any{"plugin", pluginName}, fields...)...)
}

// PluginExecuted logs plugin execution events
func (l *Logger) PluginExecuted(pluginName, hook string, fields ...any) {
	l.Debug("plugin executed",
		append([]any{"plugin", pluginName, "hook", hook}, fields...)...)
}

// Global convenience functions

// Info logs an info message using the global logger
func Info(msg string, fields ...any) {
	GetLogger().Info(msg, fields...)
}

// Debug logs a debug message using the global logger
func Debug(msg string, fields ...any) {
	GetLogger().Debug(msg, fields...)
}

// Warn logs a warning message using the global logger
func Warn(msg string, fields ...any) {
	GetLogger().Warn(msg, fields...)
}

// Error logs an error message using the global logger
func Error(msg string, fields ...any) {
	GetLogger().Error(msg, fields...)
}
