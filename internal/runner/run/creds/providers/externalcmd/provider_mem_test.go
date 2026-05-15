package externalcmd_test

import (
	"context"
	"io"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/writer"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProviderEmptyAuthProviderCmdIsNoop pins the contract that an unset
// auth-provider command short-circuits without dispatching anything to
// vexec. The previous OS implementation would have constructed
// vexec.NewOSExec() unconditionally; the in-memory backend lets the test
// assert zero invocations.
func TestProviderEmptyAuthProviderCmdIsNoop(t *testing.T) {
	t.Parallel()

	var calls int

	v := newMemVenv(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		calls++
		return vexec.Result{}
	})

	p := externalcmd.NewProvider(logger.CreateLogger(), "", newRunOpts())

	creds, err := p.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	assert.Nil(t, creds)
	assert.Zero(t, calls, "expected no subprocess invocations for an empty auth-provider command")
}

// TestProviderDirectAWSCredentials covers the awsCredentials branch of the
// auth-provider response schema: returned env vars are surfaced verbatim
// and mapped to all four AWS_* destinations.
func TestProviderDirectAWSCredentials(t *testing.T) {
	t.Parallel()

	v := newMemVenv(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "/usr/local/bin/auth", inv.Name)
		assert.Equal(t, []string{"--account", "prod"}, inv.Args)

		return vexec.Result{Stdout: []byte(`{
            "awsCredentials": {
                "ACCESS_KEY_ID": "AKIA111",
                "SECRET_ACCESS_KEY": "secret-xyz",
                "SESSION_TOKEN": "session-abc"
            }
        }`)}
	})

	p := externalcmd.NewProvider(logger.CreateLogger(), "/usr/local/bin/auth --account prod", newRunOpts())

	creds, err := p.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, providers.AWSCredentials, creds.Name)
	assert.Equal(t, "AKIA111", creds.Envs["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "secret-xyz", creds.Envs["AWS_SECRET_ACCESS_KEY"])
	assert.Equal(t, "session-abc", creds.Envs["AWS_SESSION_TOKEN"])
	assert.Equal(t, "session-abc", creds.Envs["AWS_SECURITY_TOKEN"], "AWS_SECURITY_TOKEN must mirror AWS_SESSION_TOKEN")
}

// TestProviderArbitraryEnvs covers the envs-only branch: arbitrary
// environment variables on the response are surfaced without any AWS
// specific mapping.
func TestProviderArbitraryEnvs(t *testing.T) {
	t.Parallel()

	v := newMemVenv(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte(`{"envs": {"FOO": "bar", "BAZ": "qux"}}`)}
	})

	p := externalcmd.NewProvider(logger.CreateLogger(), "auth-cmd", newRunOpts())

	creds, err := p.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, "bar", creds.Envs["FOO"])
	assert.Equal(t, "qux", creds.Envs["BAZ"])
}

func TestProviderEmptyResponseErrors(t *testing.T) {
	t.Parallel()

	v := newMemVenv(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("")}
	})

	p := externalcmd.NewProvider(logger.CreateLogger(), "auth-cmd", newRunOpts())

	_, err := p.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain JSON")
}

func TestProviderInvalidJSONErrors(t *testing.T) {
	t.Parallel()

	v := newMemVenv(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{Stdout: []byte("not json at all")}
	})

	p := externalcmd.NewProvider(logger.CreateLogger(), "auth-cmd", newRunOpts())

	_, err := p.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid JSON")
}

func TestProviderCommandFailurePropagates(t *testing.T) {
	t.Parallel()

	v := newMemVenv(func(_ context.Context, _ vexec.Invocation) vexec.Result {
		return vexec.Result{ExitCode: 2, Stderr: []byte("permission denied\n")}
	})

	p := externalcmd.NewProvider(logger.CreateLogger(), "auth-cmd", newRunOpts())

	_, err := p.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.Error(t, err)
}

// TestProviderCommandShellwordsParsing pins that quoted arguments survive
// the shellwords parse the provider applies before dispatch.
func TestProviderCommandShellwordsParsing(t *testing.T) {
	t.Parallel()

	v := newMemVenv(func(_ context.Context, inv vexec.Invocation) vexec.Result {
		assert.Equal(t, "auth", inv.Name)
		assert.Equal(t, []string{"--profile", "with space", "--region", "us-east-1"}, inv.Args)

		return vexec.Result{Stdout: []byte(`{"envs": {}}`)}
	})

	p := externalcmd.NewProvider(logger.CreateLogger(), `auth --profile "with space" --region us-east-1`, newRunOpts())

	_, err := p.GetCredentials(t.Context(), logger.CreateLogger(), v)
	require.NoError(t, err)
}

func newRunOpts() *shell.ShellOptions {
	return shell.NewShellOptions()
}

func newMemVenv(h vexec.Handler) *venv.Venv {
	return &venv.Venv{
		Exec:    vexec.NewMemExec(h),
		Env:     map[string]string{},
		Writers: &writer.Writers{Writer: io.Discard, ErrWriter: io.Discard},
	}
}
