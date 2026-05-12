//nolint:paralleltest,tparallel // Every test in this file calls RequireSSH, which uses t.Setenv and therefore can't run in parallel.
package test_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testScaffoldWithCustomDefaultTemplate = "fixtures/scaffold/custom-default-template"
)

// sshScaffoldModuleURL is the SSH-form source for scaffold-module
// against the local mirror.
func sshScaffoldModuleURL(m *helpers.TerragruntMirror) string {
	return "git::" + m.SSHURL + "//test/fixtures/scaffold/scaffold-module"
}

// sshScaffoldTemplateModule is the SSH-form source for the
// scaffold module-with-template fixture against the local mirror.
func sshScaffoldTemplateModule(m *helpers.TerragruntMirror) string {
	return "git::" + m.SSHURL + "//test/fixtures/scaffold/module-with-template"
}

// sshScaffoldExternalTemplateModule is the SSH-form source for the
// scaffold external-template/template fixture against the local mirror.
func sshScaffoldExternalTemplateModule(m *helpers.TerragruntMirror) string {
	return "git::" + m.SSHURL + "//test/fixtures/scaffold/external-template/template"
}

// sshScaffoldInputsURL is the SSH-form source for the inputs fixture
// against the local mirror.
func sshScaffoldInputsURL(m *helpers.TerragruntMirror) string {
	return "git::" + m.SSHURL + "//test/fixtures/inputs"
}

func TestSSHScaffoldWithCustomDefaultTemplate(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	// Render so __MIRROR_SSH_URL__ in the fixture's root.hcl
	// (default_template) resolves to the live SSH mirror URL.
	tmpEnvPath := mirror.RenderFixture(t, testScaffoldWithCustomDefaultTemplate)
	helpers.CleanupTerraformFolder(t, tmpEnvPath)
	testPath := filepath.Join(tmpEnvPath, testScaffoldWithCustomDefaultTemplate)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt --non-interactive --working-dir %s scaffold %s",
		filepath.Join(testPath, "unit"),
		scaffoldModuleURL(mirror),
	))
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(testPath, "unit", "terragrunt.hcl"))
	assert.FileExists(t, filepath.Join(testPath, "unit", "external-template.txt"))
}

func TestSSHScaffoldModuleExternalTemplate(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	tmpEnvPath := helpers.TmpDirWOSymlinks(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt scaffold --non-interactive --working-dir %s %s %s",
		tmpEnvPath,
		sshScaffoldModuleURL(mirror),
		sshScaffoldExternalTemplateModule(mirror),
	))
	require.NoError(t, err)
	assert.FileExists(t, tmpEnvPath+"/external-template.txt")
	assert.FileExists(t, tmpEnvPath+"/dependency/dependency.txt")
}

// TestSSHScaffoldModuleDifferentRevisionAndSSH used to assert on the
// HTTPS→SSH URL transformation in the generated terragrunt.hcl. That
// transformation is unit-tested in
// internal/cli/commands/scaffold/source_url_test.go, so this
// integration test now just verifies that scaffold runs end-to-end
// against an SSH source URL.
func TestSSHScaffoldModuleDifferentRevisionAndSSH(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	tmpEnvPath := helpers.TmpDirWOSymlinks(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt scaffold --non-interactive --working-dir %s %s --var=Ref=v0.67.4",
		tmpEnvPath,
		sshScaffoldInputsURL(mirror),
	))
	require.NoError(t, err)

	assert.FileExists(t, tmpEnvPath+"/terragrunt.hcl")
}

func TestSSHScaffoldModuleSSH(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	tmpEnvPath := helpers.TmpDirWOSymlinks(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt scaffold --non-interactive --working-dir %s %s",
		tmpEnvPath,
		sshScaffoldModuleURL(mirror),
	))
	require.NoError(t, err)
}

func TestSSHScaffoldModuleTemplate(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	tmpEnvPath := helpers.TmpDirWOSymlinks(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt scaffold --non-interactive --working-dir %s %s",
		tmpEnvPath,
		sshScaffoldTemplateModule(mirror),
	))
	require.NoError(t, err)

	assert.FileExists(t, tmpEnvPath+"/template-file.txt")
}

// TestSSHScaffoldModuleVarFile previously asserted the
// SourceUrlType=git-ssh transformation produced a github.com SSH URL
// in the generated config. That URL-format check is unit-covered by
// scaffold's source_url_test; this test now just verifies the
// var-file plumbing reaches scaffold and EnableRootInclude=false
// suppresses find_in_parent_folders.
func TestSSHScaffoldModuleVarFile(t *testing.T) {
	mirror := helpers.StartTerragruntMirror(t)
	mirror.RequireSSH(t)

	varFileContent := `
Ref: v0.67.4
EnableRootInclude: false
`
	varFile := filepath.Join(helpers.TmpDirWOSymlinks(t), "var-file.yaml")
	err := os.WriteFile(varFile, []byte(varFileContent), 0644)
	require.NoError(t, err)

	tmpEnvPath := helpers.TmpDirWOSymlinks(t)

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(
		"terragrunt scaffold --non-interactive --working-dir %s %s --var-file=%s",
		tmpEnvPath,
		sshScaffoldInputsURL(mirror),
		varFile,
	))
	require.NoError(t, err)

	content, err := util.ReadFileAsString(tmpEnvPath + "/terragrunt.hcl")
	require.NoError(t, err)
	assert.NotContains(t, content, "find_in_parent_folders")
}
