package test_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureIncludeRelativePaths = "fixtures/include-relative-paths"

// TestIncludedConfigReadsRelativePathsFromItsOwnDirectory covers the HCL functions that take a file
// path and resolve it themselves. Called from a configuration pulled in by an include block, a
// relative path names a file next to that configuration, not next to the unit doing the including.
// The fixture puts a file of the same name in both directories, so resolving against the wrong one
// reads the wrong file rather than failing.
func TestIncludedConfigReadsRelativePathsFromItsOwnDirectory(t *testing.T) {
	t.Parallel()

	workingDir, err := filepath.Abs(filepath.Join(testFixtureIncludeRelativePaths, "unit"))
	require.NoError(t, err)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --json --non-interactive --working-dir "+workingDir,
	)
	require.NoError(t, err, "stderr: %s", stderr)

	var rendered struct {
		Inputs map[string]string `json:"inputs"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &rendered))

	assert.Equal(t, "common", rendered.Inputs["tfvars_origin"], "read_tfvars_file read the wrong file")
	assert.Equal(t, "common", rendered.Inputs["config_origin"], "read_terragrunt_config read the wrong file")
}
