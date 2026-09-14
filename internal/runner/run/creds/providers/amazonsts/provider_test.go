package amazonsts_test

import (
	"context"
	"maps"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	baseAccessKeyID    = "AKIABASEKEYFORCACHE"
	assumedAccessKeyID = "ASIAASSUMEDSESSION"
	testRoleARNPrefix  = "arn:aws:iam::123456789012:role/cache-test-"
)

// TestGetCredentialsReusesCacheWhenDurationUnset reproduces the --json-out-dir
// path: AssumeRoleDuration is unset (0), the getter writes the assumed session
// into v.Env, and a second GetCredentials must hit the cache instead of
// re-assuming signed with the session credentials.
func TestGetCredentialsReusesCacheWhenDurationUnset(t *testing.T) {
	t.Parallel()

	roleARN := testRoleARNPrefix + t.Name()
	sts := newRecordingSTS(t, assumedAccessKeyID)

	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
		RoleARN:            roleARN,
		AssumeRoleDuration: 0,
	}, v.Env)

	first, err := provider.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, assumedAccessKeyID, first.Envs["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, int64(1), sts.calls.Load(), "first call must hit STS")

	auth, ok := sts.lastAuth.Load().(string)
	require.True(t, ok, "authorization header must be a string")
	assert.Contains(t, auth, "Credential="+baseAccessKeyID+"/")

	// Simulate creds.Getter writing the assumed session into the shared env.
	maps.Copy(v.Env, first.Envs)

	second, err := provider.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, first.Envs, second.Envs)
	assert.Equal(t, int64(1), sts.calls.Load(),
		"second call must reuse the cache; a miss would re-assume signed with the session")
}

// TestGetCredentialsEmptyRoleARNIsNoop pins that an unset role skips STS.
func TestGetCredentialsEmptyRoleARNIsNoop(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	memHTTP := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		calls.Add(1)
		return vhttp.Respond(http.StatusInternalServerError, []byte("unexpected"), nil), nil
	})

	v := venvtest.New().WithHTTP(memHTTP).WithEnv(map[string]string{
		"AWS_REGION":            "us-east-1",
		"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
		"AWS_SECRET_ACCESS_KEY": "base-secret",
	})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{}, v.Env)

	creds, err := provider.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	assert.Nil(t, creds)
	assert.Zero(t, calls.Load())
}

// TestGetCredentialsPropagatesSTSFailure pins that a non-OK STS response fails
// closed and does not write credentials into the caller.
func TestGetCredentialsPropagatesSTSFailure(t *testing.T) {
	t.Parallel()

	memHTTP := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		return vhttp.Respond(http.StatusForbidden, []byte("AccessDenied"), nil), nil
	})

	v := venvtest.New().WithHTTP(memHTTP).WithEnv(map[string]string{
		"AWS_REGION":            "us-east-1",
		"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
		"AWS_SECRET_ACCESS_KEY": "base-secret",
	})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
		RoleARN: testRoleARNPrefix + t.Name(),
	}, v.Env)

	creds, err := provider.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.Error(t, err)
	assert.Nil(t, creds)
}

// TestGetCredentialsUsesExplicitDurationCachesAcrossCalls pins that a caller
// who sets AssumeRoleDuration still gets a cache hit on the second entry after
// the assumed session is copied into v.Env.
func TestGetCredentialsUsesExplicitDurationCachesAcrossCalls(t *testing.T) {
	t.Parallel()

	sts := newRecordingSTS(t, "ASIAEXPLICITDURATION")
	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
		RoleARN:            testRoleARNPrefix + t.Name(),
		AssumeRoleDuration: 1800,
	}, v.Env)

	first, err := provider.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, first)
	maps.Copy(v.Env, first.Envs)

	second, err := provider.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, int64(1), sts.calls.Load())
	assert.Equal(t, providers.AWSCredentials, second.Name)
	assert.Equal(t, "ASIAEXPLICITDURATION", second.Envs["AWS_ACCESS_KEY_ID"])
}

// TestGetCredentialsShortDurationStillCaches covers durations shorter than the
// cache expiry window: the TTL clamps to the full session length so a second
// entry still hits.
func TestGetCredentialsShortDurationStillCaches(t *testing.T) {
	t.Parallel()

	sts := newRecordingSTS(t, "ASIASHORTDURATION")
	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
		RoleARN:            testRoleARNPrefix + t.Name(),
		AssumeRoleDuration: 60,
	}, v.Env)

	first, err := provider.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, first)
	maps.Copy(v.Env, first.Envs)

	second, err := provider.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, int64(1), sts.calls.Load())
	assert.Equal(t, "ASIASHORTDURATION", second.Envs["AWS_ACCESS_KEY_ID"])
}

type recordingSTS struct {
	lastAuth atomic.Value
	client   vhttp.Client
	calls    atomic.Int64
}

func newRecordingSTS(t *testing.T, assumedKeyID string) *recordingSTS {
	t.Helper()

	sts := &recordingSTS{}
	sts.lastAuth.Store("")

	xml := `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">` +
		`<AssumeRoleResult><Credentials>` +
		`<AccessKeyId>` + assumedKeyID + `</AccessKeyId>` +
		`<SecretAccessKey>assumed-secret</SecretAccessKey>` +
		`<SessionToken>assumed-token</SessionToken>` +
		`<Expiration>2030-12-31T23:59:59Z</Expiration>` +
		`</Credentials>` +
		`<AssumedRoleUser>` +
		`<AssumedRoleId>AROATEST:session</AssumedRoleId>` +
		`<Arn>arn:aws:iam::123456789012:role/test-role</Arn>` +
		`</AssumedRoleUser>` +
		`</AssumeRoleResult></AssumeRoleResponse>`

	sts.client = vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		sts.calls.Add(1)
		sts.lastAuth.Store(req.Header.Get("Authorization"))

		return vhttp.Respond(http.StatusOK, []byte(xml), nil), nil
	})

	return sts
}
