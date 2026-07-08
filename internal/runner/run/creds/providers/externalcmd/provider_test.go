package externalcmd_test

import (
	"os/exec"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/runner/run/creds/providers/externalcmd"
	"github.com/gruntwork-io/terragrunt/internal/shell"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"

	"github.com/stretchr/testify/require"
)

// TestGetCredentialsHandlesJSONNullResponse pins safe handling of stdout containing the JSON literal `null`.
func TestGetCredentialsHandlesJSONNullResponse(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("printf"); err != nil {
		t.Skipf("printf not found on PATH: %v", err)
	}

	l := logger.CreateLogger()
	opts := shell.NewShellOptions()

	provider := externalcmd.NewProvider(l, "printf null", opts)

	creds, err := provider.GetCredentials(t.Context(), l, venv.OSVenv())

	require.NoError(t, err)
	require.NotNil(t, creds)
	require.NotNil(t, creds.Envs)
}
