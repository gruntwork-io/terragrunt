package externalcmd_test

import (
	"context"
	"maps"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	awsRoleBaseAccessKeyID    = "AKIABASEIDENTITY"
	awsRoleAssumedAccessKeyID = "ASIAASSUMEDSESSION"
	awsRoleARN                = "arn:aws:iam::123456789012:role/auth-provider-test"
)

// Pins that repeated auth-provider fetches assume the role once and keep signing STS with the caller's own identity.
func TestProviderAWSRoleReusesAssumedSessionAcrossFetches(t *testing.T) {
	t.Parallel()

	sts := newRecordingSTS(t)

	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     awsRoleBaseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		}).
		WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			return vexec.Result{Stdout: []byte(`{"awsRole": {"roleARN": "` + awsRoleARN + `"}}`)}
		})

	ctx := amazonsts.WithIsolatedCredentialsCache(t.Context())
	l := logger.CreateLogger()
	p := externalcmd.NewProvider(l, "auth-cmd", newRunOpts())

	first, err := p.GetCredentials(ctx, l, v)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, awsRoleAssumedAccessKeyID, first.Envs["AWS_ACCESS_KEY_ID"])
	require.Equal(t, int64(1), sts.calls.Load(), "first fetch must assume the role")
	assert.Contains(t, sts.auth(), "Credential="+awsRoleBaseAccessKeyID+"/",
		"the first assumption must be signed by the caller's own identity")

	// Mirror creds.Getter, which writes the assumed session into the shared env.
	maps.Copy(v.Env, first.Envs)

	second, err := p.GetCredentials(ctx, l, v)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, first.Envs, second.Envs)
	assert.Equal(t, int64(1), sts.calls.Load(),
		"the second fetch must reuse the cached session rather than re-assuming")

	// A third fetch guards against a key that is stable only between two calls.
	third, err := p.GetCredentials(ctx, l, v)
	require.NoError(t, err)
	require.NotNil(t, third)
	assert.Equal(t, int64(1), sts.calls.Load())
	assert.NotContains(t, sts.auth(), "Credential="+awsRoleAssumedAccessKeyID+"/",
		"no STS call may be signed by the session a previous assumption produced")
}

// Pins that an explicit roleSessionName from the auth-provider response still reaches STS.
func TestProviderAWSRoleHonoursExplicitSessionName(t *testing.T) {
	t.Parallel()

	sts := newRecordingSTS(t)

	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     awsRoleBaseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		}).
		WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
			return vexec.Result{Stdout: []byte(`{"awsRole": {` +
				`"roleARN": "` + awsRoleARN + `", "roleSessionName": "pinned-session"}}`)}
		})

	ctx := amazonsts.WithIsolatedCredentialsCache(t.Context())
	l := logger.CreateLogger()
	p := externalcmd.NewProvider(l, "auth-cmd", newRunOpts())

	creds, err := p.GetCredentials(ctx, l, v)
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "pinned-session", sts.sessionName())
}

type recordingSTS struct {
	lastAuth    atomic.Value
	lastSession atomic.Value
	client      vhttp.Client
	calls       atomic.Int64
}

func (s *recordingSTS) auth() string {
	got, _ := s.lastAuth.Load().(string)
	return got
}

func (s *recordingSTS) sessionName() string {
	got, _ := s.lastSession.Load().(string)
	return got
}

func newRecordingSTS(t *testing.T) *recordingSTS {
	t.Helper()

	sts := &recordingSTS{}
	sts.lastAuth.Store("")
	sts.lastSession.Store("")

	body := `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">` +
		`<AssumeRoleResult><Credentials>` +
		`<AccessKeyId>` + awsRoleAssumedAccessKeyID + `</AccessKeyId>` +
		`<SecretAccessKey>assumed-secret</SecretAccessKey>` +
		`<SessionToken>assumed-token</SessionToken>` +
		`<Expiration>2030-12-31T23:59:59Z</Expiration>` +
		`</Credentials><AssumedRoleUser>` +
		`<AssumedRoleId>AROATEST:session</AssumedRoleId>` +
		`<Arn>` + awsRoleARN + `</Arn>` +
		`</AssumedRoleUser></AssumeRoleResult></AssumeRoleResponse>`

	sts.client = vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		sts.calls.Add(1)
		sts.lastAuth.Store(req.Header.Get("Authorization"))

		if err := req.ParseForm(); err == nil {
			sts.lastSession.Store(req.FormValue("RoleSessionName"))
		}

		return vhttp.Respond(http.StatusOK, []byte(body), nil), nil
	})

	return sts
}
