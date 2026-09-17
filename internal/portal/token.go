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
	// tokenPath is where the device code is exchanged for the credential the
	// portal issues once the user approves.
	tokenPath = "/api/v1/oauth/device/token"

	// deviceCodeGrantType names the exchange in RFC 8628 §3.4.
	deviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"

	slowDownIncrement = 5 * time.Second

	// maxTransientFailures bounds the run of failed polls the exchange rides
	// out, so a portal that has stopped answering ends the login while the user
	// is still watching rather than at the authorization's deadline.
	maxTransientFailures = 5

	// pollSlack is the extra attempts [pollLimit] adds. One covers the poll that
	// outlasts the deadline, the other the remainder the division drops.
	pollSlack = 2

	// minPollInterval floors the divisor in [pollLimit]. The portal reports the
	// interval in whole seconds, so anything shorter means it named none.
	minPollInterval = time.Second
)

// Token is the credential the portal issued once the user approved the login
// request, together with the org it reaches. [Secret] keeps the credential out
// of terminal output and log lines.
type Token struct {
	AccessToken Secret
	TokenType   string
	Scope       string
	Org         Org
	Account     Account
	ExpiresIn   time.Duration
}

// Org is the Gruntwork organization a [Token] is scoped to. The ID is the
// stable key. The name is shown to the user and read again on every login, so
// an org renamed in the portal keeps the entry it already had.
type Org struct {
	ID   string
	Name string
}

// Account is who the portal says approved the login. The portal may omit it,
// in which case the confirmation line names the org alone.
type Account struct {
	Email string
}

// tokenBody is the success shape of RFC 6749 §5.1, plus the org and account the
// portal adds so login can name what the token reaches without a second call.
type tokenBody struct {
	AccessToken string      `json:"access_token"`
	TokenType   string      `json:"token_type"`
	Scope       string      `json:"scope"`
	Org         orgBody     `json:"org"`
	Account     accountBody `json:"account"`
	ExpiresIn   int64       `json:"expires_in"`
}

type orgBody struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type accountBody struct {
	Email string `json:"email"`
}

// PollToken exchanges the device code in auth for a token, waiting for the user
// to approve the login request in their browser (RFC 8628 §3.4). It blocks
// until the portal issues a token, until the user refuses, until auth runs out
// of time, or until ctx is cancelled.
//
// A refusal returns [ErrLoginDenied] and a request that expired unanswered
// returns [ErrLoginExpired]. Anything else the portal refuses comes back as an
// [Error] carrying the [ErrorCode] it named.
func PollToken(
	ctx context.Context,
	l log.Logger,
	c vhttp.Client,
	baseURL string,
	auth *DeviceAuthorization,
) (*Token, error) {
	endpoint, err := url.JoinPath(baseURL, tokenPath)
	if err != nil {
		return nil, fmt.Errorf("building the portal token URL from %q: %w", baseURL, err)
	}

	form := url.Values{
		"grant_type":  {deviceCodeGrantType},
		"device_code": {auth.DeviceCode.Reveal()},
		"client_id":   {ClientID},
	}

	pollCtx, cancel := context.WithTimeout(ctx, auth.ExpiresIn)
	defer cancel()

	c = withoutRedirects(c)
	p := &poller{interval: auth.Interval}

	for range pollLimit(auth.ExpiresIn, auth.Interval) {
		if err := wait(pollCtx, p.interval); err != nil {
			return nil, pollFailure(ctx, pollCtx, err)
		}

		token, err := exchangeDeviceCode(pollCtx, l, c, endpoint, form)
		if err == nil {
			return token, nil
		}

		if pollCtx.Err() != nil {
			return nil, pollFailure(ctx, pollCtx, err)
		}

		if again, terminal := p.next(l, err); !again {
			return nil, terminal
		}
	}

	return nil, ErrPollLimit
}

// poller carries what the portal's last answer changed about the next attempt.
type poller struct {
	interval time.Duration
	failures int
}

