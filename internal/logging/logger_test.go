package logging

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestNewJSONLogger(t *testing.T) {
	logger := NewJSONLogger(slog.LevelInfo)
	if logger == nil {
		t.Fatal("expected logger to be created")
	}
}

func TestNewTextLogger(t *testing.T) {
	logger := NewTextLogger(slog.LevelInfo)
	if logger == nil {
		t.Fatal("expected logger to be created")
	}
}

func TestNewNopLogger(t *testing.T) {
	logger := NewNopLogger()
	if logger == nil {
		t.Fatal("expected logger to be created")
	}
}

func TestSetAndGetLogger(t *testing.T) {
	originalLogger := GetLogger()
	defer SetLogger(originalLogger)

	newLogger := NewTextLogger(slog.LevelDebug)
	SetLogger(newLogger)

	retrieved := GetLogger()
	if retrieved != newLogger {
		t.Error("expected retrieved logger to match set logger")
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := NewLogger(handler)

	loggerWithFields := logger.WithFields("key", "value")
	loggerWithFields.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "key=value") {
		t.Errorf("expected output to contain 'key=value', got: %s", output)
	}
}

func TestRouteRegistered(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := NewLogger(handler)

	logger.RouteRegistered("GET", "/api/users")

	output := buf.String()
	if !strings.Contains(output, "route registered") {
		t.Errorf("expected output to contain 'route registered', got: %s", output)
	}
	if !strings.Contains(output, "method=GET") {
		t.Errorf("expected output to contain 'method=GET', got: %s", output)
	}
	if !strings.Contains(output, "path=/api/users") {
		t.Errorf("expected output to contain 'path=/api/users', got: %s", output)
	}
}

func TestSchemaGenerated(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := NewLogger(handler)

	logger.SchemaGenerated("User")

	output := buf.String()
	if !strings.Contains(output, "schema generated") {
		t.Errorf("expected output to contain 'schema generated', got: %s", output)
	}
	if !strings.Contains(output, "type=User") {
		t.Errorf("expected output to contain 'type=User', got: %s", output)
	}
}

func TestSpecBuilt(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	logger := NewLogger(handler)

	logger.SpecBuilt(10, 5)

	output := buf.String()
	if !strings.Contains(output, "OpenAPI spec built") {
		t.Errorf("expected output to contain 'OpenAPI spec built', got: %s", output)
	}
	if !strings.Contains(output, "routes=10") {
		t.Errorf("expected output to contain 'routes=10', got: %s", output)
	}
	if !strings.Contains(output, "schemas=5") {
		t.Errorf("expected output to contain 'schemas=5', got: %s", output)
	}
}

func TestHandlerExecuted(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	logger := NewLogger(handler)

	logger.HandlerExecuted("POST", "/api/users", 201)

	output := buf.String()
	if !strings.Contains(output, "handler executed") {
		t.Errorf("expected output to contain 'handler executed', got: %s", output)
	}
	if !strings.Contains(output, "status=201") {
		t.Errorf("expected output to contain 'status=201', got: %s", output)
	}
}

func TestErrorOccurred(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	})
	logger := NewLogger(handler)

	testErr := errors.New("test error")
	logger.ErrorOccurred("GET", "/api/users", testErr)

	output := buf.String()
	if !strings.Contains(output, "error occurred") {
		t.Errorf("expected output to contain 'error occurred', got: %s", output)
	}
	if !strings.Contains(output, "test error") {
		t.Errorf("expected output to contain 'test error', got: %s", output)
	}
}

// TestConfigure: the logger passed to apix.SetLogger receives library events
// and adapter errors. nil drops library events again and sends adapter errors
// to slog.Default().
func TestConfigure(t *testing.T) {
	t.Cleanup(func() { Configure(nil) })

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	Configure(logger)
	if ErrorLogger() != logger {
		t.Error("ErrorLogger must return the configured logger")
	}
	GetLogger().SpecBuilt(3, 2)
	if !strings.Contains(buf.String(), `msg="OpenAPI spec built" routes=3 schemas=2`) {
		t.Errorf("library event missing, got %q", buf.String())
	}

	Configure(nil)
	if ErrorLogger() != slog.Default() {
		t.Error("without a configured logger, ErrorLogger must return slog.Default()")
	}
	if GetLogger().Enabled(context.Background(), slog.LevelError) {
		t.Error("without a configured logger, library events must be dropped")
	}
}
