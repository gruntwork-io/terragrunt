//go:build ssh

// We don't want contributors to have to install SSH keys to run these tests, so we skip
// them by default. Contributors need to opt in to run these tests by setting the
// build flag `ssh` when running the tests. This is done by adding the `-tags ssh` flag
// to the `go test` command. For example:
//
// go test -tags ssh ./...

package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testScaffoldModuleGit                 = "git@github.com:gruntwork-io/terragrunt.git//test/fixtures/scaffold/scaffold-module"
	testScaffoldTemplateModule            = "git@github.com:gruntwork-io/terragrunt.git//test/fixtures/scaffold/module-with-template"
	testScaffoldExternalTemplateModule    = "git@github.com:gruntwork-io/terragrunt.git//test/fixtures/scaffold/external-template/template"
	testScaffoldWithCustomDefaultTemplate = "fixtures/scaffold/custom-default-template"
)

func TestSSHScaffoldWithCustomDefaultTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testScaffoldWithCustomDefaultTemplate)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testScaffoldWithCustomDefaultTemplate)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt --non-interactive --working-dir %s scaffold %s",
		filepath.Join(testPath, "unit"),
		testScaffoldModuleURL,
	))

	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")

	assert.FileExists(t, filepath.Join(testPath, "unit", "terragrunt.hcl"))
	assert.FileExists(t, filepath.Join(testPath, "unit", "external-template.txt"))
}

func TestSSHScaffoldModuleExternalTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s %s", tmpEnvPath, testScaffoldModuleGit, testScaffoldExternalTemplateModule))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	// check that exists file from external template
	assert.FileExists(t, tmpEnvPath+"/external-template.txt")
	assert.FileExists(t, tmpEnvPath+"/dependency/dependency.txt")
}

func TestSSHScaffoldModuleDifferentRevisionAndSSH(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s --var=Ref=v0.67.4 --var=SourceUrlType=git-ssh", tmpEnvPath, testScaffoldModuleShort))
	require.NoError(t, err)
	assert.Contains(t, stderr, "git::ssh://git@github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.67.4")
	assert.Contains(t, stderr, "Scaffolding completed")
}

func TestSSHScaffoldModuleSSH(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s", tmpEnvPath, testScaffoldModuleGit))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
}

func TestSSHScaffoldModuleTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s", tmpEnvPath, testScaffoldTemplateModule))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	// check that exists file from .boilerplate dir
	assert.FileExists(t, tmpEnvPath+"/template-file.txt")
}

func TestSSHScaffoldModuleVarFile(t *testing.T) {
	t.Parallel()
	// generate var file with specific version, without root include and use GIT/SSH to clone module.
	varFileContent := `
Ref: v0.67.4
EnableRootInclude: false
SourceUrlType: "git-ssh"
`
	varFile := filepath.Join(t.TempDir(), "var-file.yaml")
	err := os.WriteFile(varFile, []byte(varFileContent), 0644)
	require.NoError(t, err)

	tmpEnvPath := t.TempDir()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s --var-file=%s", tmpEnvPath, testScaffoldModuleShort, varFile))
	require.NoError(t, err)
	assert.Contains(t, stderr, "git::ssh://git@github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.67.4")
	assert.Contains(t, stderr, "Scaffolding completed")
	content, err := util.ReadFileAsString(tmpEnvPath + "/terragrunt.hcl")
	require.NoError(t, err)
	assert.NotContains(t, content, "find_in_parent_folders")
}
