package login_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/login"
	"github.com/gruntwork-io/terragrunt/internal/portal"
	"github.com/gruntwork-io/terragrunt/internal/vbrowser"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	portalBaseURL    = "https://portal.example.com"
	authorizePath    = "/api/v1/oauth/device/authorize"
	tokenPath        = "/api/v1/oauth/device/token"
	userCode         = "FAKE-CODE"
	approvalURL      = portalBaseURL + "/auth/device?user_code=FAKE-CODE"
	accountEmail     = "you@example.com"
	organizationID   = "org_fake"
	organizationName = "Acme"
)

const authorizationBody = `{
	"device_code": "fake-device-code",
	"user_code": "FAKE-CODE",
	"verification_uri": "https://portal.example.com/auth/device",
	"verification_uri_complete": "https://portal.example.com/auth/device?user_code=FAKE-CODE",
	"expires_in": 600,
	"interval": 5
}`

const issuedTokenBody = `{
	"access_token": "fake-access-token",
	"token_type": "Bearer",
	"expires_in": 2592000,
	"scope": "catalog:read",
	"org": {"id": "org_fake", "name": "Acme"},
	"account": {"email": "you@example.com"}
}`

const reissuedTokenBody = `{
	"access_token": "fresh-access-token",
	"token_type": "Bearer",
	"expires_in": 2592000,
	"scope": "catalog:read",
	"org": {"id": "org_fake", "name": "Acme"},
	"account": {"email": "you@example.com"}
}`

// tokenLifetime is the thirty days the portal issues, so a credential a test
// saves is unexpired for as long as that test runs.
const tokenLifetime = 30 * 24 * time.Hour

const deniedBody = `{"error":"access_denied","error_description":"The user declined."}`

// errUnexpectedRequest ends a stub portal's answer to a request no test staged,
// so a login that reaches somewhere unforeseen fails rather than hangs.
var errUnexpectedRequest = errors.New("unexpected request")

// answer is one response a stub portal gives.
type answer struct {
	body   string
	status int
}

// portalAnswering builds a client answering each path with the response staged
// for it.
func portalAnswering(t *testing.T, answers map[string]answer) vhttp.Client {
	t.Helper()

	return vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		staged, ok := answers[req.URL.Path]
		if !ok {
			t.Errorf("the login reached %s, which no answer was staged for", req.URL.Path)

			return nil, errUnexpectedRequest
		}

		header := http.Header{"Content-Type": {"application/json"}}

		return vhttp.Respond(staged.status, []byte(staged.body), header), nil
	})
}

// approvingPortal answers the login that ends with the portal issuing token.
func approvingPortal(t *testing.T, token string) vhttp.Client {
	t.Helper()

	return portalAnswering(t, map[string]answer{
		authorizePath: {status: http.StatusOK, body: authorizationBody},
		tokenPath:     {status: http.StatusOK, body: token},
	})
}

// recordingBrowser reports the URL a login sent the user to.
func recordingBrowser(opened *string) vbrowser.Opener {
	return vbrowser.NewMemOpener(func(_ context.Context, rawURL string) error {
		*opened = rawURL

		return nil
	})
}

func newOptions(baseURL string) *login.Options {
	opts := login.NewOptions(options.NewTerragruntOptions(vexec.NewOSExec()))
	opts.BaseURL = baseURL

	return opts
}

func run(t *testing.T, v *venv.Venv) error {
	t.Helper()

	return login.Run(t.Context(), logger.CreateLogger(), v, newOptions(portalBaseURL))
}

func TestRunSignsInAndKeepsTheCredential(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var (
			out    bytes.Buffer
			opened string
		)

		v := venvtest.New().
			WithHTTP(approvingPortal(t, issuedTokenBody)).
			WithBrowser(recordingBrowser(&opened)).
			WithWriter(&out)

		require.NoError(t, run(t, v))

		assert.Equal(t, approvalURL, opened)
		assert.Contains(t, out.String(), userCode)
		assert.Contains(t, out.String(), "Signed in as "+accountEmail+" — "+organizationName)

		tokens, err := portal.LoadTokens(logger.CreateLogger(), v, portalBaseURL)
		require.NoError(t, err)
		require.Len(t, tokens, 1)
		assert.Equal(t, "fake-access-token", tokens[organizationID].AccessToken.Reveal())
	})
}

