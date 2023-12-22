package test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"

	"github.com/stretchr/testify/require"
)

const (
	TEST_SCAFOLD_MODULE                   = "https://github.com/gruntwork-io/terragrunt.git//test/fixture-scaffold/scaffold-module"
	TEST_SCAFOLD_MODULE_GIT               = "git@github.com:gruntwork-io/terragrunt.git//test/fixture-scaffold/scaffold-module"
	TEST_SCAFOLD_MODULE_SHORT             = "github.com/gruntwork-io/terragrunt.git//test/fixture-inputs"
	TEST_SCAFOLD_TEMPLATE_MODULE          = "git@github.com:gruntwork-io/terragrunt.git//test/fixture-scaffold/module-with-template"
	TEST_SCAFOLD_EXTERNAL_TEMPLATE_MODULE = "git@github.com:gruntwork-io/terragrunt.git//test/fixture-scaffold/external-template"
)

func TestTerragruntScaffoldModule(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, TEST_SCAFOLD_MODULE), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
	require.FileExists(t, fmt.Sprintf("%s/terragrunt.hcl", tmpEnvPath))
}

func TestTerragruntScaffoldModuleShortUrl(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, TEST_SCAFOLD_MODULE_SHORT), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
	// check that find_in_parent_folders is generated in terragrunt.hcl
	content, err := util.ReadFileAsString(fmt.Sprintf("%s/terragrunt.hcl", tmpEnvPath))
	require.NoError(t, err)
	require.Contains(t, content, "find_in_parent_folders")
}

func TestTerragruntScaffoldModuleShortUrlNoRootInclude(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s --var=EnableRootInclude=false", tmpEnvPath, TEST_SCAFOLD_MODULE_SHORT), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
	// check that find_in_parent_folders is NOT generated in  terragrunt.hcl
	content, err := util.ReadFileAsString(fmt.Sprintf("%s/terragrunt.hcl", tmpEnvPath))
	require.NoError(t, err)
	require.NotContains(t, content, "find_in_parent_folders")
}

func TestTerragruntScaffoldModuleDifferentRevision(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s --var=Ref=v0.53.1", tmpEnvPath, TEST_SCAFOLD_MODULE_SHORT), &stdout, &stderr)

	require.NoError(t, err)
	require.Contains(t, stderr.String(), "git::https://github.com/gruntwork-io/terragrunt.git//test/fixture-inputs?ref=v0.53.1")
	require.Contains(t, stderr.String(), "Scaffolding completed")
}

func TestTerragruntScaffoldModuleDifferentRevisionAndSsh(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s --var=Ref=v0.53.1 --var=SourceUrlType=git-ssh", tmpEnvPath, TEST_SCAFOLD_MODULE_SHORT), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "git::ssh://git@github.com/gruntwork-io/terragrunt.git//test/fixture-inputs?ref=v0.53.1")
	require.Contains(t, stderr.String(), "Scaffolding completed")
}

func TestTerragruntScaffoldModuleSsh(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, TEST_SCAFOLD_MODULE_GIT), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
}

func TestTerragruntScaffoldModuleTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
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

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s %s", tmpEnvPath, TEST_SCAFOLD_MODULE_GIT, TEST_SCAFOLD_EXTERNAL_TEMPLATE_MODULE), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "Scaffolding completed")
	// check that exists file from external template
	require.FileExists(t, fmt.Sprintf("%s/external-template.txt", tmpEnvPath))
}

func TestTerragruntScaffoldErrorNoModuleUrl(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold", tmpEnvPath), &stdout, &stderr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "No module URL passed")
}

func TestTerragruntScaffoldModuleVarFile(t *testing.T) {
	t.Parallel()
	// generate var file with specific version, without root include and use GIT/SSH to clone module.
	varFileContent := `
Ref: v0.53.1
EnableRootInclude: false
SourceUrlType: "git-ssh"
`
	varFile := filepath.Join(t.TempDir(), "var-file.yaml")
	err := os.WriteFile(varFile, []byte(varFileContent), 0644)
	require.NoError(t, err)

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	stdout := bytes.Buffer{}
	stderr := bytes.Buffer{}

	err = runTerragruntCommand(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s --var-file=%s", tmpEnvPath, TEST_SCAFOLD_MODULE_SHORT, varFile), &stdout, &stderr)
	require.NoError(t, err)
	require.Contains(t, stderr.String(), "git::ssh://git@github.com/gruntwork-io/terragrunt.git//test/fixture-inputs?ref=v0.53.1")
	require.Contains(t, stderr.String(), "Scaffolding completed")
	content, err := util.ReadFileAsString(fmt.Sprintf("%s/terragrunt.hcl", tmpEnvPath))
	require.NoError(t, err)
	require.NotContains(t, content, "find_in_parent_folders")
}
