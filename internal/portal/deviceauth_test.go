package portal_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const authorizationBody = `{
	"device_code": "fake-device-code",
	"user_code": "FAKE-CODE",
	"verification_uri": "https://portal.example.com/auth/device",
	"verification_uri_complete": "https://portal.example.com/auth/device?user_code=FAKE-CODE",
	"expires_in": 600,
	"interval": 5
}`

// respondJSON builds a client that answers every request with status and body,
// recording the request it answered.
func respondJSON(recorded **http.Request, status int, body string, header http.Header) vhttp.Client {
	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		if recorded != nil {
			*recorded = req
		}

		if header == nil {
			header = http.Header{}
		}

		header.Set("Content-Type", "application/json")

		return vhttp.Respond(status, []byte(body), header), nil
	})
}

func TestAuthorizeDeviceRequest(t *testing.T) {
	t.Parallel()

	var got *http.Request

	c := respondJSON(&got, http.StatusOK, authorizationBody, nil)

	_, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com/")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, http.MethodPost, got.Method)
	assert.Equal(t, "https://portal.example.com/api/v1/oauth/device/authorize", got.URL.String())
	assert.Equal(t, "application/x-www-form-urlencoded", got.Header.Get("Content-Type"))
	assert.Equal(t, "application/json", got.Header.Get("Accept"))

	body, err := io.ReadAll(got.Body)
	require.NoError(t, err)

	form, err := url.ParseQuery(string(body))
	require.NoError(t, err)

	assert.Equal(t, "terragrunt-cli", form.Get("client_id"))
	assert.Equal(t, "catalog:read", form.Get("scope"))
}

func TestAuthorizeDeviceResponse(t *testing.T) {
	t.Parallel()

	c := respondJSON(nil, http.StatusOK, authorizationBody, nil)

	auth, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")
	require.NoError(t, err)

	assert.Equal(t, "fake-device-code", auth.DeviceCode.Reveal())
	assert.Equal(t, "FAKE-CODE", auth.UserCode)
	assert.Equal(t, "https://portal.example.com/auth/device", auth.VerificationURI)
	assert.Equal(
		t,
		"https://portal.example.com/auth/device?user_code=FAKE-CODE",
		auth.VerificationURIComplete,
	)
	assert.Equal(t, 600*time.Second, auth.ExpiresIn)
	assert.Equal(t, 5*time.Second, auth.Interval)
}

// TestAuthorizeDeviceDefaultsInterval pins the five-second wait RFC 8628 §3.2
// assigns a response that names no polling interval.
func TestAuthorizeDeviceDefaultsInterval(t *testing.T) {
	t.Parallel()

	body := `{
		"device_code": "fake-device-code",
		"user_code": "FAKE-CODE",
		"verification_uri": "https://portal.example.com/auth/device",
		"expires_in": 600
	}`

	c := respondJSON(nil, http.StatusOK, body, nil)

	auth, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")
	require.NoError(t, err)

	assert.Equal(t, 5*time.Second, auth.Interval)
	assert.Empty(t, auth.VerificationURIComplete)
}

func TestAuthorizeDeviceRejectsUnusableResponse(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		body string
	}{
		{name: "not json", body: `<html>hello</html>`},
		{name: "wrong field type", body: `{"device_code":"fake-device-code","user_code":"FAKE-CODE",` +
			`"verification_uri":"https://p.example.com/d","expires_in":"600"}`},
		{
			name: "already expired",
			body: `{"device_code":"fake-device-code","user_code":"FAKE-CODE","verification_uri":"https://p.example.com/d"}`,
		},
		{
			name: "unbrowsable verification uri",
			body: `{"device_code":"fake-device-code","user_code":"FAKE-CODE","verification_uri":"javascript:fake-script","expires_in":600}`,
		},
		{
			name: "unbrowsable complete verification uri",
			body: `{"device_code":"fake-device-code","user_code":"FAKE-CODE","verification_uri":"https://p.example.com/d",` +
				`"verification_uri_complete":"file:///fake/path","expires_in":600}`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := respondJSON(nil, http.StatusOK, tt.body, nil)

			auth, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")
			require.ErrorIs(t, err, portal.ErrMalformedResponse)
			assert.Nil(t, auth)
		})
	}
}

