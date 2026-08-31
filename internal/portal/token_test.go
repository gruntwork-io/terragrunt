package portal_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const portalBaseURL = "https://portal.example.com"

const issuedTokenBody = `{
	"access_token": "fake-access-token",
	"token_type": "Bearer",
	"expires_in": 2592000,
	"scope": "catalog:read",
	"org": {"id": "org_fake", "name": "Acme"},
	"account": {"email": "someone@example.com"}
}`

const (
	pendingBody  = `{"error":"authorization_pending","error_description":"The user has not answered yet."}`
	slowDownBody = `{"error":"slow_down","error_description":"Polling too fast."}`
	deniedBody   = `{"error":"access_denied","error_description":"The user declined."}`
	expiredBody  = `{"error":"expired_token","error_description":"The login request expired."}`
	faultBody    = `{"error":"server_error","error_description":"Internal server error."}`
	gatewayBody  = `<html>502 Bad Gateway</html>`
)

// pollsBeforeGivingUp is how many polls a portal that keeps failing gets: the
// run the poller rides out, and the one that ends the login.
const pollsBeforeGivingUp = 6

// pollResponse is one answer a stub portal gives a poll.
type pollResponse struct {
	header http.Header
	body   string
	status int
}

// portalStub answers polls in turn, repeating its last answer once the list
// runs out. It records when each poll arrived, so a test can assert the wait
// between them.
type portalStub struct {
	answers  []pollResponse
	requests []*http.Request
	forms    []url.Values
	at       []time.Time
}

func (p *portalStub) client() vhttp.Client {
	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		form, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		p.requests = append(p.requests, req)
		p.forms = append(p.forms, form)
		p.at = append(p.at, time.Now())

		answer := p.answers[min(len(p.at)-1, len(p.answers)-1)]

		header := http.Header{"Content-Type": {"application/json"}}
		maps.Copy(header, answer.header)

		return vhttp.Respond(answer.status, []byte(answer.body), header), nil
	})
}

// gaps is how long the poller waited before each poll, counting from since.
func (p *portalStub) gaps(since time.Time) []time.Duration {
	gaps := make([]time.Duration, 0, len(p.at))

	for _, at := range p.at {
		gaps = append(gaps, at.Sub(since))
		since = at
	}

	return gaps
}

func refusedWith(body string) *portalStub {
	return &portalStub{answers: []pollResponse{{status: http.StatusBadRequest, body: body}}}
}

func answeredWith(status int, body string) *portalStub {
	return &portalStub{answers: []pollResponse{{status: status, body: body}}}
}

// testAuthorization is a login request the user has not answered yet, with the
// ten-minute lifetime and five-second interval a portal would send.
func testAuthorization() *portal.DeviceAuthorization {
	return &portal.DeviceAuthorization{
		DeviceCode:      portal.Secret("fake-device-code"),
		UserCode:        "FAKE-CODE",
		VerificationURI: portalBaseURL + "/auth/device",
		ExpiresIn:       10 * time.Minute,
		Interval:        5 * time.Second,
	}
}

func poll(t *testing.T, c vhttp.Client, auth *portal.DeviceAuthorization) (*portal.Token, error) {
	t.Helper()

	return portal.PollToken(t.Context(), logger.CreateLogger(), c, portalBaseURL, auth)
}

func TestPollTokenRequest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := answeredWith(http.StatusOK, issuedTokenBody)

		_, err := portal.PollToken(
			t.Context(),
			logger.CreateLogger(),
			stub.client(),
			portalBaseURL+"/",
			testAuthorization(),
		)
		require.NoError(t, err)
		require.Len(t, stub.requests, 1)

		got := stub.requests[0]
		assert.Equal(t, http.MethodPost, got.Method)
		assert.Equal(t, portalBaseURL+"/api/v1/oauth/device/token", got.URL.String())
		assert.Equal(t, "application/x-www-form-urlencoded", got.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", got.Header.Get("Accept"))

		form := stub.forms[0]
		assert.Equal(t, "urn:ietf:params:oauth:grant-type:device_code", form.Get("grant_type"))
		assert.Equal(t, "fake-device-code", form.Get("device_code"))
		assert.Equal(t, "terragrunt-cli", form.Get("client_id"))
	})
}

