package test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	// TODO: change ref values to master once the feature is merged
	TEST_SCAFOLD_MODULE       = "https://github.com/gruntwork-io/terragrunt.git//test/fixture-scaffold/scaffold-module?ref=feature/scaffold"
	TEST_SCAFOLD_MODULE_GIT   = "git@github.com:gruntwork-io/terragrunt.git//test/fixture-scaffold/scaffold-module?ref=feature/scaffold"
	TEST_SCAFOLD_MODULE_SHORT = "github.com/gruntwork-io/terragrunt.git//test/fixture-inputs?ref=master"

	TEST_SCAFOLD_TEMPLATE_MODULE          = "git@github.com:gruntwork-io/terragrunt.git//test/fixture-scaffold/module-with-template?ref=feature/scaffold"
	TEST_SCAFOLD_EXTERNAL_TEMPLATE_MODULE = "git@github.com:gruntwork-io/terragrunt.git//test/fixture-scaffold/external-template?ref=feature/scaffold"
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

func TestTerragruntScaffoldModuleShortUrl(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()
	err := os.MkdirAll(tmpEnvPath, 0755)
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, TEST_SCAFOLD_MODULE_SHORT), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
}

func TestTerragruntScaffoldModuleSsh(t *testing.T) {
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

func TestTerragruntScaffoldModuleTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()
	err := os.MkdirAll(tmpEnvPath, 0755)
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, TEST_SCAFOLD_TEMPLATE_MODULE), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
	// check that exists file from .boilerplate dir
	require.FileExists(t, fmt.Sprintf("%s/template-file.txt", tmpEnvPath))
}

func TestTerragruntScaffoldModuleExternalTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()
	err := os.MkdirAll(tmpEnvPath, 0755)
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s %s", tmpEnvPath, TEST_SCAFOLD_MODULE_GIT, TEST_SCAFOLD_EXTERNAL_TEMPLATE_MODULE), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
	// check that exists file from external template
	require.FileExists(t, fmt.Sprintf("%s/external-template.txt", tmpEnvPath))
}