// TestAuthorizeDeviceRejectsMissingField pins which field each check rejects.
// Matching only the malformed-response class cannot tell them apart, because a
// field left unchecked fails later for a different reason.
func TestAuthorizeDeviceRejectsMissingField(t *testing.T) {
	t.Parallel()

	tc := []struct {
		field string
		body  string
	}{
		{
			field: "device_code",
			body:  `{"user_code":"FAKE-CODE","verification_uri":"https://p.example.com/d","expires_in":600}`,
		},
		{
			field: "user_code",
			body:  `{"device_code":"fake-device-code","verification_uri":"https://p.example.com/d","expires_in":600}`,
		},
		{
			field: "verification_uri",
			body:  `{"device_code":"fake-device-code","user_code":"FAKE-CODE","expires_in":600}`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()

			c := respondJSON(nil, http.StatusOK, tt.body, nil)

			_, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")

			var missing *portal.MissingFieldError
			require.ErrorAs(t, err, &missing)
			assert.Equal(t, tt.field, missing.Field)
			require.ErrorIs(t, err, portal.ErrMalformedResponse)
		})
	}
}

// TestAuthorizeDeviceRejectsNonObjectResponse pins the message for a response
// that is valid JSON but not an object. A JSON null unmarshals into the struct
// without error, so it would otherwise be reported as a missing field, and the
// rest would be reported by naming a Go type the user has no use for.
func TestAuthorizeDeviceRejectsNonObjectResponse(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`null`, `[1,2,3]`, `"hello"`, `42`} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()

			c := respondJSON(nil, http.StatusOK, body, nil)

			_, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")
			require.ErrorIs(t, err, portal.ErrResponseNotObject)
		})
	}
}

func TestAuthorizeDeviceReportsRefusal(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name            string
		body            string
		wantDescription string
		header          http.Header
		wantCode        portal.ErrorCode
		status          int
		wantRetryAfter  time.Duration
	}{
		{
			name:            "unknown client",
			status:          http.StatusBadRequest,
			body:            `{"error":"invalid_client","error_description":"Unknown OAuth client."}`,
			wantCode:        portal.ErrorCodeInvalidClient,
			wantDescription: "Unknown OAuth client.",
		},
		{
			name:            "disallowed scope",
			status:          http.StatusBadRequest,
			body:            `{"error":"invalid_scope","error_description":"The scope is not permitted."}`,
			wantCode:        portal.ErrorCodeInvalidScope,
			wantDescription: "The scope is not permitted.",
		},
		{
			name:            "rate limited",
			status:          http.StatusTooManyRequests,
			body:            `{"error":"slow_down","error_description":"Too many login requests."}`,
			header:          http.Header{"Retry-After": {"37"}},
			wantCode:        portal.ErrorCodeSlowDown,
			wantDescription: "Too many login requests.",
			wantRetryAfter:  37 * time.Second,
		},
		{
			name:            "portal failure",
			status:          http.StatusInternalServerError,
			body:            `{"error":"server_error","error_description":"Internal server error."}`,
			wantCode:        portal.ErrorCodeServerError,
			wantDescription: "Internal server error.",
		},
		{
			name:   "intermediary standing in",
			status: http.StatusBadGateway,
			body:   `<html>502 Bad Gateway</html>`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := respondJSON(nil, tt.status, tt.body, tt.header)

			_, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")

			var portalErr *portal.Error
			require.ErrorAs(t, err, &portalErr)

			assert.Equal(t, tt.wantCode, portalErr.Code)
			assert.Equal(t, tt.wantDescription, portalErr.Description)
			assert.Equal(t, tt.status, portalErr.StatusCode)
			assert.Equal(t, tt.wantRetryAfter, portalErr.RetryAfter)
		})
	}
}

// TestAuthorizeDeviceIgnoresUnreadableRetryAfter pins that the HTTP-date form of
// Retry-After, which the portal does not send, reads as no advice rather than as
// an immediate retry.
func TestAuthorizeDeviceIgnoresUnreadableRetryAfter(t *testing.T) {
	t.Parallel()

	c := respondJSON(
		nil,
		http.StatusTooManyRequests,
		`{"error":"slow_down"}`,
		http.Header{"Retry-After": {"Wed, 21 Oct 2026 07:28:00 GMT"}},
	)

	_, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")

	var portalErr *portal.Error
	require.ErrorAs(t, err, &portalErr)

	assert.Zero(t, portalErr.RetryAfter)
}

