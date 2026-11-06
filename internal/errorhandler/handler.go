// Package errorhandler writes adapter error responses. An error that carries an
// HTTP status (apix.StatusCoder) is sent with that status; any other error is
// logged and replaced by InternalError, so its message never reaches the client.
package errorhandler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	apix "github.com/Infra-Forge/infra-apix"
)

// HandleError writes err as the response for the net/http-based adapters (chi
// and mux). With useProblemDetails, the body is RFC 9457 application/problem+json.
func HandleError(w http.ResponseWriter, r *http.Request, err error, useProblemDetails bool) {
	var statusCoder apix.StatusCoder
	if !errors.As(err, &statusCoder) {
		LogUnhandled(r.Method, r.URL.Path, err)
		statusCoder = InternalError()
		err = statusCoder
	}
	status := statusCoder.HTTPStatus()

	if useProblemDetails {
		// Marshal before writing headers so a failure can still fall back to text.
		if data, marshalErr := json.Marshal(apix.ToProblemDetails(err)); marshalErr == nil {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(status)
			w.Write(data)
			return
		}
	}

	http.Error(w, err.Error(), status)
}

// InternalError is sent in place of an error that carries no HTTP status: a
// 500 whose only text is the status text.
func InternalError() *apix.ProblemDetails {
	return &apix.ProblemDetails{
		Status: http.StatusInternalServerError,
		Title:  http.StatusText(http.StatusInternalServerError),
	}
}

// LogUnhandled records an error that reached an adapter without an HTTP status.
// The client receives InternalError instead. Method and path come from the
// request and are quoted, so an encoded line break (%0A) cannot forge a log
// line.
func LogUnhandled(method, path string, err error) {
	log.Printf("unrecognized error in %q %q: %v", method, path, err)
}
