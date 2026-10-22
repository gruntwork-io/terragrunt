package router

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// ResponseWriter is the [http.ResponseWriter] handlers receive. It remembers
// the status it sent so middleware can log it and [WriteError] can tell
// whether a response is already under way.
type ResponseWriter interface {
	http.ResponseWriter

	// Status returns the status code sent, or 0 while the header is unsent.
	Status() int
}

// NewResponseWriter wraps w so the status it sends can be read back.
func NewResponseWriter(w http.ResponseWriter) ResponseWriter {
	return &responseWriter{ResponseWriter: w}
}

type responseWriter struct {
	http.ResponseWriter

	status int
}

// WriteHeader sends code once. Later calls are dropped, since net/http would
// only log them as superfluous.
func (w *responseWriter) WriteHeader(code int) {
	if w.status != 0 {
		return
	}

	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}

	return w.ResponseWriter.Write(b)
}

func (w *responseWriter) Status() int {
	return w.status
}

// Unwrap lets [http.ResponseController] reach the flusher of the underlying
// writer, which the reverse proxy streams through.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// HTTPError is an error a handler returns to answer with a specific status.
type HTTPError struct {
	Message string
	Code    int
}

// NewHTTPError returns an HTTPError whose message is the standard text for code.
func NewHTTPError(code int) *HTTPError {
	return &HTTPError{
		Code:    code,
		Message: http.StatusText(code),
	}
}

func (err *HTTPError) Error() string {
	return fmt.Sprintf("code=%d, message=%s", err.Code, err.Message)
}

// WriteError answers with err as a plain-text response. An [HTTPError] sets
// the status and body; any other error is reported as a 500 without its text,
// which may name internal paths or hosts. A response already under way is
// left alone.
func WriteError(w ResponseWriter, err error) {
	if w.Status() != 0 {
		return
	}

	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		httpErr = NewHTTPError(http.StatusInternalServerError)
	}

	http.Error(w, httpErr.Message, httpErr.Code)
}

// JSON answers with v encoded as JSON under the given status.
func JSON(w http.ResponseWriter, code int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)

	return json.NewEncoder(w).Encode(v)
}
