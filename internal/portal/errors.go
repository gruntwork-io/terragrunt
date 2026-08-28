package portal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// ErrMalformedResponse reports a response the portal did not refuse but that
// cannot be read as the expected shape. It is distinct from
// a failure to read the response at all, which names the transport error
// instead, and from an [Error], which is the portal refusing on purpose.
var ErrMalformedResponse = errors.New("malformed portal response")

// ErrResponseNotObject reports a response body that is well-formed JSON but not
// an object.
var ErrResponseNotObject = fmt.Errorf("%w: not a JSON object", ErrMalformedResponse)

// MissingFieldError reports a response that left out a field the CLI needs. It
// unwraps to [ErrMalformedResponse], so a caller that does not care which field
// was missing can match the class instead.
type MissingFieldError struct {
	Field string
}

func (e *MissingFieldError) Error() string {
	return ErrMalformedResponse.Error() + ": missing " + e.Field
}

func (e *MissingFieldError) Unwrap() error {
	return ErrMalformedResponse
}

// ErrorCode names a failure the portal describes in its response body. Callers
// reach it by unwrapping an [Error] out of the returned error.
type ErrorCode string

const (
	// ErrorCodeInvalidRequest reports a request body the portal could not read.
	ErrorCodeInvalidRequest ErrorCode = "invalid_request"

	// ErrorCodeInvalidClient reports a client the portal has no registration for.
	ErrorCodeInvalidClient ErrorCode = "invalid_client"

	// ErrorCodeInvalidScope reports a scope that was missing, unknown, or not
	// permitted for the client.
	ErrorCodeInvalidScope ErrorCode = "invalid_scope"

	// ErrorCodeSlowDown reports a rate limit. [Error.RetryAfter] carries how
	// long the portal asked the caller to wait.
	ErrorCodeSlowDown ErrorCode = "slow_down"

	// ErrorCodeServerError reports an unexpected failure inside the portal.
	ErrorCodeServerError ErrorCode = "server_error"
)

// Error reports a request the portal refused.
type Error struct {
	Code        ErrorCode
	Description string
	RetryAfter  time.Duration
	StatusCode  int
}

func (e *Error) Error() string {
	if e.Code == "" {
		return "portal responded with unexpected status " + strconv.Itoa(e.StatusCode)
	}

	msg := "portal rejected the request: " + string(e.Code)
	if e.Description != "" {
		msg += ": " + e.Description
	}

	return msg
}

// errorBody is the failure shape of RFC 6749 §5.2.
type errorBody struct {
	Code        ErrorCode `json:"error"`
	Description string    `json:"error_description"`
}

// newError reads what the portal said about a refusal. A body that is not the
// documented shape leaves Code empty. A gateway answering in the portal's place
// uses a format of its own, so only its status identifies the failure.
func newError(resp *http.Response, body io.Reader) *Error {
	err := &Error{
		StatusCode: resp.StatusCode,
		RetryAfter: retryAfter(resp.Header),
	}

	var parsed errorBody
	if json.NewDecoder(body).Decode(&parsed) == nil {
		err.Code = parsed.Code
		err.Description = parsed.Description
	}

	return err
}

// retryAfter reads the delta-seconds form of Retry-After (RFC 9110 §10.2.3),
// which is what the portal sends. An HTTP-date, or a header the portal omitted,
// reads as no advice.
func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.Atoi(header.Get("Retry-After"))
	if err != nil || seconds < 0 {
		return 0
	}

	return time.Duration(seconds) * time.Second
}
