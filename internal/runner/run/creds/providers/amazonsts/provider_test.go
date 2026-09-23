package amazonsts_test

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

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

func testCtx(t *testing.T) context.Context {
	t.Helper()
	return amazonsts.WithIsolatedCredentialsCache(t.Context())
}

// TestGetCredentialsReusesCacheWhenDurationUnset reproduces the --json-out-dir
// path: AssumeRoleDuration is unset (0), the getter writes the assumed session
// into v.Env, and a second GetCredentials must hit the cache instead of
// re-assuming signed with the session credentials.
func TestGetCredentialsReusesCacheWhenDurationUnset(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	roleARN := testRoleARNPrefix + t.Name()
	sts := newRecordingSTS(t, assumedAccessKeyID, time.Hour)

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

	first, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, assumedAccessKeyID, first.Envs["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, int64(1), sts.calls.Load(), "first call must hit STS")
	assert.Equal(t, "3600", sts.lastDuration.Load().(string))

	auth, ok := sts.lastAuth.Load().(string)
	require.True(t, ok, "authorization header must be a string")
	assert.Contains(t, auth, "Credential="+baseAccessKeyID+"/")

	maps.Copy(v.Env, first.Envs)

	second, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
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

	creds, err := provider.GetCredentials(testCtx(t), logger.CreateLogger(), v)
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

	creds, err := provider.GetCredentials(testCtx(t), logger.CreateLogger(), v)
	require.Error(t, err)
	assert.Nil(t, creds)
}

// TestGetCredentialsUsesExplicitDurationCachesAcrossCalls pins that a caller
// who sets AssumeRoleDuration still gets a cache hit on the second entry after
// the assumed session is copied into v.Env.
func TestGetCredentialsUsesExplicitDurationCachesAcrossCalls(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	sts := newRecordingSTS(t, "ASIAEXPLICITDURATION", 30*time.Minute)
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

	first, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "1800", sts.lastDuration.Load().(string))
	maps.Copy(v.Env, first.Envs)

	second, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, int64(1), sts.calls.Load())
	assert.Equal(t, providers.AWSCredentials, second.Name)
	assert.Equal(t, "ASIAEXPLICITDURATION", second.Envs["AWS_ACCESS_KEY_ID"])
}

// TestGetCredentialsAWSMinimumDurationStillCaches uses the AWS minimum role
// session duration (900 seconds).
func TestGetCredentialsAWSMinimumDurationStillCaches(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	sts := newRecordingSTS(t, "ASIAMINIMUMDURATION", 15*time.Minute)
	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
		RoleARN:            testRoleARNPrefix + t.Name(),
		AssumeRoleDuration: 900,
	}, v.Env)

	first, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "900", sts.lastDuration.Load().(string))
	maps.Copy(v.Env, first.Envs)

	second, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, int64(1), sts.calls.Load())
	assert.Equal(t, "ASIAMINIMUMDURATION", second.Envs["AWS_ACCESS_KEY_ID"])
}

// TestGetCredentialsNegativeDurationDefaultsAndCaches treats nonpositive
// durations like AssumeIamRole does: default one hour, then cache hit.
func TestGetCredentialsNegativeDurationDefaultsAndCaches(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	sts := newRecordingSTS(t, "ASIANEGATIVEDURATION", time.Hour)
	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
		RoleARN:            testRoleARNPrefix + t.Name(),
		AssumeRoleDuration: -1,
	}, v.Env)

	first, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "3600", sts.lastDuration.Load().(string))
	maps.Copy(v.Env, first.Envs)

	second, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, int64(1), sts.calls.Load())
}

// TestGetCredentialsIsolatesDifferentSessionNames pins that the same role ARN
// with different session names does not share a cached session.
func TestGetCredentialsIsolatesDifferentSessionNames(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	roleARN := testRoleARNPrefix + t.Name()

	for _, tc := range []struct {
		sessionName string
		assumedKey  string
	}{
		{sessionName: "unit-a", assumedKey: "ASIAA"},
		{sessionName: "unit-b", assumedKey: "ASIAB"},
	} {
		sts := newRecordingSTS(t, tc.assumedKey, time.Hour)
		v := venvtest.New().
			WithHTTP(sts.client).
			WithEnv(map[string]string{
				"AWS_REGION":            "us-east-1",
				"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
				"AWS_SECRET_ACCESS_KEY": "base-secret",
			})

		provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
			RoleARN:               roleARN,
			AssumeRoleSessionName: tc.sessionName,
		}, v.Env)

		creds, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
		require.NoError(t, err)
		require.NotNil(t, creds)
		assert.Equal(t, tc.assumedKey, creds.Envs["AWS_ACCESS_KEY_ID"])
		assert.Equal(t, int64(1), sts.calls.Load())
	}
}