// next reports whether err leaves the login open, in which case the caller waits
// out the interval and tries again. When it does not, the error returned beside
// it is what the login ends with.
func (p *poller) next(l log.Logger, err error) (bool, error) {
	if transient(err) {
		p.failures++
		if p.failures > maxTransientFailures {
			return false, err
		}

		return true, nil
	}

	p.failures = 0

	var portalErr *Error
	if !errors.As(err, &portalErr) {
		return false, err
	}

	switch portalErr.Code {
	case ErrorCodeAuthorizationPending:
		return true, nil
	case ErrorCodeSlowDown:
		// RFC 8628 §3.5 adds five seconds per slow_down. Retry-After can ask for
		// longer, so the wait becomes whichever of the two is greater.
		p.interval = max(p.interval+slowDownIncrement, portalErr.RetryAfter)

		return true, nil
	case ErrorCodeAccessDenied:
		return false, ErrLoginDenied
	case ErrorCodeExpiredToken:
		return false, ErrLoginExpired
	case ErrorCodeInvalidRequest, ErrorCodeInvalidClient, ErrorCodeInvalidScope, ErrorCodeServerError:
		return false, err
	default:
		l.Debugf("The portal refused the login with the unrecognized code %q", portalErr.Code)

		return false, err
	}
}

// exchangeDeviceCode makes one attempt at the token endpoint. A portal that
// refuses answers with an [Error] carrying the [ErrorCode] it named, which is
// how the caller learns whether to poll again.
func exchangeDeviceCode(
	ctx context.Context,
	l log.Logger,
	c vhttp.Client,
	endpoint string,
	form url.Values,
) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("building the portal token request: %w", err)
	}

	req.Header.Set("Content-Type", formContentType)
	req.Header.Set("Accept", jsonContentType)

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchanging the device code with the portal: %w", err)
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

	return parseToken(body)
}

// parseToken reads a token the portal reported as issued. The credential, the
// org it is scoped to, and its lifetime are required, because nothing
// downstream can store or send the token without them. The org name and the
// account email are only shown to the user, so a response that omits them still
// logs that user in.
func parseToken(r io.Reader) (*Token, error) {
	body := &bodyReader{r: r}

	var raw json.RawMessage
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, body.wrap(err)
	}

	if raw[0] != '{' {
		return nil, ErrResponseNotObject
	}

	var parsed tokenBody
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformedResponse, err)
	}

	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "access_token", value: parsed.AccessToken},
		{name: "org.id", value: parsed.Org.ID},
	} {
		if field.value == "" {
			return nil, &MissingFieldError{Field: field.name}
		}
	}

	expiresIn, ok := secondsToDuration(parsed.ExpiresIn)
	if !ok {
		return nil, fmt.Errorf("%w: unusable expires_in of %d", ErrMalformedResponse, parsed.ExpiresIn)
	}

	return &Token{
		AccessToken: Secret(parsed.AccessToken),
		TokenType:   parsed.TokenType,
		Scope:       parsed.Scope,
		Org:         Org{ID: parsed.Org.ID, Name: parsed.Org.Name},
		Account:     Account{Email: parsed.Account.Email},
		ExpiresIn:   expiresIn,
	}, nil
}

// withoutRedirects returns c with redirect following turned off. The stdlib
// resends a POST body on a 307 or 308, which would carry the device code to
// whatever host the response names.
func withoutRedirects(c vhttp.Client) vhttp.Client {
	cc := *c
	cc.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &cc
}

// transient reports a poll that failed for a reason that may not outlast the
// next interval. A body the portal did send but the CLI cannot read is not one
// of them, because the next poll gets the same answer.
func transient(err error) bool {
	if errors.Is(err, ErrMalformedResponse) {
		return false
	}

	var portalErr *Error
	if !errors.As(err, &portalErr) {
		return true
	}

	return portalErr.Code == "" || portalErr.StatusCode >= http.StatusInternalServerError
}

// pollLimit is how many attempts the authorization's lifetime leaves room for.
// The interval only grows, so the one the portal opened with fits the most
// polls in.
func pollLimit(expiresIn, interval time.Duration) int64 {
	return int64(expiresIn/max(interval, minPollInterval)) + pollSlack
}

func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// pollFailure reports the authorization running out of time as
// [ErrLoginExpired]. The caller giving up, the authorization expiring, and a
// timeout the HTTP client puts on a single request all reach here as a context
// error, so which context is done is what tells them apart.
func pollFailure(ctx, pollCtx context.Context, err error) error {
	if pollCtx.Err() != nil && ctx.Err() == nil {
		return ErrLoginExpired
	}

	return err
}
