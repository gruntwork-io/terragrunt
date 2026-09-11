package portal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	deviceAuthorizationPath = "/api/v1/oauth/device/authorize"

	formContentType = "application/x-www-form-urlencoded"
	jsonContentType = "application/json"

	// defaultPollInterval is the wait RFC 8628 §3.2 assigns a response that
	// names no interval of its own.
	defaultPollInterval = 5 * time.Second

	// maxResponseBytes caps what is read from the portal, so a response that
	// never ends cannot hold a login open indefinitely.
	maxResponseBytes = 1 << 20
)

// DeviceAuthorization is the login request the portal created, waiting for the
// user to approve it in a browser (RFC 8628 §3.2).
type DeviceAuthorization struct {
	DeviceCode              Secret
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               time.Duration
	Interval                time.Duration
}

// deviceAuthorizationBody is the success shape of RFC 8628 §3.2.
type deviceAuthorizationBody struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int64  `json:"expires_in"`
	Interval                int64  `json:"interval"`
}

// AuthorizeDevice asks the portal at baseURL to create a login request the user
// can approve in a browser (RFC 8628 §3.1). A portal that refuses answers with
// an [Error] carrying the [ErrorCode] it named.
func AuthorizeDevice(
	ctx context.Context,
	l log.Logger,
	c vhttp.Client,
	baseURL string,
) (*DeviceAuthorization, error) {
	endpoint, err := url.JoinPath(baseURL, deviceAuthorizationPath)
	if err != nil {
		return nil, fmt.Errorf("building the portal device authorization URL from %q: %w", baseURL, err)
	}

	form := url.Values{
		"client_id": {ClientID},
		"scope":     {ScopeCatalogRead},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building the portal device authorization request: %w", err)
	}

	req.Header.Set("Content-Type", formContentType)
	req.Header.Set("Accept", jsonContentType)

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting device authorization from the portal: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			l.Warnf("Error closing response body: %v", err)
		}
	}()

	body := io.LimitReader(resp.Body, maxResponseBytes)

	if resp.StatusCode != http.StatusOK {
		return nil, newError(resp, body)
	}

	return parseDeviceAuthorization(body)
}

// parseDeviceAuthorization reads a device authorization the portal reported as
// successful. Every field login goes on to use is required here, so a response
// missing one fails now rather than as an empty prompt or an unexchangeable
// code later.
func parseDeviceAuthorization(r io.Reader) (*DeviceAuthorization, error) {
	body := &bodyReader{r: r}

	var raw json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, body.wrap(err)
	}

	if raw[0] != '{' {
		return nil, ErrResponseNotObject
	}

	var parsed deviceAuthorizationBody
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedResponse, err)
	}

	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "device_code", value: parsed.DeviceCode},
		{name: "user_code", value: parsed.UserCode},
		{name: "verification_uri", value: parsed.VerificationURI},
	} {
		if field.value == "" {
			return nil, &MissingFieldError{Field: field.name}
		}
	}

	expiresIn, ok := secondsToDuration(parsed.ExpiresIn)
	if !ok {
		return nil, fmt.Errorf("%w: unusable expires_in of %d", ErrMalformedResponse, parsed.ExpiresIn)
	}

	if err := checkBrowsable(parsed.VerificationURI); err != nil {
		return nil, err
	}

	// RFC 8628 §3.2 makes the pre-filled URL optional, so its absence is not a
	// failure.
	if parsed.VerificationURIComplete != "" {
		if err := checkBrowsable(parsed.VerificationURIComplete); err != nil {
			return nil, err
		}
	}

	interval, ok := secondsToDuration(parsed.Interval)
	if !ok {
		interval = defaultPollInterval
	}

	return &DeviceAuthorization{
		DeviceCode:              Secret(parsed.DeviceCode),
		UserCode:                parsed.UserCode,
		VerificationURI:         parsed.VerificationURI,
		VerificationURIComplete: parsed.VerificationURIComplete,
		ExpiresIn:               expiresIn,
		Interval:                interval,
	}, nil
}

// bodyReader records the last transport failure, so a connection that drops
// partway through is not reported as a response the portal got wrong. The
// decoder sees a truncated document either way and cannot tell the two apart.
type bodyReader struct {
	r       io.Reader
	readErr error
}

func (b *bodyReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		b.readErr = err
	}

	return n, err
}

// wrap reports a decode failure as the transport failure behind it, when there
// was one.
func (b *bodyReader) wrap(err error) error {
	if b.readErr != nil {
		return fmt.Errorf("reading the portal response: %w", b.readErr)
	}

	return fmt.Errorf("%w: %w", ErrMalformedResponse, err)
}

// checkBrowsable rejects a verification URL that is not a web page. Login has
// nowhere to send the user, so it fails here rather than printing a dead URL
// and waiting for an approval that cannot arrive. What such a URL would do to
// the host is the browser opener's concern, and it refuses them too.
func checkBrowsable(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: unparseable verification URL %q: %w", ErrMalformedResponse, rawURL, err)
	}

	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("%w: verification URL %q is not an HTTP URL", ErrMalformedResponse, rawURL)
	}

	if parsed.Host == "" {
		return fmt.Errorf("%w: verification URL %q names no host", ErrMalformedResponse, rawURL)
	}

	return nil
}
