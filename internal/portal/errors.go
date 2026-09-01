package portal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// ErrLoginDenied reports a login request the user refused at the portal.
var ErrLoginDenied = errors.New("the login request was denied")

// ErrLoginExpired reports a login request that ran out of time before the user
// approved it.
var ErrLoginExpired = errors.New("the login request expired before it was approved")

// ErrPollLimit reports a poll loop that ran past the attempts its authorization
// allows. An ordinary login ends on that deadline first, so this reports a bug
// here rather than anything the portal did.
var ErrPollLimit = errors.New("the login poll ran past the attempts its request allows")

// ErrCredentialRejected reports a credential the portal will not accept, which
// is what an expired one and a withdrawn one both come back as.
var ErrCredentialRejected = errors.New("the portal rejected the stored credential")

// ErrNoHostedCatalog reports a portal serving no catalog.
var ErrNoHostedCatalog = errors.New("the portal serves no catalog")

// ErrPortalUnreachable reports a portal that gave no answer at all: the request
// failed to arrive, the connection broke, or the CLI stopped waiting for a
// reply.
var ErrPortalUnreachable = errors.New("the portal could not be reached")

// RateLimitedError reports a portal that rate limited the CLI and went on doing
// so for every retry.
type RateLimitedError struct {
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	if e.RetryAfter > 0 {
		return "the portal is rate limiting requests; it asked to wait " + e.RetryAfter.String() + " before trying again"
	}

	return "the portal is rate limiting requests; try again shortly"
}

// ErrUnusablePortalURL reports a portal base URL the CLI cannot address, and so
// cannot file a credential under either. A caller matches it to tell a base URL
// the user has to correct from a failure it can do nothing about.
var ErrUnusablePortalURL = errors.New("unusable portal base URL")

// ErrNoPortalHost reports a base URL naming no host, which is what a bare
// hostname with no scheme in front of it parses as.
var ErrNoPortalHost = fmt.Errorf("%w: it names no host", ErrUnusablePortalURL)

// ErrPortalSchemeUnsupported reports a base URL the CLI cannot send a request
// to, and whose credentials it therefore cannot keep apart from those of
// another scheme.
var ErrPortalSchemeUnsupported = fmt.Errorf("%w: only http and https are addressable", ErrUnusablePortalURL)

// UnusablePortalURLError carries the address that could not be used. It unwraps
// to the reason and, through that, to [ErrUnusablePortalURL], so a caller can
// match either. [UnusablePortalURLError.Error] redacts the address, which may
// carry a password.
type UnusablePortalURLError struct {
	Err error
	URL string
}

func (e *UnusablePortalURLError) Error() string {
	return fmt.Sprintf("%q: %v", redactURL(e.URL), e.Err)
}

func (e *UnusablePortalURLError) Unwrap() error {
	return e.Err
}

// redactURL replaces the password in rawURL. An address [url.Parse] rejects is
// returned unchanged.
func redactURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	return parsed.Redacted()
}

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

	// ErrorCodeFeatureNotEnabled reports a portal that will not serve the CLI.
	// It is the portal's own code rather than one RFC 6749 §5.2 defines.
	ErrorCodeFeatureNotEnabled ErrorCode = "feature_not_enabled"

	// ErrorCodeSlowDown reports a rate limit. [Error.RetryAfter] carries how
	// long the portal asked the caller to wait.
	ErrorCodeSlowDown ErrorCode = "slow_down"

	// ErrorCodeServerError reports an unexpected failure inside the portal.
	ErrorCodeServerError ErrorCode = "server_error"

	// ErrorCodeAuthorizationPending reports a login request the user has not
	// answered yet. This is an expected sentinel error indicating
	// that polling should continue at the same rate.
	ErrorCodeAuthorizationPending ErrorCode = "authorization_pending"

	// ErrorCodeAccessDenied reports a login request the user refused.
	ErrorCodeAccessDenied ErrorCode = "access_denied"

	// ErrorCodeExpiredToken reports a login request the portal discarded because
	// nobody answered it in time.
	ErrorCodeExpiredToken ErrorCode = "expired_token"
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
// which is what the portal sends. An HTTP-date, a count that overflows a
// [time.Duration], or a header the portal omitted, reads as no advice.
func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.Atoi(header.Get("Retry-After"))
	if err != nil {
		return 0
	}

	delay, ok := secondsToDuration(int64(seconds))
	if !ok {
		return 0
	}

	return delay
}