// TestGetCredentialsIsolatesDifferentSourceSecrets pins that changing only the
// secret access key forces a new STS call for the same role and access key ID.
func TestGetCredentialsIsolatesDifferentSourceSecrets(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	roleARN := testRoleARNPrefix + t.Name()

	for i, secret := range []string{"secret-one", "secret-two"} {
		sts := newRecordingSTS(t, "ASIASECRET"+string(rune('A'+i)), time.Hour)
		v := venvtest.New().
			WithHTTP(sts.client).
			WithEnv(map[string]string{
				"AWS_REGION":            "us-east-1",
				"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
				"AWS_SECRET_ACCESS_KEY": secret,
			})

		provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
			RoleARN: roleARN,
		}, v.Env)

		creds, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
		require.NoError(t, err)
		require.NotNil(t, creds)
		assert.Equal(t, int64(1), sts.calls.Load())
	}
}

// TestGetCredentialsSessionIndexRequiresFullCredentialTuple pins that presenting
// only a cached access key ID with a different secret does not recover the session.
func TestGetCredentialsSessionIndexRequiresFullCredentialTuple(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	roleARN := testRoleARNPrefix + t.Name()
	sts := newRecordingSTS(t, assumedAccessKeyID, time.Hour)

	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
		RoleARN: roleARN,
	}, v.Env)

	first, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, int64(1), sts.calls.Load())

	v.Env["AWS_ACCESS_KEY_ID"] = first.Envs["AWS_ACCESS_KEY_ID"]
	v.Env["AWS_SECRET_ACCESS_KEY"] = "wrong-secret"
	v.Env["AWS_SESSION_TOKEN"] = "wrong-token"
	v.Env["AWS_SECURITY_TOKEN"] = "wrong-token"

	sts.assumedKey = "ASIASECONDSESSION"
	second, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, int64(2), sts.calls.Load(), "partial session credentials must not hit the cache")
	assert.Equal(t, "ASIASECONDSESSION", second.Envs["AWS_ACCESS_KEY_ID"])
}

// TestGetCredentialsIsolatesDifferentWebIdentityTokens pins that different
// web identity tokens for the same role do not share a cache entry.
func TestGetCredentialsIsolatesDifferentWebIdentityTokens(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	roleARN := testRoleARNPrefix + t.Name()

	for i, token := range []string{"token-a", "token-b"} {
		assumedKey := "ASIAWEB" + string(rune('A'+i))
		sts := newRecordingSTS(t, assumedKey, time.Hour)
		sts.webIdentity = true

		v := venvtest.New().
			WithHTTP(sts.client).
			WithEnv(map[string]string{
				"AWS_REGION": "us-east-1",
			})

		provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
			RoleARN:          roleARN,
			WebIdentityToken: token,
		}, v.Env)

		creds, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
		require.NoError(t, err)
		require.NotNil(t, creds)
		assert.Equal(t, assumedKey, creds.Envs["AWS_ACCESS_KEY_ID"])
		assert.Equal(t, int64(1), sts.calls.Load())
	}
}

// TestGetCredentialsRefreshAfterExpiryUsesSourceIdentity verifies that after
// the cache safety window passes, a refresh is signed with the original base
// credentials rather than the assumed session already present in v.Env.
func TestGetCredentialsRefreshAfterExpiryUsesSourceIdentity(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx := testCtx(t)
		roleARN := testRoleARNPrefix + t.Name()
		sts := newRecordingSTS(t, assumedAccessKeyID, 10*time.Minute)

		v := venvtest.New().
			WithHTTP(sts.client).
			WithEnv(map[string]string{
				"AWS_REGION":            "us-east-1",
				"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
				"AWS_SECRET_ACCESS_KEY": "base-secret",
			})

		provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
			RoleARN:            roleARN,
			AssumeRoleDuration: 900,
		}, v.Env)

		first, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
		require.NoError(t, err)
		require.NotNil(t, first)
		assert.Equal(t, int64(1), sts.calls.Load())

		maps.Copy(v.Env, first.Envs)

		time.Sleep(5*time.Minute + time.Second)

		second, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
		require.NoError(t, err)
		require.NotNil(t, second)
		assert.Equal(t, int64(2), sts.calls.Load(), "expired cache must refresh")

		auth, ok := sts.lastAuth.Load().(string)
		require.True(t, ok)
		assert.Contains(t, auth, "Credential="+baseAccessKeyID+"/",
			"refresh must be signed with the original source credentials")
	})
}

// TestGetCredentialsRefreshFailureReusesValidSession pins that a failed early
// refresh still returns the cached target session until its real expiration.
func TestGetCredentialsRefreshFailureReusesValidSession(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		ctx := testCtx(t)
		roleARN := testRoleARNPrefix + t.Name()
		sts := newRecordingSTS(t, assumedAccessKeyID, 10*time.Minute)

		v := venvtest.New().
			WithHTTP(sts.client).
			WithEnv(map[string]string{
				"AWS_REGION":            "us-east-1",
				"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
				"AWS_SECRET_ACCESS_KEY": "base-secret",
			})

		provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
			RoleARN:            roleARN,
			AssumeRoleDuration: 900,
		}, v.Env)

		first, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
		require.NoError(t, err)
		require.NotNil(t, first)
		maps.Copy(v.Env, first.Envs)

		sts.failNext.Store(true)
		time.Sleep(5*time.Minute + time.Second)

		second, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
		require.NoError(t, err)
		require.NotNil(t, second)
		assert.Equal(t, first.Envs, second.Envs)
		assert.Equal(t, int64(2), sts.calls.Load(), "refresh was attempted")
	})
}

