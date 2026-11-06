package test_test

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureFileSetPattern = "fixtures/fileset-pattern"

func TestFileSetNonCanonicalPatternsInConfig(t *testing.T) {
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

	want := map[string][]string{
		"dot_slash":     {"resources/a.json"},
		"canonical":     {"resources/a.json"},
		"double_star":   {"resources/a.json", "resources/sub/b.json"},
		"absolute_path": {"resources/a.json"},
		"parent":        {"a.json"},
		"above":         {"../shared/c.json"},
		"included":      {"resources/root.json"},
		"escaped":       {"resources/a.json"},
	}

	// A backslash is a path separator on Windows, so the pattern cleans to resources/x/*.json there.
	if runtime.GOOS == "windows" {
		want["escaped"] = []string{}
	}

	assert.Equal(t, want, rendered.Inputs)
}