// TestRunDoesNotPrintTheCredential pins that the token the portal issued stays
// out of the terminal, where the one-time code the user has to read does not.
func TestRunDoesNotPrintTheCredential(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var (
			out    bytes.Buffer
			opened string
		)

		v := venvtest.New().
			WithHTTP(approvingPortal(t, issuedTokenBody)).
			WithBrowser(recordingBrowser(&opened)).
			WithWriter(&out)

		require.NoError(t, run(t, v))

		assert.NotContains(t, out.String(), "fake-access-token")
		assert.NotContains(t, out.String(), "fake-device-code")
	})
}

// TestRunNamesTheOrganizationWithoutAnAccount pins that a portal sending no
// account email still logs the user in, with a line naming what it reached.
func TestRunNamesTheOrganizationWithoutAnAccount(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var out bytes.Buffer

		body := `{"access_token":"fake-access-token","token_type":"Bearer","expires_in":2592000,` +
			`"org":{"id":"org_fake","name":"Acme"}}`

		v := venvtest.New().
			WithHTTP(approvingPortal(t, body)).
			WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error { return nil })).
			WithWriter(&out)

		require.NoError(t, run(t, v))

		assert.Contains(t, out.String(), "Signed in to "+organizationName)
	})
}

// TestRunLeavesAnUnexpiredLoginAlone pins that a second login changes nothing.
// The HTTP client refuses every request and the browser refuses to open, so a
// run that reached either one would fail rather than quietly repeat the login.
func TestRunLeavesAnUnexpiredLoginAlone(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	v := venvtest.New().
		WithHTTP(vhttp.NewNoNetworkClient()).
		WithBrowser(vbrowser.NewNoBrowserOpener()).
		WithWriter(&out)

	require.NoError(t, portal.SaveToken(logger.CreateLogger(), v, portalBaseURL, &portal.Token{
		AccessToken: portal.Secret("fake-access-token"),
		TokenType:   "Bearer",
		Scope:       portal.ScopeCatalogRead,
		Org:         portal.Org{ID: organizationID, Name: organizationName},
		Account:     portal.Account{Email: accountEmail},
		ExpiresIn:   tokenLifetime,
	}))

	require.NoError(t, run(t, v))

	assert.Contains(t, out.String(), "Already signed in as "+accountEmail+" — "+organizationName)
	assert.Contains(t, out.String(), "--"+login.ForceFlagName)
	assert.NotContains(t, out.String(), userCode)
}

// TestRunReportsEveryCurrentLoginInOrder pins the order a user signed in to more
// than one organization is shown, which map iteration would otherwise vary
// between runs, and the id a line falls back to when the portal named no
// organization.
func TestRunReportsEveryCurrentLoginInOrder(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	v := venvtest.New().
		WithHTTP(vhttp.NewNoNetworkClient()).
		WithBrowser(vbrowser.NewNoBrowserOpener()).
		WithWriter(&out)

	saved := []struct {
		org     portal.Org
		account portal.Account
	}{
		{org: portal.Org{ID: "org_delta", Name: "Zenith"}, account: portal.Account{Email: "z@example.com"}},
		{org: portal.Org{ID: "org_beta", Name: organizationName}, account: portal.Account{Email: "b@example.com"}},
		{org: portal.Org{ID: "org_zulu"}},
		{org: portal.Org{ID: "org_alpha", Name: organizationName}, account: portal.Account{Email: "a@example.com"}},
	}

	for _, entry := range saved {
		require.NoError(t, portal.SaveToken(logger.CreateLogger(), v, portalBaseURL, &portal.Token{
			AccessToken: portal.Secret("fake-access-token"),
			TokenType:   "Bearer",
			Scope:       portal.ScopeCatalogRead,
			ExpiresIn:   tokenLifetime,
			Org:         entry.org,
			Account:     entry.account,
		}))
	}

	require.NoError(t, run(t, v))

	assert.Equal(t, []string{
		"Already signed in to org_zulu",
		"Already signed in as a@example.com — " + organizationName,
		"Already signed in as b@example.com — " + organizationName,
		"Already signed in as z@example.com — Zenith",
		"Run `" + login.Command(newOptions(portalBaseURL).Experiments) + " --" + login.ForceFlagName + "` to sign in again.",
	}, strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n"))
}

// TestRunForceReplacesAnUnexpiredCredential pins the way back for a user whose
// credential stopped working before it ran out. The portal revokes nothing and
// the CLI has no logout, so the forced run has to reach the portal and put what
// it issues over the entry already on disk.
func TestRunForceReplacesAnUnexpiredCredential(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var out bytes.Buffer

		v := venvtest.New().
			WithHTTP(approvingPortal(t, reissuedTokenBody)).
			WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error { return nil })).
			WithWriter(&out)

		require.NoError(t, portal.SaveToken(logger.CreateLogger(), v, portalBaseURL, &portal.Token{
			AccessToken: portal.Secret("stale-access-token"),
			TokenType:   "Bearer",
			Scope:       portal.ScopeCatalogRead,
			Org:         portal.Org{ID: organizationID, Name: organizationName},
			Account:     portal.Account{Email: accountEmail},
			ExpiresIn:   tokenLifetime,
		}))

		opts := newOptions(portalBaseURL)
		opts.Force = true

		require.NoError(t, login.Run(t.Context(), logger.CreateLogger(), v, opts))

		assert.Contains(t, out.String(), "Signed in as "+accountEmail+" — "+organizationName)

		tokens, err := portal.LoadTokens(logger.CreateLogger(), v, portalBaseURL)
		require.NoError(t, err)
		require.Len(t, tokens, 1)
		assert.Equal(t, "fresh-access-token", tokens[organizationID].AccessToken.Reveal())
	})
}