// TestGetCredentialsCoalescesConcurrentSameKeyAssumes pins singleflight: many
// callers with the same identity produce one STS request.
func TestGetCredentialsCoalescesConcurrentSameKeyAssumes(t *testing.T) {
	t.Parallel()

	ctx := testCtx(t)
	sts := newRecordingSTS(t, "ASIACONCURRENT", time.Hour)
	sts.gate = make(chan struct{})

	v := venvtest.New().
		WithHTTP(sts.client).
		WithEnv(map[string]string{
			"AWS_REGION":            "us-east-1",
			"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
			"AWS_SECRET_ACCESS_KEY": "base-secret",
		})

	provider := amazonsts.NewProvider(logger.CreateLogger(), iam.RoleOptions{
		RoleARN: testRoleARNPrefix + t.Name(),
	}, v.Env)

	const callers = 16

	var started sync.WaitGroup
	started.Add(callers)

	results := make(chan *providers.Credentials, callers)
	errs := make(chan error, callers)

	for range callers {
		go func() {
			started.Done()
			started.Wait()

			creds, err := provider.GetCredentials(ctx, logger.CreateLogger(), v)
			if err != nil {
				errs <- err
				return
			}

			results <- creds
		}()
	}

	started.Wait()
	close(sts.gate)

	for range callers {
		select {
		case err := <-errs:
			require.NoError(t, err)
		case creds := <-results:
			require.NotNil(t, creds)
			assert.Equal(t, "ASIACONCURRENT", creds.Envs["AWS_ACCESS_KEY_ID"])
		}
	}

	assert.Equal(t, int64(1), sts.calls.Load())
}

type recordingSTS struct {
	lastAuth     atomic.Value
	lastDuration atomic.Value
	client       vhttp.Client
	gate         chan struct{}
	assumedKey   string
	auths        []string
	sessionTTL   time.Duration
	calls        atomic.Int64
	mu           sync.Mutex
	failNext     atomic.Bool
	webIdentity  bool
}

// signers returns the access key id that signed each request, in order.
func (s *recordingSTS) signers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.auths))

	for _, auth := range s.auths {
		signer := "NONE"
		if m := credentialRe.FindStringSubmatch(auth); m != nil {
			signer = m[1]
		}

		out = append(out, signer)
	}

	return out
}

func newRecordingSTS(t *testing.T, assumedKeyID string, sessionTTL time.Duration) *recordingSTS {
	t.Helper()

	sts := &recordingSTS{
		assumedKey: assumedKeyID,
		sessionTTL: sessionTTL,
	}
	sts.lastAuth.Store("")
	sts.lastDuration.Store("")

	sts.client = vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		if sts.gate != nil {
			<-sts.gate
		}

		sts.calls.Add(1)

		auth := req.Header.Get("Authorization")
		sts.lastAuth.Store(auth)

		sts.mu.Lock()
		sts.auths = append(sts.auths, auth)
		sts.mu.Unlock()

		body, err := io.ReadAll(req.Body)
		if err == nil {
			values, parseErr := url.ParseQuery(string(body))
			if parseErr == nil {
				sts.lastDuration.Store(values.Get("DurationSeconds"))
			}
		}

		if sts.failNext.Swap(false) {
			return vhttp.Respond(http.StatusForbidden, []byte("ExpiredToken"), nil), nil
		}

		expiration := time.Now().Add(sts.sessionTTL).UTC().Format(time.RFC3339)
		resultTag := "AssumeRoleResult"
		responseTag := "AssumeRoleResponse"

		if sts.webIdentity {
			resultTag = "AssumeRoleWithWebIdentityResult"
			responseTag = "AssumeRoleWithWebIdentityResponse"
		}

		xml := `<` + responseTag + ` xmlns="https://sts.amazonaws.com/doc/2011-06-15/">` +
			`<` + resultTag + `><Credentials>` +
			`<AccessKeyId>` + sts.assumedKey + `</AccessKeyId>` +
			`<SecretAccessKey>assumed-secret</SecretAccessKey>` +
			`<SessionToken>assumed-token</SessionToken>` +
			`<Expiration>` + expiration + `</Expiration>` +
			`</Credentials>` +
			`<AssumedRoleUser>` +
			`<AssumedRoleId>AROATEST:session</AssumedRoleId>` +
			`<Arn>arn:aws:iam::123456789012:role/test-role</Arn>` +
			`</AssumedRoleUser>` +
			`</` + resultTag + `></` + responseTag + `>`

		return vhttp.Respond(http.StatusOK, []byte(xml), nil), nil
	})

	return sts
}
