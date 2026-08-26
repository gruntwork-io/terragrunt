package creds_test

import (
	"context"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errProviderUnavailable = errors.New("auth provider unavailable")

type stubProvider struct {
	creds *providers.Credentials
	err   error
	name  string
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) GetCredentials(
	_ context.Context,
	_ log.Logger,
	_ *venv.Venv,
) (*providers.Credentials, error) {
	return p.creds, p.err
}

// TestObtainAndUpdateEnvLaterProviderWins pins the ordering the S3 backend relies on: when an
// auth-provider command supplies source credentials and the amazonsts provider then assumes a
// role, the role session must be what ends up in v.Env. Backend operations read their
// credentials from there and skip re-assuming the role, so the session has to win.
func TestObtainAndUpdateEnvLaterProviderWins(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New()

	authProvider := &stubProvider{
		name: "external command",
		creds: &providers.Credentials{
			Name: providers.AWSCredentials,
			Envs: map[string]string{
				"AWS_ACCESS_KEY_ID":     "source-access-key",
				"AWS_SECRET_ACCESS_KEY": "source-secret-key",
			},
		},
	}
	stsProvider := &stubProvider{
		name: "API calls to Amazon STS",
		creds: &providers.Credentials{
			Name: providers.AWSCredentials,
			Envs: map[string]string{
				"AWS_ACCESS_KEY_ID":     "role-session-access-key",
				"AWS_SECRET_ACCESS_KEY": "role-session-secret-key",
				"AWS_SESSION_TOKEN":     "role-session-token",
			},
		},
	}

	err := creds.NewGetter().ObtainAndUpdateEnvIfNecessary(t.Context(), l, v, authProvider, stsProvider)
	require.NoError(t, err)

	assert.Equal(t, "role-session-access-key", v.Env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "role-session-secret-key", v.Env["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "role-session-token", v.Env["AWS_SESSION_TOKEN"])
}

// TestObtainAndUpdateEnvNilCredsLeaveEnvUntouched pins that a provider with nothing to
// contribute (e.g. amazonsts without a configured role) neither clobbers credentials written by
// an earlier provider nor touches unrelated env entries.
func TestObtainAndUpdateEnvNilCredsLeaveEnvUntouched(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New()
	v.Env["UNRELATED"] = "kept"

	authProvider := &stubProvider{
		name: "external command",
		creds: &providers.Credentials{
			Name: providers.AWSCredentials,
			Envs: map[string]string{
				"AWS_ACCESS_KEY_ID":     "source-access-key",
				"AWS_SECRET_ACCESS_KEY": "source-secret-key",
			},
		},
	}
	noopProvider := &stubProvider{
		name: "API calls to Amazon STS",
	}

	err := creds.NewGetter().ObtainAndUpdateEnvIfNecessary(t.Context(), l, v, authProvider, noopProvider)
	require.NoError(t, err)

	assert.Equal(t, "source-access-key", v.Env["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "source-secret-key", v.Env["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "kept", v.Env["UNRELATED"])
}

// TestObtainAndUpdateEnvPropagatesProviderFailure pins that the chain fails closed: the first
// provider error is returned as-is and no later provider runs, so units never run
// OpenTofu/Terraform with half-populated credentials.
func TestObtainAndUpdateEnvPropagatesProviderFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New()

	failingProvider := &stubProvider{
		name: "external command",
		err:  errProviderUnavailable,
	}
	stsProvider := &stubProvider{
		name: "API calls to Amazon STS",
		creds: &providers.Credentials{
			Name: providers.AWSCredentials,
			Envs: map[string]string{
				"AWS_ACCESS_KEY_ID": "role-session-access-key",
				"AWS_SESSION_TOKEN": "role-session-token",
			},
		},
	}

	err := creds.NewGetter().ObtainAndUpdateEnvIfNecessary(t.Context(), l, v, failingProvider, stsProvider)
	require.ErrorIs(t, err, errProviderUnavailable)
	assert.Empty(t, v.Env, "a provider failure must abort the chain before any later provider runs")
}

// TestObtainCredsForParsingWithoutAuthProviderCmd pins the path every parse without
// auth_provider_cmd takes: no subprocess is dispatched and v.Env is left as the caller built it.
func TestObtainCredsForParsingWithoutAuthProviderCmd(t *testing.T) {
	t.Parallel()

	var calls int

	l := logger.CreateLogger()
	v := venvtest.New().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls++

		return vexec.Result{}
	})
	v.Env["UNRELATED"] = "kept"

	getter, err := creds.ObtainCredsForParsing(t.Context(), l, v, "", shell.NewShellOptions(map[string]string{}))
	require.NoError(t, err)
	assert.NotNil(t, getter)
	assert.Zero(t, calls, "expected no subprocess invocations for an empty auth-provider command")
	assert.Equal(t, map[string]string{"UNRELATED": "kept"}, v.Env)
}

// TestObtainCredsForParsingPopulatesEnvBeforeParsing pins that auth-provider credentials reach
// v.Env before HCL parsing, which is what makes sops_decrypt_file() and get_aws_account_id() work
// inside locals.
func TestObtainCredsForParsingPopulatesEnvBeforeParsing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		wantEnv map[string]string
		name    string
		stdout  string
	}{
		{
			name:   "arbitrary envs",
			stdout: `{"envs": {"SOPS_AGE_KEY": "age-key", "FOO": "bar"}}`,
			wantEnv: map[string]string{
				"SOPS_AGE_KEY": "age-key",
				"FOO":          "bar",
			},
		},
		{
			name: "aws credentials",
			stdout: `{
                "awsCredentials": {
                    "ACCESS_KEY_ID": "AKIA111",
                    "SECRET_ACCESS_KEY": "secret-xyz",
                    "SESSION_TOKEN": "session-abc"
                }
            }`,
			wantEnv: map[string]string{
				"AWS_ACCESS_KEY_ID":     "AKIA111",
				"AWS_SECRET_ACCESS_KEY": "secret-xyz",
				"AWS_SESSION_TOKEN":     "session-abc",
				"AWS_SECURITY_TOKEN":    "session-abc",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				assert.Equal(t, "auth-cmd", inv.Name)

				return vexec.Result{Stdout: []byte(tc.stdout)}
			})

			getter, err := creds.ObtainCredsForParsing(t.Context(), l, v, "auth-cmd", shell.NewShellOptions(map[string]string{}))
			require.NoError(t, err)
			assert.NotNil(t, getter)
			assert.Equal(t, tc.wantEnv, v.Env)
		})
	}
}

// TestObtainCredsForParsingReportsCommandFailure pins that a failing auth-provider command aborts
// parsing with the process execution error rather than continuing with an unauthenticated env.
func TestObtainCredsForParsingReportsCommandFailure(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithHandler(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 2, Stderr: []byte("permission denied\n")}
	})
	v.Env["UNRELATED"] = "kept"

	getter, err := creds.ObtainCredsForParsing(t.Context(), l, v, "auth-cmd", shell.NewShellOptions(map[string]string{}))
	require.Error(t, err)
	assert.Nil(t, getter)

	var procErr util.ProcessExecutionError

	require.ErrorAs(t, err, &procErr)
	assert.Equal(t, map[string]string{"UNRELATED": "kept"}, v.Env)
}
