package amazonsts_test

import (
	"context"
	"maps"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	selfAssumeBaseKeyID   = "AKIABASEIDENTITY"
	selfAssumeMintedTuple = "ASIAMINTEDSESSION"
)

var selfAssumeCredRe = regexp.MustCompile(`Credential=([A-Z0-9]+)/`)

// The invariant: no sts:AssumeRole request may be signed by credentials this process minted.
//
// Every self-assume defect found so far violated exactly this, each on a different code shape:
// the --json-out-dir second run, the --auth-provider-cmd per-call session name, and fetches
// whose role options differ. A test pinned to any one shape cannot catch the next one, so this
// table walks the whole surface of things that change the cache key or the entry's lifetime.
func TestNoAssumeRoleIsSignedByAMintedSession(t *testing.T) {
	t.Parallel()

	baseARN := "arn:aws:iam::123456789012:role/base"

	tc := []struct {
		name   string
		first  iam.RoleOptions
		second iam.RoleOptions
	}{
		{
			name:   "same options twice",
			first:  iam.RoleOptions{RoleARN: baseARN},
			second: iam.RoleOptions{RoleARN: baseARN},
		},
		{
			name:   "different role arns",
			first:  iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/dependency"},
			second: iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/unit"},
		},
		{
			name:   "same arn different duration",
			first:  iam.RoleOptions{RoleARN: baseARN, AssumeRoleDuration: 3600},
			second: iam.RoleOptions{RoleARN: baseARN, AssumeRoleDuration: 1800},
		},
		{
			name:   "same arn different session name",
			first:  iam.RoleOptions{RoleARN: baseARN, AssumeRoleSessionName: "first"},
			second: iam.RoleOptions{RoleARN: baseARN, AssumeRoleSessionName: "second"},
		},
		{
			name:   "session name set then unset",
			first:  iam.RoleOptions{RoleARN: baseARN, AssumeRoleSessionName: "pinned"},
			second: iam.RoleOptions{RoleARN: baseARN},
		},
		{
			name:   "duration set then unset",
			first:  iam.RoleOptions{RoleARN: baseARN, AssumeRoleDuration: 900},
			second: iam.RoleOptions{RoleARN: baseARN},
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sts := newSelfAssumeSTS()
			v := newSelfAssumeVenv(sts)
			ctx := amazonsts.WithIsolatedCredentialsCache(t.Context())
			l := logger.CreateLogger()

			first, err := amazonsts.NewProvider(l, tt.first, v.Env).GetCredentials(ctx, l, v)
			require.NoError(t, err)
			require.NotNil(t, first)

			// The credential getter writes the assumed session into the shared env; mirror that.
			maps.Copy(v.Env, first.Envs)

			_, err = amazonsts.NewProvider(l, tt.second, v.Env).GetCredentials(ctx, l, v)
			require.NoError(t, err)

			assertNoMintedSigner(t, sts)
		})
	}
}

// Pins the invariant when a third fetch follows, since a key can be stable across two calls only.
func TestNoAssumeRoleIsSignedByAMintedSessionAcrossThreeFetches(t *testing.T) {
	t.Parallel()

	sts := newSelfAssumeSTS()
	v := newSelfAssumeVenv(sts)
	ctx := amazonsts.WithIsolatedCredentialsCache(t.Context())
	l := logger.CreateLogger()

	opts := iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/repeat"}

	for range 3 {
		creds, err := amazonsts.NewProvider(l, opts, v.Env).GetCredentials(ctx, l, v)
		require.NoError(t, err)
		require.NotNil(t, creds)

		maps.Copy(v.Env, creds.Envs)
	}

	assertNoMintedSigner(t, sts)
}

