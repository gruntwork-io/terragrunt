package test_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureFileSetPattern = "fixtures/fileset-pattern"

func TestFileSetMatchesOpenTofuForNonCanonicalPatterns(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFileSetPattern)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureFileSetPattern)
	unitPath := filepath.Join(tmpEnvPath, testFixtureFileSetPattern, "unit")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format json --non-interactive --working-dir "+unitPath,
	)
	require.NoError(t, err)

	var rendered struct {
		Inputs map[string][]string `json:"inputs"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &rendered))

	var tofuFiles map[string][]string

	require.NoError(t, json.Unmarshal(runWrappedBinaryInDir(t, unitPath), &tofuFiles))
	require.Len(t, tofuFiles, 10)

	for name, files := range tofuFiles {
		require.NotEmpty(t, files, name)
	}

	assert.Equal(t, tofuFiles, rendered.Inputs)
}

// runWrappedBinaryInDir applies the configuration in dir with the wrapped binary, without Terragrunt, and returns its files output.
func runWrappedBinaryInDir(t *testing.T, dir string) []byte {
	t.Helper()

	for _, args := range [][]string{{"init", "-input=false"}, {"apply", "-auto-approve", "-input=false"}} {
		cmd := exec.CommandContext(t.Context(), helpers.WrappedBinary(t.Context()), args...)
		cmd.Dir = dir

		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	cmd := exec.CommandContext(t.Context(), helpers.WrappedBinary(t.Context()), "output", "-json", "files")
	cmd.Dir = dir

	out, err := cmd.Output()
	require.NoError(t, err)

	return out
}
