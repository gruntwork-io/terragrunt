//go:build !tflint
// +build !tflint

package test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

func TestTflintFindsNoIssuesWithValidCode(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TEST_FIXTURE_TFLINT_NO_ISSUES_FOUND)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_TFLINT_NO_ISSUES_FOUND)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")

	assert.Contains(t, errOut.String(), fmt.Sprintf("--config %s/.tflint.hcl", modulePath))
}

func TestTflintFindsModule(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TEST_FIXTURE_TFLINT_MODULE_FOUND)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_TFLINT_MODULE_FOUND)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")
}

func TestTflintFindsIssuesWithInvalidInput(t *testing.T) {
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TEST_FIXTURE_TFLINT_ISSUES_FOUND)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_TFLINT_ISSUES_FOUND)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-working-dir %s", modulePath), os.Stdout, errOut)
	assert.Error(t, err, "Tflint found issues in the project. Check for the tflint logs")
}

func TestTflintWithoutConfigFile(t *testing.T) {
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TEST_FIXTURE_TFLINT_NO_CONFIG_FILE)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_TFLINT_NO_CONFIG_FILE)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-working-dir %s", modulePath), io.Discard, errOut)
	assert.Error(t, err, "Could not find .tflint.hcl config file in the parent folders:")
}

func TestTflintFindsConfigInCurrentPath(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := copyEnvironmentWithTflint(t, TEST_FIXTURE_TFLINT_NO_TF_SOURCE_PATH)
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_TFLINT_NO_TF_SOURCE_PATH)
	err := runTerragruntCommand(t, fmt.Sprintf("terragrunt plan --terragrunt-log-level debug --terragrunt-working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.Contains(t, errOut.String(), "Tflint has run successfully. No issues found")
	assert.Contains(t, errOut.String(), fmt.Sprintf("--config %s/.tflint.hcl", modulePath))
}

func TestInitSameModule(t *testing.T) {

	cleanupTerraformFolder(t, TEST_FIXTURE_PARALLEL_RUN)
	rootPath := copyEnvironmentWithTflint(t, TEST_FIXTURE_PARALLEL_RUN)
	t.Cleanup(func() {
		cleanupTerraformFolder(t, rootPath)
	})
	modulePath := util.JoinPath(rootPath, TEST_FIXTURE_PARALLEL_RUN)
	runPath := util.JoinPath(rootPath, TEST_FIXTURE_PARALLEL_RUN, "dev")
	appTemplate := util.JoinPath(rootPath, TEST_FIXTURE_PARALLEL_RUN, "dev", "app")

	fmt.Printf("modulePath: %v \n", modulePath)
	fmt.Printf("runPath: %v \n", runPath)

	for i := 0; i < 50; i++ {
		appPath := util.JoinPath(modulePath, "dev", fmt.Sprintf("app-%d", i))
		err := util.CopyFolderContents(appTemplate, appPath, ".terragrunt-test", []string{})
		assert.NoError(t, err)
	}
	runTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir %s", runPath))
}
