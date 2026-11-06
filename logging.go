package apix

import (
	"log/slog"

	"github.com/Infra-Forge/infra-apix/internal/logging"
)

// SetLogger sends apix's logs to logger.
//
// The adapters log at Error level every error they answer with a generic 500:
// one without an HTTP status, or with a status outside 4xx and 5xx. The line
// carries the method, the path, the X-Request-ID and X-Correlation-ID request
// headers as request_id and correlation_id when set, and the error. It is
// logged with the request's context, so a context-aware handler can add trace
// ids. The Echo adapter is the exception: it leaves an error without an HTTP
// status to Echo's HTTPErrorHandler, so it logs only an error with a status
// outside 4xx and 5xx. Echo's default HTTPErrorHandler does not log the errors
// it answers. echo.ProblemDetailsErrorHandler without a fallback answers any
// error itself and logs it like the others. runtime.Handler logs the same line
// for a document it cannot build, validate or encode, except under
// RegisterEcho, which leaves the error to Echo's HTTPErrorHandler too.
// Route registration, schema generation and document builds are logged at
// Info and Debug level; set the logger before registering routes to see their
// events.
//
// Without SetLogger, or after SetLogger(nil), apix logs only those errors, to
// slog.Default().
func SetLogger(logger *slog.Logger) {
	logging.Configure(logger)
}
