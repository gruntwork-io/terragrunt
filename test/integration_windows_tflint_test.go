//go:build windows && tflint
// +build windows,tflint

package test_test

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
)

const testFixtureTflintNoIssuesFound = "fixtures/tflint/no-issues-found"

// Get rid of this once we have no internal tflint
func TestWindowsTflintIsInvoked(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintNoIssuesFound)
	modulePath := filepath.Join(rootPath, testFixtureTflintNoIssuesFound)
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt plan --log-level debug --working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")

	// TFLint config should be found in the original working directory, not inside .terragrunt-cache
	// The config path should end with .tflint.hcl but NOT be inside .terragrunt-cache
	// Use cross-platform regex patterns that handle both Unix / and Windows \ path separators
	found, err := regexp.MatchString(`--config\s+[^\s]*\.tflint\.hcl`, errOut.String())
	assert.NoError(t, err)
	assert.True(t, found, "Expected tflint to be invoked with --config pointing to .tflint.hcl")
	assert.NotRegexp(t, `--config\s+[^\s]*[/\\]?\.terragrunt-cache`, errOut.String(), "TFLint config should not be inside cache directory")
}
