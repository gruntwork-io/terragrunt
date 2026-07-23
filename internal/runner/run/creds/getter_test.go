package creds_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds"
	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubProvider struct {
	creds *providers.Credentials
	name  string
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) GetCredentials(
	_ context.Context,
	_ log.Logger,
	_ venv.Venv,
) (*providers.Credentials, error) {
	return p.creds, nil
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