// truncatedReader serves prefix and then fails, standing in for a connection
// that drops partway through a response.
type truncatedReader struct {
	err    error
	prefix []byte
	read   int
}

func (r *truncatedReader) Read(p []byte) (int, error) {
	if r.read >= len(r.prefix) {
		return 0, r.err
	}

	n := copy(p, r.prefix[r.read:])
	r.read += n

	return n, nil
}

// TestAuthorizeDeviceReportsTruncatedBodyAsRead pins that a connection dropping
// partway through a response is reported as the read failure it is. The decoder
// sees the same truncated document either way, so without the distinction a
// dropped connection reads as a portal sending malformed JSON.
func TestAuthorizeDeviceReportsTruncatedBodyAsRead(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("connection reset by peer")

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(&truncatedReader{
				prefix: []byte(`{"device_code":"fake-device-code",`),
				err:    sentinel,
			}),
		}, nil
	})

	_, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")
	require.ErrorIs(t, err, sentinel)
	require.NotErrorIs(t, err, portal.ErrMalformedResponse)
}

func TestAuthorizeDevicePropagatesTransportFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("dial failed")

	c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return nil, sentinel
	})

	_, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "https://portal.example.com")
	require.ErrorIs(t, err, sentinel)
}

// TestAuthorizeDeviceCarriesContext pins that the caller's context reaches the
// request, so a cancelled login abandons an in-flight call instead of waiting
// out the client's timeout.
func TestAuthorizeDeviceCarriesContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	c := vhttp.NewMemClient(func(reqCtx context.Context, _ *http.Request) (*http.Response, error) {
		return nil, reqCtx.Err()
	})

	_, err := portal.AuthorizeDevice(ctx, logger.CreateLogger(), c, "https://portal.example.com")
	require.ErrorIs(t, err, context.Canceled)
}

func TestAuthorizeDeviceRejectsUnusableBaseURL(t *testing.T) {
	t.Parallel()

	c := respondJSON(nil, http.StatusOK, authorizationBody, nil)

	_, err := portal.AuthorizeDevice(t.Context(), logger.CreateLogger(), c, "://portal.example.com")
	require.Error(t, err)
}

// TestSecretDoesNotReachLogOutput pins what [portal.Secret] exists for: a
// device authorization rendered through Terragrunt's own logger carries the
// user code and not the device code. Going through the real formatter rather
// than fmt covers the formatter, which is free to render a value however it
// likes.
//
// This guards accidental rendering. A deliberate [portal.Secret.Reveal] prints
// the credential by design, and covering everything the login command writes
// needs the command itself.
func TestSecretDoesNotReachLogOutput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	l := logger.CreateLogger().WithOptions(log.WithOutput(&buf))

	auth := &portal.DeviceAuthorization{
		DeviceCode: portal.Secret("fake-device-code"),
		UserCode:   "FAKE-CODE",
	}

	l.Infof("device authorization: %v", auth)
	l.Infof("device authorization: %+v", auth)

	assert.NotContains(t, buf.String(), "fake-device-code")
	assert.Contains(t, buf.String(), "FAKE-CODE", "the user code is meant to be shown")

	// %#v does not consult String, so GoString is what keeps a struct dump from
	// carrying the code.
	assert.NotContains(t, fmt.Sprintf("%#v", auth), "fake-device-code")
}

func TestErrorMessage(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		err  *portal.Error
		want string
	}{
		{
			name: "code and description",
			err:  &portal.Error{Code: portal.ErrorCodeInvalidScope, Description: "Not permitted.", StatusCode: 400},
			want: "portal rejected the request: invalid_scope: Not permitted.",
		},
		{
			name: "code alone",
			err:  &portal.Error{Code: portal.ErrorCodeSlowDown, StatusCode: 429},
			want: "portal rejected the request: slow_down",
		},
		{
			name: "status alone",
			err:  &portal.Error{StatusCode: 502},
			want: "portal responded with unexpected status 502",
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}