func TestPollTokenReturnsTheApprovedToken(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &portalStub{answers: []pollResponse{
			{status: http.StatusBadRequest, body: pendingBody},
			{status: http.StatusBadRequest, body: pendingBody},
			{status: http.StatusOK, body: issuedTokenBody},
		}}

		token, err := poll(t, stub.client(), testAuthorization())
		require.NoError(t, err)

		assert.Equal(t, "fake-access-token", token.AccessToken.Reveal())
		assert.Equal(t, "Bearer", token.TokenType)
		assert.Equal(t, "catalog:read", token.Scope)
		assert.Equal(t, portal.Org{ID: "org_fake", Name: "Acme"}, token.Org)
		assert.Equal(t, portal.Account{Email: "someone@example.com"}, token.Account)
		assert.Equal(t, 720*time.Hour, token.ExpiresIn)

		assert.Len(t, stub.at, 3, "a pending answer is waited out, not reported")
	})
}

// TestPollTokenKeepsAnIssuedTokenWithoutAnAccount pins that a response with no
// account still logs the user in. The email address is only shown back to
// them.
func TestPollTokenKeepsAnIssuedTokenWithoutAnAccount(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		body := `{"access_token":"fake-access-token","token_type":"Bearer","expires_in":2592000,` +
			`"org":{"id":"org_fake","name":"Acme"}}`

		token, err := poll(t, answeredWith(http.StatusOK, body).client(), testAuthorization())
		require.NoError(t, err)

		assert.Equal(t, "org_fake", token.Org.ID)
		assert.Empty(t, token.Account.Email)
	})
}

// TestPollTokenWaitsTheGivenInterval pins that the first poll waits too. A poll
// sent the instant the user is shown the code cannot find an approval yet.
func TestPollTokenWaitsTheGivenInterval(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := &portalStub{answers: []pollResponse{
			{status: http.StatusBadRequest, body: pendingBody},
			{status: http.StatusBadRequest, body: pendingBody},
			{status: http.StatusOK, body: issuedTokenBody},
		}}

		auth := testAuthorization()
		auth.Interval = 7 * time.Second

		start := time.Now()

		_, err := poll(t, stub.client(), auth)
		require.NoError(t, err)

		assert.Equal(t, []time.Duration{7 * time.Second, 7 * time.Second, 7 * time.Second}, stub.gaps(start))
	})
}

// TestPollTokenWidensTheIntervalOnSlowDown pins the arithmetic of RFC 8628
// §3.5: five more seconds per slow_down, and the portal's own Retry-After when
// it asks for longer than that.
func TestPollTokenWidensTheIntervalOnSlowDown(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		answers  []pollResponse
		wantGaps []time.Duration
	}{
		{
			name: "five more seconds",
			answers: []pollResponse{
				{status: http.StatusBadRequest, body: slowDownBody},
				{status: http.StatusOK, body: issuedTokenBody},
			},
			wantGaps: []time.Duration{5 * time.Second, 10 * time.Second},
		},
		{
			name: "each slow_down widens again",
			answers: []pollResponse{
				{status: http.StatusBadRequest, body: slowDownBody},
				{status: http.StatusBadRequest, body: slowDownBody},
				{status: http.StatusOK, body: issuedTokenBody},
			},
			wantGaps: []time.Duration{5 * time.Second, 10 * time.Second, 15 * time.Second},
		},
		{
			name: "the portal asks for longer",
			answers: []pollResponse{
				{
					status: http.StatusTooManyRequests,
					body:   slowDownBody,
					header: http.Header{"Retry-After": {"30"}},
				},
				{status: http.StatusOK, body: issuedTokenBody},
			},
			wantGaps: []time.Duration{5 * time.Second, 30 * time.Second},
		},
		{
			name: "the portal asks for less than the increment",
			answers: []pollResponse{
				{
					status: http.StatusTooManyRequests,
					body:   slowDownBody,
					header: http.Header{"Retry-After": {"2"}},
				},
				{status: http.StatusOK, body: issuedTokenBody},
			},
			wantGaps: []time.Duration{5 * time.Second, 10 * time.Second},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				stub := &portalStub{answers: tt.answers}
				start := time.Now()

				_, err := poll(t, stub.client(), testAuthorization())
				require.NoError(t, err)

				assert.Equal(t, tt.wantGaps, stub.gaps(start))
			})
		})
	}
}

func TestPollTokenReportsRefusalByTheUser(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := refusedWith(deniedBody)

		token, err := poll(t, stub.client(), testAuthorization())
		require.ErrorIs(t, err, portal.ErrLoginDenied)
		require.NotErrorIs(t, err, portal.ErrLoginExpired)

		assert.Nil(t, token)
		assert.Len(t, stub.at, 1, "a refusal is final")
	})
}