// Pins the invariant when concurrent fetches race the cache, as they do under run --all.
func TestNoAssumeRoleIsSignedByAMintedSessionUnderConcurrency(t *testing.T) {
	t.Parallel()

	sts := newSelfAssumeSTS()
	v := newSelfAssumeVenv(sts)
	ctx := amazonsts.WithIsolatedCredentialsCache(t.Context())
	l := logger.CreateLogger()

	seed, err := amazonsts.NewProvider(l, iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/seed"}, v.Env).
		GetCredentials(ctx, l, v)
	require.NoError(t, err)
	maps.Copy(v.Env, seed.Envs)

	var wg sync.WaitGroup

	for i := range 8 {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			opts := iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/concurrent"}
			if i%2 == 0 {
				opts.AssumeRoleDuration = 1800
			}

			_, _ = amazonsts.NewProvider(l, opts, v.Env).GetCredentials(ctx, l, v)
		}(i)
	}

	wg.Wait()
	assertNoMintedSigner(t, sts)
}

// Pins that a minted session never becomes the signing identity even when the env holds only it.
func TestNoAssumeRoleIsSignedByAMintedSessionWhenEnvHasOnlySession(t *testing.T) {
	t.Parallel()

	sts := newSelfAssumeSTS()
	v := newSelfAssumeVenv(sts)
	ctx := amazonsts.WithIsolatedCredentialsCache(t.Context())
	l := logger.CreateLogger()

	opts := iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/only-session"}

	first, err := amazonsts.NewProvider(l, opts, v.Env).GetCredentials(ctx, l, v)
	require.NoError(t, err)
	maps.Copy(v.Env, first.Envs)

	second, err := amazonsts.NewProvider(l, opts, v.Env).GetCredentials(ctx, l, v)
	require.NoError(t, err)
	assert.Equal(t, providers.AWSCredentials, second.Name)
	assertNoMintedSigner(t, sts)
}

func assertNoMintedSigner(t *testing.T, sts *selfAssumeSTS) {
	t.Helper()

	for i, signer := range sts.signers() {
		assert.Equalf(t, selfAssumeBaseKeyID, signer,
			"sts:AssumeRole call %d was signed by %q. Only the caller's own identity (%q) may sign an "+
				"assume-role request; a session this process minted must never become the signing identity, "+
				"because AWS rejects a role assuming itself with AccessDenied.",
			i+1, signer, selfAssumeBaseKeyID)
	}
}

func newSelfAssumeVenv(sts *selfAssumeSTS) *venv.Venv {
	return venvtest.New().WithHTTP(sts.client).WithEnv(map[string]string{
		"AWS_REGION":            "us-east-1",
		"AWS_ACCESS_KEY_ID":     selfAssumeBaseKeyID,
		"AWS_SECRET_ACCESS_KEY": "base-secret",
	})
}

type selfAssumeSTS struct {
	client vhttp.Client
	seen   []string
	mu     sync.Mutex
	minted atomic.Int64
}

func (s *selfAssumeSTS) signers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.seen...)
}

func newSelfAssumeSTS() *selfAssumeSTS {
	sts := &selfAssumeSTS{}

	sts.client = vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		signer := "NONE"
		if m := selfAssumeCredRe.FindStringSubmatch(req.Header.Get("Authorization")); m != nil {
			signer = m[1]
		}

		sts.mu.Lock()
		sts.seen = append(sts.seen, signer)
		sts.mu.Unlock()

		// Each response mints a distinct session so a reused one is identifiable in the signer list.
		keyID := selfAssumeMintedTuple + strings.ToUpper(string(rune('A'+int(sts.minted.Add(1)-1)%26)))

		body := `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">` +
			`<AssumeRoleResult><Credentials>` +
			`<AccessKeyId>` + keyID + `</AccessKeyId>` +
			`<SecretAccessKey>minted-secret</SecretAccessKey>` +
			`<SessionToken>minted-token</SessionToken>` +
			`<Expiration>2030-12-31T23:59:59Z</Expiration>` +
			`</Credentials><AssumedRoleUser>` +
			`<AssumedRoleId>AROATEST:session</AssumedRoleId>` +
			`<Arn>arn:aws:sts::123456789012:assumed-role/x/session</Arn>` +
			`</AssumedRoleUser></AssumeRoleResult></AssumeRoleResponse>`

		return vhttp.Respond(http.StatusOK, []byte(body), nil), nil
	})

	return sts
}
