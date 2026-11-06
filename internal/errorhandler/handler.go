// Package errorhandler writes adapter error responses. An error that carries a
// 4xx or 5xx HTTP status (apix.StatusCoder) is sent with that status; any other
// error is logged and replaced by InternalError, so its message never reaches
// the client.
package errorhandler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	apix "github.com/Infra-Forge/infra-apix"
	"github.com/Infra-Forge/infra-apix/internal/logging"
)

// The request headers that the error log line carries as request_id and
// correlation_id.
const (
	RequestIDHeader     = "X-Request-ID"
	CorrelationIDHeader = "X-Correlation-ID"
)

// Request is what the error log line says about the request.
type Request struct {
	Method        string
	Path          string
	RequestID     string
	CorrelationID string
}

// RequestOf describes r for the error log line.
func RequestOf(r *http.Request) Request {
	return Request{
		Method:        r.Method,
		Path:          r.URL.Path,
		RequestID:     r.Header.Get(RequestIDHeader),
		CorrelationID: r.Header.Get(CorrelationIDHeader),
	}
}

// Attrs returns the log attributes for req: method, path, and request_id and
// correlation_id when set.
func (req Request) Attrs() []slog.Attr {
	attrs := []slog.Attr{slog.String("method", req.Method), slog.String("path", req.Path)}
	if req.RequestID != "" {
		attrs = append(attrs, slog.String("request_id", req.RequestID))
	}
	if req.CorrelationID != "" {
		attrs = append(attrs, slog.String("correlation_id", req.CorrelationID))
	}
	return attrs
}

// HandleError writes err as the response for the net/http-based adapters (chi
// and mux). With useProblemDetails, the body is RFC 9457 application/problem+json.
func HandleError(w http.ResponseWriter, r *http.Request, err error, useProblemDetails bool) {
	status, err := Resolve(r.Context(), RequestOf(r), err)

	if useProblemDetails {
		// Marshal before writing headers so a failure can still fall back to text.
		if data, marshalErr := json.Marshal(apix.ToProblemDetails(err)); marshalErr == nil {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(status)
			if _, writeErr := w.Write(data); writeErr != nil {
				LogWriteFailed(r, "error response write failed", writeErr)
			}
			return
		}
	}

	http.Error(w, err.Error(), status)
}

// Resolve returns the status and the error to send for err. err is sent as is
// only when it carries a 4xx or 5xx status. Otherwise Resolve logs it for req
// and returns InternalError: net/http panics on a status below 100 or above
// 999, and any other status would present the error as an informational,
// success or redirect response.
func Resolve(ctx context.Context, req Request, err error) (int, error) {
	var statusCoder apix.StatusCoder
	if !errors.As(err, &statusCoder) {
		LogReplaced(ctx, req, err, slog.String("reason", "no HTTP status"))
		return http.StatusInternalServerError, InternalError()
	}
	status := statusCoder.HTTPStatus()
	if status < 400 || status > 599 {
		LogReplaced(ctx, req, err, slog.String("reason", "HTTP status outside 4xx and 5xx"), slog.Int("status", status))
		return http.StatusInternalServerError, InternalError()
	}
	return status, err
}

// InternalError is sent in place of an error that carries no HTTP error
// status: a 500 whose only text is the status text.
func InternalError() *apix.ProblemDetails {
	return &apix.ProblemDetails{
		Status: http.StatusInternalServerError,
		Title:  http.StatusText(http.StatusInternalServerError),
	}
}

// LogReplaced logs, at Error level and with the request's context, an error
// that the client receives as a generic 500: InternalError here, or the
// runtime handler's plain 500 for a document it cannot build. attrs come first
// and start with the reason. It goes to the logger passed to apix.SetLogger,
// or else to slog.Default(). slog quotes attribute values, so a line break
// decoded from the path (%0A) cannot forge a log line.
func LogReplaced(ctx context.Context, req Request, err error, attrs ...slog.Attr) {
	attrs = append(attrs, req.Attrs()...)
	attrs = append(attrs, slog.Any("error", err))
	logging.ErrorLogger().LogAttrs(ctx, slog.LevelError, "error answered with a generic 500", attrs...)
}

// LogWriteFailed logs a response the client did not receive in full, which
// usually means it went away. Like apix's other library events, it goes only
// to a logger passed to apix.SetLogger, at Warn level.
func LogWriteFailed(r *http.Request, msg string, err error) {
	attrs := append(RequestOf(r).Attrs(), slog.Any("error", err))
	logging.GetLogger().LogAttrs(r.Context(), slog.LevelWarn, msg, attrs...)
}
