package amazonsts_test

import (
	"maps"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/amazonsts"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	selfAssumeBaseKeyID   = "AKIABASEIDENTITY"
	selfAssumeMintedKeyID = "ASIAMINTEDSESSION"
)

// credentialRe extracts the access key id from a SigV4 Authorization header.
var credentialRe = regexp.MustCompile(`Credential=([A-Z0-9]+)/`)

// Pins the invariant that no sts:AssumeRole request may be signed by credentials this process minted.
func TestNoAssumeRoleIsSignedByAMintedSession(t *testing.T) {
	t.Parallel()

	baseARN := "arn:aws:iam::123456789012:role/base"

	tcs := []struct {
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

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sts := newRecordingSTS(t, selfAssumeMintedKeyID, time.Hour)
			v := newSelfAssumeVenv(sts)
			ctx := amazonsts.WithIsolatedCredentialsCache(t.Context())
			l := logger.CreateLogger()

			first, err := amazonsts.NewProvider(l, tc.first, v.Env).GetCredentials(ctx, l, v)
			require.NoError(t, err)
			require.NotNil(t, first)

			// The credential getter writes the assumed session into the shared env; mirror that.
			maps.Copy(v.Env, first.Envs)

			_, err = amazonsts.NewProvider(l, tc.second, v.Env).GetCredentials(ctx, l, v)
			require.NoError(t, err)

			assertNoMintedSigner(t, sts)
		})
	}
}

// Pins the invariant when a third fetch follows, since a key can be stable across two calls only.
func TestNoAssumeRoleIsSignedByAMintedSessionAcrossThreeFetches(t *testing.T) {
	t.Parallel()

	sts := newRecordingSTS(t, selfAssumeMintedKeyID, time.Hour)
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

	sts := newRecordingSTS(t, selfAssumeMintedKeyID, time.Hour)
	v := newSelfAssumeVenv(sts)
	ctx := amazonsts.WithIsolatedCredentialsCache(t.Context())
	l := logger.CreateLogger()

	seed, err := amazonsts.NewProvider(l, iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/seed"}, v.Env).
		GetCredentials(ctx, l, v)
	require.NoError(t, err)
	maps.Copy(v.Env, seed.Envs)

	const concurrent = 8

	var wg sync.WaitGroup

	errs := make([]error, concurrent)
	got := make([]*providers.Credentials, concurrent)

	for i := range concurrent {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			opts := iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/concurrent"}
			if i%2 == 0 {
				opts.AssumeRoleDuration = 1800
			}

			got[i], errs[i] = amazonsts.NewProvider(l, opts, v.Env).GetCredentials(ctx, l, v)
		}(i)
	}

	wg.Wait()

	// Without this the test passes vacuously when every concurrent fetch fails before reaching STS.
	for i := range concurrent {
		require.NoErrorf(t, errs[i], "concurrent fetch %d failed", i)
		require.NotNilf(t, got[i], "concurrent fetch %d returned no credentials", i)
	}

	assertNoMintedSigner(t, sts)
}

// Pins that a minted session never becomes the signing identity even when the env holds only it.
func TestNoAssumeRoleIsSignedByAMintedSessionWhenEnvHasOnlySession(t *testing.T) {
	t.Parallel()

	sts := newRecordingSTS(t, selfAssumeMintedKeyID, time.Hour)
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

func assertNoMintedSigner(t *testing.T, sts *recordingSTS) {
	t.Helper()

	signers := sts.signers()
	// An empty list would satisfy the loop below, so a run that never reached STS must fail here.
	require.NotEmpty(t, signers, "expected at least one sts:AssumeRole call to inspect")

	for i, signer := range signers {
		assert.Equalf(t, selfAssumeBaseKeyID, signer,
			"sts:AssumeRole call %d was signed by %q. Only the caller's own identity (%q) may sign an "+
				"assume-role request; a session this process minted must never become the signing identity, "+
				"because AWS rejects a role assuming itself with AccessDenied.",
			i+1, signer, selfAssumeBaseKeyID)
	}
}

func newSelfAssumeVenv(sts *recordingSTS) *venv.Venv {
	return venvtest.New().WithHTTP(sts.client).WithEnv(map[string]string{
		"AWS_REGION":            "us-east-1",
		"AWS_ACCESS_KEY_ID":     selfAssumeBaseKeyID,
		"AWS_SECRET_ACCESS_KEY": "base-secret",
	})
}