func TestPollTokenReportsARequestThePortalExpired(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := refusedWith(expiredBody)

		token, err := poll(t, stub.client(), testAuthorization())
		require.ErrorIs(t, err, portal.ErrLoginExpired)
		require.NotErrorIs(t, err, portal.ErrLoginDenied)

		assert.Nil(t, token)
		assert.Len(t, stub.at, 1)
	})
}

// TestPollTokenStopsAtTheAuthorizationDeadline pins that the authorization's own
// lifetime ends the polling. A portal that answers authorization_pending forever
// would otherwise hold the terminal open with no way out but a signal.
func TestPollTokenStopsAtTheAuthorizationDeadline(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := refusedWith(pendingBody)

		auth := testAuthorization()
		auth.ExpiresIn = 32 * time.Second

		start := time.Now()

		_, err := poll(t, stub.client(), auth)
		require.ErrorIs(t, err, portal.ErrLoginExpired)

		assert.Equal(t, 32*time.Second, time.Since(start))
		assert.Len(t, stub.at, 6, "one poll every five seconds until the request ran out")
	})
}

// TestPollTokenRunsOutOfTimeBeforeItRunsOutOfAttempts pins that the attempt
// bound sits outside the deadline over a full-length login. A login nobody
// answers ends on time, not on attempts.
func TestPollTokenRunsOutOfTimeBeforeItRunsOutOfAttempts(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := refusedWith(pendingBody)

		auth := testAuthorization()
		auth.ExpiresIn = 10*time.Minute + 2*time.Second

		_, err := poll(t, stub.client(), auth)
		require.ErrorIs(t, err, portal.ErrLoginExpired)
		require.NotErrorIs(t, err, portal.ErrPollLimit)

		assert.Len(t, stub.at, 120, "one poll every five seconds until the request ran out")
	})
}

// TestPollTokenStopsWhenTheContextIsCancelledWithRacing pins that a login
// cancelled while the poller waits reports the cancellation rather than a
// request that ran out of time, which is a different thing to tell the user.
func TestPollTokenStopsWhenTheContextIsCancelledWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		stub := refusedWith(pendingBody)

		go func() {
			synctest.Sleep(12 * time.Second)
			cancel()
		}()

		_, err := portal.PollToken(ctx, logger.CreateLogger(), stub.client(), portalBaseURL, testAuthorization())
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, portal.ErrLoginExpired)

		assert.Len(t, stub.at, 2)
	})
}

// TestPollTokenReportsTheDeadlineThatAbandonedAPoll pins what the user is told
// when the authorization runs out while a poll is in flight. The transport
// reports whatever it makes of a request abandoned under it, and the reason it
// was abandoned is the login request running out of time.
func TestPollTokenReportsTheDeadlineThatAbandonedAPoll(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		c := vhttp.NewMemClient(func(ctx context.Context, _ *http.Request) (*http.Response, error) {
			<-ctx.Done()

			return nil, errors.New("connection reset by peer")
		})

		auth := testAuthorization()
		auth.ExpiresIn = 32 * time.Second

		_, err := portal.PollToken(t.Context(), logger.CreateLogger(), c, portalBaseURL, auth)
		require.ErrorIs(t, err, portal.ErrLoginExpired)
	})
}

// TestPollTokenStopsAtTheCallersDeadline pins that a deadline the caller set is
// reported as itself. A run that ran out of time is not a login request the
// user was too slow to approve, and only the second one is worth starting over.
func TestPollTokenStopsAtTheCallersDeadline(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
		defer cancel()

		stub := refusedWith(pendingBody)

		_, err := portal.PollToken(ctx, logger.CreateLogger(), stub.client(), portalBaseURL, testAuthorization())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NotErrorIs(t, err, portal.ErrLoginExpired)

		assert.Len(t, stub.at, 2)
	})
}

// TestPollTokenReportsARequestThatTimedOut pins that a timeout on one request is
// not the login request expiring. A client that gives each request a deadline of
// its own fails a slow poll with the same error the authorization's own deadline
// produces, and the portal is still holding the login open.
func TestPollTokenReportsARequestThatTimedOut(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("awaiting headers: %w", context.DeadlineExceeded)
		})

		_, err := portal.PollToken(t.Context(), logger.CreateLogger(), c, portalBaseURL, testAuthorization())
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.NotErrorIs(t, err, portal.ErrLoginExpired)
	})
}

