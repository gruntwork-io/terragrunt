package test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	TEST_SCAFOLD_MODULE     = "https://github.com/gruntwork-io/terragrunt.git//test/fixture-scaffold/scaffold-module?ref=feature/scaffold"
	TEST_SCAFOLD_MODULE_GIT = "git@github.com:gruntwork-io/terragrunt.git//test/fixture-scaffold/scaffold-module?ref=feature/scaffold"
)

func TestTerragruntScaffoldModule(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()
	err := os.MkdirAll(tmpEnvPath, 0755)
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, TEST_SCAFOLD_MODULE), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
}

func TestTerragruntScaffoldModuleGit(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()
	err := os.MkdirAll(tmpEnvPath, 0755)
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, TEST_SCAFOLD_MODULE_GIT), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
}
