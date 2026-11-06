// Package errorhandler writes adapter error responses. An error that carries a
// 4xx or 5xx HTTP status (apix.StatusCoder) is sent with that status; any other
// error is logged and replaced by InternalError, so its message never reaches
// the client.
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
	status, err := Resolve(r.Method, r.URL.Path, err)

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

// Resolve returns the status and the error to send for err. err is sent as is
// only when it carries a 4xx or 5xx status. Otherwise Resolve logs it and
// returns InternalError: net/http panics on a status below 100 or above 999,
// and any other status would present the error as an informational, success
// or redirect response.
func Resolve(method, path string, err error) (int, error) {
	var statusCoder apix.StatusCoder
	if !errors.As(err, &statusCoder) {
		LogUnhandled(method, path, err)
		return http.StatusInternalServerError, InternalError()
	}
	status := statusCoder.HTTPStatus()
	if status < 400 || status > 599 {
		log.Printf("error with HTTP status %d, not 4xx or 5xx, in %q %q: %v", status, method, path, err)
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

// LogUnhandled records an error that reached an adapter without an HTTP status.
// The client receives InternalError instead. Method and path come from the
// request and are quoted, so an encoded line break (%0A) cannot forge a log
// line.
func LogUnhandled(method, path string, err error) {
	log.Printf("unrecognized error in %q %q: %v", method, path, err)
}
