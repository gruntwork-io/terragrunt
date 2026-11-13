//go:build azure && tofu

package test_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureTofuStateEncryptionAzureVault(t *testing.T) {
	t.Parallel()

	if os.Getenv("TG_AZURE_TEST_VAULT_URI") == "" || os.Getenv("TG_AZURE_TEST_VAULT_KEY_NAME") == "" {
		t.Skip("TG_AZURE_TEST_VAULT_URI and TG_AZURE_TEST_VAULT_KEY_NAME are required for live test")
	}

	const fixture = "fixtures/tofu-state-encryption/azure-vault"

	tmpEnvPath := helpers.CopyEnvironment(t, fixture)
	workDir := filepath.Join(tmpEnvPath, fixture)

	helpers.RunTerragrunt(
		t,
		"terragrunt run --non-interactive --working-dir "+workDir+" -- apply -auto-approve",
	)
	validateStateIsEncrypted(t, stateFile, workDir)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt run --non-interactive --working-dir "+workDir+" -- output -raw encryption_test",
	)
	require.NoError(t, err)
	assert.Equal(t, "azure-vault-encryption", strings.TrimSpace(stdout))
}
