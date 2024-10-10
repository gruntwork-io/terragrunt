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
	testScaffoldModuleURL              = "https://github.com/gruntwork-io/terragrunt.git//test/fixtures/scaffold/scaffold-module"
	testScaffoldModuleGit              = "git@github.com:gruntwork-io/terragrunt.git//test/fixtures/scaffold/scaffold-module"
	testScaffoldModuleShort            = "github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs"
	testScaffoldTemplateModule         = "git@github.com:gruntwork-io/terragrunt.git//test/fixtures/scaffold/module-with-template"
	testScaffoldExternalTemplateModule = "git@github.com:gruntwork-io/terragrunt.git//test/fixtures/scaffold/external-template"
	testScaffoldLocalModulePath        = "fixtures/scaffold/scaffold-module"
	testScaffold3rdPartyModulePath     = "git::https://github.com/Azure/terraform-azurerm-avm-res-compute-virtualmachine.git//.?ref=v0.15.0"
)

func TestScaffoldModule(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, testScaffoldModuleURL))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	assert.FileExists(t, tmpEnvPath+"/terragrunt.hcl")
}

func TestScaffoldModuleShortUrl(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, testScaffoldModuleShort))

	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	// check that find_in_parent_folders is generated in terragrunt.hcl
	content, err := util.ReadFileAsString(tmpEnvPath + "/terragrunt.hcl")
	require.NoError(t, err)
	assert.Contains(t, content, "find_in_parent_folders")
}

func TestScaffoldModuleShortUrlNoRootInclude(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s --var=EnableRootInclude=false", tmpEnvPath, testScaffoldModuleShort))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	// check that find_in_parent_folders is NOT generated in  terragrunt.hcl
	content, err := util.ReadFileAsString(tmpEnvPath + "/terragrunt.hcl")
	require.NoError(t, err)
	assert.NotContains(t, content, "find_in_parent_folders")
}

func TestScaffoldModuleDifferentRevision(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s --var=Ref=v0.67.4", tmpEnvPath, testScaffoldModuleShort))

	require.NoError(t, err)
	assert.Contains(t, stderr, "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.67.4")
	assert.Contains(t, stderr, "Scaffolding completed")
}

func TestScaffoldModuleDifferentRevisionAndSsh(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s --var=Ref=v0.67.4 --var=SourceUrlType=git-ssh", tmpEnvPath, testScaffoldModuleShort))
	require.NoError(t, err)
	assert.Contains(t, stderr, "git::ssh://git@github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.67.4")
	assert.Contains(t, stderr, "Scaffolding completed")
}

func TestScaffoldModuleSsh(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, testScaffoldModuleGit))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
}

func TestScaffoldModuleTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, testScaffoldTemplateModule))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	// check that exists file from .boilerplate dir
	assert.FileExists(t, tmpEnvPath+"/template-file.txt")
}

func TestScaffoldModuleExternalTemplate(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s %s", tmpEnvPath, testScaffoldModuleGit, testScaffoldExternalTemplateModule))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	// check that exists file from external template
	assert.FileExists(t, tmpEnvPath+"/external-template.txt")
}

func TestScaffoldErrorNoModuleUrl(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold", tmpEnvPath))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No module URL passed")
}

func TestScaffoldModuleVarFile(t *testing.T) {
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

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s --var-file=%s", tmpEnvPath, testScaffoldModuleShort, varFile))
	require.NoError(t, err)
	assert.Contains(t, stderr, "git::ssh://git@github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.67.4")
	assert.Contains(t, stderr, "Scaffolding completed")
	content, err := util.ReadFileAsString(tmpEnvPath + "/terragrunt.hcl")
	require.NoError(t, err)
	assert.NotContains(t, content, "find_in_parent_folders")
}

func TestScaffoldLocalModule(t *testing.T) {
	t.Parallel()

	tmpEnvPath, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)

	workingDir, err := os.Getwd()
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, fmt.Sprintf("%s//%s", workingDir, testScaffoldLocalModulePath)))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	assert.FileExists(t, tmpEnvPath+"/terragrunt.hcl")
}

func TestScaffold3rdPartyModule(t *testing.T) {
	t.Parallel()

	tmpRoot, err := os.MkdirTemp("", "terragrunt-scaffold-test")
	require.NoError(t, err)
	tmpEnvPath := filepath.Join(tmpRoot, "app")
	err = os.MkdirAll(tmpEnvPath, 0755)
	require.NoError(t, err)

	// create "root" terragrunt.hcl
	err = os.WriteFile(filepath.Join(tmpRoot, "terragrunt.hcl"), []byte(""), 0644)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s scaffold %s", tmpEnvPath, testScaffold3rdPartyModulePath))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	assert.FileExists(t, tmpEnvPath+"/terragrunt.hcl")

	// validate the generated files
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --terragrunt-non-interactive --terragrunt-working-dir %s hclvalidate", tmpEnvPath))
	require.NoError(t, err)
}