// TestPollTokenCarriesContext pins that the caller's context reaches the
// request, so a cancelled login abandons a call already in flight.
func TestPollTokenCarriesContext(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		c := vhttp.NewMemClient(func(reqCtx context.Context, _ *http.Request) (*http.Response, error) {
			cancel()

			return nil, reqCtx.Err()
		})

		_, err := portal.PollToken(ctx, logger.CreateLogger(), c, portalBaseURL, testAuthorization())
		require.ErrorIs(t, err, context.Canceled)
		require.NotErrorIs(t, err, portal.ErrLoginExpired)
	})
}

func TestPollTokenPropagatesTransportFailure(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		sentinel := errors.New("dial failed")
		polls := 0

		c := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
			polls++

			return nil, sentinel
		})

		_, err := portal.PollToken(t.Context(), logger.CreateLogger(), c, portalBaseURL, testAuthorization())
		require.ErrorIs(t, err, sentinel)

		assert.Equal(t, pollsBeforeGivingUp, polls, "a connection that will not open is tried again first")
	})
}

// TestPollTokenRidesOutATransientFailure pins that one failed poll does not end
// a login. The user cannot restart the wait once their code is on screen, so a
// poll that failed for a reason the next one may not hit is waited out like any
// other answer.
func TestPollTokenRidesOutATransientFailure(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name   string
		body   string
		status int
	}{
		{name: "an intermediary answers in the portal's place", status: http.StatusBadGateway, body: gatewayBody},
		{name: "the portal reports a fault of its own", status: http.StatusInternalServerError, body: faultBody},
		{name: "an answer that stops partway", status: http.StatusBadRequest, body: `{"error":"authorization_p`},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				stub := &portalStub{answers: []pollResponse{
					{status: tt.status, body: tt.body},
					{status: http.StatusOK, body: issuedTokenBody},
				}}

				token, err := poll(t, stub.client(), testAuthorization())
				require.NoError(t, err)

				assert.Equal(t, "org_fake", token.Org.ID)
				assert.Len(t, stub.at, 2)
			})
		})
	}
}

// TestPollTokenGivesUpOnASustainedFailure pins that the tolerance is bounded,
// and that what reaches the caller once it runs out is what answered the poll.
func TestPollTokenGivesUpOnASustainedFailure(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		body     string
		wantCode portal.ErrorCode
		status   int
	}{
		{
			name:   "an intermediary answering in the portal's place",
			status: http.StatusBadGateway,
			body:   gatewayBody,
		},
		{
			name:     "the portal reporting a fault of its own",
			status:   http.StatusInternalServerError,
			body:     faultBody,
			wantCode: portal.ErrorCodeServerError,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				stub := answeredWith(tt.status, tt.body)

				_, err := poll(t, stub.client(), testAuthorization())

				var portalErr *portal.Error
				require.ErrorAs(t, err, &portalErr)

				assert.Equal(t, tt.wantCode, portalErr.Code)
				assert.Equal(t, tt.status, portalErr.StatusCode)
				assert.Len(t, stub.at, pollsBeforeGivingUp)
			})
		})
	}
}

// TestPollTokenDoesNotFollowARedirect pins that the device code goes to the
// configured portal and nowhere else. The stdlib resends the body of a POST on
// a 307, and the device code in that body is the whole credential of this
// exchange.
func TestPollTokenDoesNotFollowARedirect(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var hosts []string

		c := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
			hosts = append(hosts, req.URL.Host)

			header := http.Header{"Location": {"https://elsewhere.example.com/api/v1/oauth/device/token"}}

			return vhttp.Respond(http.StatusTemporaryRedirect, nil, header), nil
		})

		_, err := portal.PollToken(t.Context(), logger.CreateLogger(), c, portalBaseURL, testAuthorization())
		require.Error(t, err)

		assert.NotEmpty(t, hosts)
		assert.NotContains(t, hosts, "elsewhere.example.com")
	})
}

func TestPollTokenRejectsUnusableResponse(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name string
		body string
	}{
		{name: "not json", body: `<html>hello</html>`},
		{
			name: "wrong field type",
			body: `{"access_token":"fake-access-token","org":{"id":"org_fake"},"expires_in":"2592000"}`,
		},
		{
			name: "no lifetime",
			body: `{"access_token":"fake-access-token","org":{"id":"org_fake"}}`,
		},
		{
			name: "a lifetime too large to hold",
			body: `{"access_token":"fake-access-token","org":{"id":"org_fake"},"expires_in":2592000000000}`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				token, err := poll(t, answeredWith(http.StatusOK, tt.body).client(), testAuthorization())
				require.ErrorIs(t, err, portal.ErrMalformedResponse)
				assert.Nil(t, token)
			})
		})
	}
}

