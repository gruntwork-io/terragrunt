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

	"github.com/stretchr/testify/require"
)

const mintedAccessKeyID = "ASIAMINTEDSESSION"

var credentialRe = regexp.MustCompile(`Credential=([A-Z0-9]+)/`)

// TestNoAssumeRoleIsSignedByAMintedSession checks differing role options still sign with the caller's key.
func TestNoAssumeRoleIsSignedByAMintedSession(t *testing.T) {
	t.Parallel()

	baseARN := "arn:aws:iam::123456789012:role/base"

	tcs := []struct {
		name   string
		first  iam.RoleOptions
		second iam.RoleOptions
	}{
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

			sts := newRecordingSTS(t, mintedAccessKeyID, time.Hour)
			v := newSelfAssumeVenv(sts)
			ctx := testCtx(t)
			l := logger.CreateLogger()

			first, err := amazonsts.NewProvider(l, tc.first, v.Env).GetCredentials(ctx, l, v)
			require.NoError(t, err)
			require.NotNil(t, first)

			maps.Copy(v.Env, first.Envs)

			_, err = amazonsts.NewProvider(l, tc.second, v.Env).GetCredentials(ctx, l, v)
			require.NoError(t, err)

			assertNoMintedSigner(t, sts)
		})
	}
}

// TestNoAssumeRoleIsSignedByAMintedSessionAcrossThreeFetches covers a third fetch.
func TestNoAssumeRoleIsSignedByAMintedSessionAcrossThreeFetches(t *testing.T) {
	t.Parallel()

	sts := newRecordingSTS(t, mintedAccessKeyID, time.Hour)
	v := newSelfAssumeVenv(sts)
	ctx := testCtx(t)
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

// TestNoAssumeRoleIsSignedByAMintedSessionUnderConcurrency covers fetches racing the cache.
func TestNoAssumeRoleIsSignedByAMintedSessionUnderConcurrency(t *testing.T) {
	t.Parallel()

	sts := newRecordingSTS(t, mintedAccessKeyID, time.Hour)
	v := newSelfAssumeVenv(sts)
	ctx := testCtx(t)
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

	for i := range concurrent {
		require.NoErrorf(t, errs[i], "fetch %d failed", i)
		require.NotNilf(t, got[i], "fetch %d returned no credentials", i)
	}

	assertNoMintedSigner(t, sts)
}

// assertNoMintedSigner fails if any STS call was signed by a session this process minted.
func assertNoMintedSigner(t *testing.T, sts *recordingSTS) {
	t.Helper()

	signers := sts.signers()
	require.NotEmpty(t, signers, "no STS call to inspect")

	for i, signer := range signers {
		require.Equalf(t, baseAccessKeyID, signer,
			"STS call %d was signed by %q, not the caller's key", i+1, signer)
	}
}

// newSelfAssumeVenv returns a venv holding the caller's own credentials.
func newSelfAssumeVenv(sts *recordingSTS) *venv.Venv {
	return venvtest.New().WithHTTP(sts.client).WithEnv(map[string]string{
		"AWS_REGION":            "us-east-1",
		"AWS_ACCESS_KEY_ID":     baseAccessKeyID,
		"AWS_SECRET_ACCESS_KEY": "base-secret",
	})
}
