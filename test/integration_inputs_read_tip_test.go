package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureInputsReadTip = "fixtures/tips/inputs-read"

// TestTipInputsReadShownWhenTrackingReads pins that a command tracking reads prints the
// inputs-read-not-tracked tip naming each read in `inputs`, those of an included file too, and
// not a read in `locals`.
func TestTipInputsReadShownWhenTrackingReads(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args string
	}{
		{name: "reading output", args: "--reading"},
		{name: "reading filter", args: "--filter reading=config.yml"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureInputsReadTip)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputsReadTip)
			rootPath := filepath.Join(tmpEnvPath, testFixtureInputsReadTip)

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt find "+tc.args+" --no-color --working-dir "+rootPath,
			)
			require.NoError(t, err)

			assert.Contains(t, stderr, "TIP (inputs-read-not-tracked)")
			assert.Contains(t, stderr, " --> "+filepath.Join("app", "terragrunt.hcl")+":6:24\n")
			assert.Contains(t, stderr, "6 |   message = yamldecode(file(\"../config.yml\")).message\n"+
				"  |                        ^^^^^^^^^^^^^^^^^^^^^")
			assert.Contains(t, stderr, " --> root.hcl:2:24\n")
			assert.Contains(t, stderr, "2 |   account = yamldecode(file(\"config.yml\")).message\n"+
				"  |                        ^^^^^^^^^^^^^^^^^^")
			assert.NotContains(t, stderr, filepath.Join("other", "terragrunt.hcl"))
			assert.NotContains(t, stderr, "\x1b[")
		})
	}
}

// TestTipInputsReadQuiet pins that the inputs-read-not-tracked tip stays quiet when no command
// tracks reads and when the tip is disabled.
func TestTipInputsReadQuiet(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		args string
	}{
		{name: "reads not tracked", args: ""},
		{name: "tip disabled", args: "--reading --no-tip inputs-read-not-tracked"},
		{name: "tips disabled", args: "--reading --no-tips"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureInputsReadTip)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputsReadTip)
			rootPath := filepath.Join(tmpEnvPath, testFixtureInputsReadTip)

			_, stderr, err := helpers.RunTerragruntCommandWithOutput(
				t,
				"terragrunt find "+tc.args+" --no-color --working-dir "+rootPath,
			)
			require.NoError(t, err)

			assert.NotContains(t, stderr, "TIP (inputs-read-not-tracked)")
		})
	}
}
