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
	testScaffoldModuleURL          = "https://github.com/gruntwork-io/terragrunt.git//test/fixtures/scaffold/scaffold-module"
	testScaffoldModuleShort        = "github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs"
	testScaffoldLocalModulePath    = "fixtures/scaffold/scaffold-module"
	testScaffoldWithRootHCL        = "fixtures/scaffold/root-hcl"
	testScaffold3rdPartyModulePath = "git::https://github.com/Azure/terraform-azurerm-avm-res-compute-virtualmachine.git//.?ref=v0.15.0"
)

func TestScaffoldModule(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s", tmpEnvPath, testScaffoldModuleURL))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	assert.FileExists(t, tmpEnvPath+"/terragrunt.hcl")
}

func TestScaffoldModuleShortUrl(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s", tmpEnvPath, testScaffoldModuleShort))

	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	// check that find_in_parent_folders is generated in terragrunt.hcl
	content, err := util.ReadFileAsString(tmpEnvPath + "/terragrunt.hcl")
	require.NoError(t, err)
	assert.Contains(t, content, "find_in_parent_folders")
}

func TestScaffoldModuleShortUrlNoRootInclude(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s --var=EnableRootInclude=false", tmpEnvPath, testScaffoldModuleShort))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	// check that find_in_parent_folders is NOT generated in  terragrunt.hcl
	content, err := util.ReadFileAsString(tmpEnvPath + "/terragrunt.hcl")
	require.NoError(t, err)
	assert.NotContains(t, content, "find_in_parent_folders")
}

func TestScaffoldModuleDifferentRevision(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s --var=Ref=v0.67.4", tmpEnvPath, testScaffoldModuleShort))

	require.NoError(t, err)
	assert.Contains(t, stderr, "git::https://github.com/gruntwork-io/terragrunt.git//test/fixtures/inputs?ref=v0.67.4")
	assert.Contains(t, stderr, "Scaffolding completed")
}

func TestScaffoldErrorNoModuleUrl(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt scaffold --non-interactive --working-dir "+tmpEnvPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No module URL passed")
}

func TestScaffoldLocalModule(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	workingDir, err := os.Getwd()
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s", tmpEnvPath, fmt.Sprintf("%s//%s", workingDir, testScaffoldLocalModulePath)))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	assert.FileExists(t, tmpEnvPath+"/terragrunt.hcl")
}

func TestScaffold3rdPartyModule(t *testing.T) {
	t.Parallel()

	tmpRoot := t.TempDir()

	tmpEnvPath := filepath.Join(tmpRoot, "app")
	err := os.MkdirAll(tmpEnvPath, 0755)
	require.NoError(t, err)

	// create "root" terragrunt.hcl
	err = os.WriteFile(filepath.Join(tmpRoot, "terragrunt.hcl"), []byte(""), 0644)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt scaffold --non-interactive --working-dir %s %s", tmpEnvPath, testScaffold3rdPartyModulePath))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	assert.FileExists(t, tmpEnvPath+"/terragrunt.hcl")

	// validate the generated files
	_, _, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt hclvalidate --non-interactive --working-dir "+tmpEnvPath)
	require.NoError(t, err)
}

func TestScaffoldOutputFolderFlag(t *testing.T) {
	t.Parallel()

	tmpEnvPath := t.TempDir()

	outputFolder := tmpEnvPath + "/foo/bar"
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt --non-interactive --working-dir %s scaffold %s --output-folder %s", tmpEnvPath, testScaffoldModuleURL, outputFolder))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")
	assert.FileExists(t, outputFolder+"/terragrunt.hcl")
}

func TestScaffoldWithRootHCL(t *testing.T) {
	t.Parallel()

	tmpEnvPath := helpers.CopyEnvironment(t, testScaffoldWithRootHCL)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, testScaffoldWithRootHCL)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt --non-interactive --working-dir %s scaffold %s",
		filepath.Join(testPath, "unit"),
		testScaffoldModuleURL,
	))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Scaffolding completed")

	assert.FileExists(t, filepath.Join(testPath, "unit", "terragrunt.hcl"))

	// Read the file
	content, err := util.ReadFileAsString(filepath.Join(testPath, "unit", "terragrunt.hcl"))
	require.NoError(t, err)
	assert.Contains(t, content, `path = find_in_parent_folders("root.hcl")`)
}