// TestPollTokenRejectsMissingField pins which field each check rejects. Matching
// only the malformed-response class cannot tell them apart, because a field left
// unchecked fails later for a different reason.
func TestPollTokenRejectsMissingField(t *testing.T) {
	t.Parallel()

	tc := []struct {
		field string
		body  string
	}{
		{field: "access_token", body: `{"org":{"id":"org_fake"},"expires_in":2592000}`},
		{field: "org.id", body: `{"access_token":"fake-access-token","expires_in":2592000}`},
	}

	for _, tt := range tc {
		t.Run(tt.field, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				_, err := poll(t, answeredWith(http.StatusOK, tt.body).client(), testAuthorization())

				var missing *portal.MissingFieldError
				require.ErrorAs(t, err, &missing)
				assert.Equal(t, tt.field, missing.Field)
				require.ErrorIs(t, err, portal.ErrMalformedResponse)
			})
		})
	}
}

func TestPollTokenRejectsNonObjectResponse(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`null`, `[1,2,3]`, `"hello"`, `42`} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				_, err := poll(t, answeredWith(http.StatusOK, body).client(), testAuthorization())
				require.ErrorIs(t, err, portal.ErrResponseNotObject)
			})
		})
	}
}

// TestPollTokenReportsRefusal pins that a refusal no further poll can change
// stops the poller at once, and reaches the caller as the portal described it
// rather than as a denial or an expiry.
func TestPollTokenReportsRefusal(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		body     string
		wantCode portal.ErrorCode
		status   int
	}{
		{
			name:     "unknown client",
			status:   http.StatusBadRequest,
			body:     `{"error":"invalid_client","error_description":"Unknown OAuth client."}`,
			wantCode: portal.ErrorCodeInvalidClient,
		},
		{
			name:     "unusable scope",
			status:   http.StatusBadRequest,
			body:     `{"error":"invalid_scope","error_description":"Unknown scope."}`,
			wantCode: portal.ErrorCodeInvalidScope,
		},
		{
			name:     "a code this package does not name",
			status:   http.StatusBadRequest,
			body:     `{"error":"account_suspended","error_description":"This account is suspended."}`,
			wantCode: portal.ErrorCode("account_suspended"),
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				stub := answeredWith(tt.status, tt.body)

				_, err := poll(t, stub.client(), testAuthorization())

				var portalErr *portal.Error
				require.ErrorAs(t, err, &portalErr)

				assert.Equal(t, tt.wantCode, portalErr.Code)
				assert.Equal(t, tt.status, portalErr.StatusCode)
				assert.Len(t, stub.at, 1)
			})
		})
	}
}

func TestPollTokenRejectsUnusableBaseURL(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		stub := answeredWith(http.StatusOK, issuedTokenBody)

		_, err := portal.PollToken(
			t.Context(),
			logger.CreateLogger(),
			stub.client(),
			"://portal.example.com",
			testAuthorization(),
		)
		require.Error(t, err)
		assert.Empty(t, stub.at, "an unusable base URL is caught before the first poll")
	})
}

// TestTokenDoesNotReachLogOutput pins what [portal.Secret] exists for on the
// issued credential: a token rendered through Terragrunt's own logger carries
// the org it reaches and not the credential. Going through the real formatter
// rather than fmt covers the formatter, which is free to render a value however
// it likes.
func TestTokenDoesNotReachLogOutput(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer

		l := logger.CreateLogger().WithOptions(log.WithOutput(&buf))

		token, err := portal.PollToken(
			t.Context(),
			l,
			answeredWith(http.StatusOK, issuedTokenBody).client(),
			portalBaseURL,
			testAuthorization(),
		)
		require.NoError(t, err)

		l.Infof("token: %v", token)
		l.Infof("token: %+v", token)

		assert.NotContains(t, buf.String(), "fake-access-token")
		assert.NotContains(t, buf.String(), "fake-device-code")
		assert.Contains(t, buf.String(), "Acme", "the org is meant to be shown")

		assert.NotContains(t, fmt.Sprintf("%#v", token), "fake-access-token")
	})
}