// TestRunCompletesWithoutABrowser pins the headless path: a host that cannot
// open a window still gets the code and the URL, and the login finishes once
// the user approves it from somewhere else.
func TestRunCompletesWithoutABrowser(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var out bytes.Buffer

		v := venvtest.New().
			WithHTTP(approvingPortal(t, issuedTokenBody)).
			WithWriter(&out)

		require.NoError(t, run(t, v))

		assert.Contains(t, out.String(), userCode)
		assert.Contains(t, out.String(), approvalURL)
		assert.Contains(t, out.String(), "Signed in as "+accountEmail+" — "+organizationName)
	})
}

// TestRunReportsAPortalThatIsNotAcceptingLogins pins that a portal with the
// feature switched off ends the run with something the user can act on, rather
// than a raw refusal code or a wait for an approval that cannot arrive.
func TestRunReportsAPortalThatIsNotAcceptingLogins(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name   string
		body   string
		status int
	}{
		{
			name:   "feature_not_enabled",
			status: http.StatusForbidden,
			body: `{"error":"feature_not_enabled",` +
				`"message":"The Terragrunt catalog feature is not enabled for this organization."}`,
		},
		{
			name:   "invalid_client",
			status: http.StatusBadRequest,
			body:   `{"error":"invalid_client","error_description":"Device login is off."}`,
		},
		{
			name:   "invalid_scope",
			status: http.StatusBadRequest,
			body:   `{"error":"invalid_scope","error_description":"catalog:read is not permitted."}`,
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				var out bytes.Buffer

				c := portalAnswering(t, map[string]answer{
					authorizePath: {status: tt.status, body: tt.body},
				})

				v := venvtest.New().
					WithHTTP(c).
					WithBrowser(vbrowser.NewNoBrowserOpener()).
					WithWriter(&out)

				err := run(t, v)

				require.ErrorIs(t, err, login.ErrLoginUnavailable)

				var portalErr *portal.Error

				require.ErrorAs(t, err, &portalErr)
				assert.Empty(t, out.String())
			})
		})
	}
}

// TestRunPassesOnAnUnrecognizedRefusal pins that only a refusal login can
// explain is reworded. A fault the portal reports may well clear on a retry,
// and calling it a switched-off feature would send the user to an
// administrator over nothing.
func TestRunPassesOnAnUnrecognizedRefusal(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		c := portalAnswering(t, map[string]answer{
			authorizePath: {
				status: http.StatusInternalServerError,
				body:   `{"error":"server_error","error_description":"Internal server error."}`,
			},
		})

		v := venvtest.New().WithHTTP(c).WithBrowser(vbrowser.NewNoBrowserOpener())

		err := run(t, v)

		require.Error(t, err)
		require.NotErrorIs(t, err, login.ErrLoginUnavailable)

		var portalErr *portal.Error

		require.ErrorAs(t, err, &portalErr)
		assert.Equal(t, portal.ErrorCodeServerError, portalErr.Code)
	})
}

// TestRunKeepsNothingWhenTheUserDeclines pins that a refused login leaves no
// credential behind for a later command to find.
func TestRunKeepsNothingWhenTheUserDeclines(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		c := portalAnswering(t, map[string]answer{
			authorizePath: {status: http.StatusOK, body: authorizationBody},
			tokenPath:     {status: http.StatusBadRequest, body: deniedBody},
		})

		v := venvtest.New().
			WithHTTP(c).
			WithBrowser(vbrowser.NewMemOpener(func(context.Context, string) error { return nil }))

		err := run(t, v)
		require.ErrorIs(t, err, portal.ErrLoginDenied)

		tokens, err := portal.LoadTokens(logger.CreateLogger(), v, portalBaseURL)
		require.NoError(t, err)
		assert.Empty(t, tokens)
	})
}
