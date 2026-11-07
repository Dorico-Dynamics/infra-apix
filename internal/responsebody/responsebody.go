// Package responsebody sends the success responses of the net/http-based
// adapters, chi and mux, and records whether a custom encoder has started one.
//
// Once the status is out, an error can no longer be answered: a second status
// is ignored, and an error body would be appended to the response.
package responsebody

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/Infra-Forge/infra-apix/internal/errorhandler"
)

// WriteJSON sends payload as JSON with status. It encodes before writing
// anything, so a payload that cannot be encoded is returned as an error while
// the caller can still answer with an error response. The body is
// json.Encoder's, trailing newline included. A write that fails once the status
// is out is logged as a library event and not returned.
func WriteJSON(w http.ResponseWriter, r *http.Request, status int, payload any) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body.Bytes()); err != nil {
		errorhandler.LogWriteFailed(r, "response write failed", err)
	}
	return nil
}

// Writer is the ResponseWriter a custom encoder receives. It records whether
// the encoder started the response, by a final status, a body write or a
// flush, so the adapter can tell whether an error the encoder returns can
// still be answered.
type Writer struct {
	http.ResponseWriter
	committed bool
}

// NewWriter wraps w.
func NewWriter(w http.ResponseWriter) *Writer {
	return &Writer{ResponseWriter: w}
}

// Committed reports whether the response has started.
func (w *Writer) Committed() bool {
	return w.committed
}

// WriteHeader records a final status. A 1xx status is informational and
// leaves the response open.
func (w *Writer) WriteHeader(status int) {
	if status >= http.StatusOK {
		w.committed = true
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write records that the response has started.
func (w *Writer) Write(b []byte) (int, error) {
	w.committed = true
	return w.ResponseWriter.Write(b)
}

// Flush keeps http.Flusher available to the encoder. A flush sends the
// status, so it starts the response; it does nothing when the underlying
// writer cannot flush.
func (w *Writer) Flush() {
	if http.NewResponseController(w.ResponseWriter).Flush() == nil {
		w.committed = true
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *Writer) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
