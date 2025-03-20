package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureFindBasic             = "fixtures/find/basic"
	testFixtureFindHidden            = "fixtures/find/hidden"
	testFixtureFindDAG               = "fixtures/find/dag"
	testFixtureFindInternalVExternal = "fixtures/find/internal-v-external"
)

func TestFindBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindBasic)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+testFixtureFindBasic)
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "stack\nunit\n", stdout)
}

func TestFindBasicJSON(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindBasic)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+testFixtureFindBasic+" --json")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.JSONEq(t, `[{"type": "stack", "path": "stack"}, {"type": "unit", "path": "unit"}]`, stdout)
}

func TestFindHidden(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		hidden   bool
		expected string
	}{
		{
			name:     "visible",
			expected: "stack\nunit\n",
		},
		{
			name:     "hidden",
			hidden:   true,
			expected: ".hide/unit\nstack\nunit\n",
		},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindHidden)

			cmd := "terragrunt find --experiment cli-redesign --no-color --working-dir " + testFixtureFindHidden

			if tt.hidden {
				cmd += " --hidden"
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			assert.Empty(t, stderr)
			assert.Equal(t, tt.expected, stdout)
		})
	}
}

func TestFindDAG(t *testing.T) {
	t.Parallel()

	tc := []struct {
		name     string
		sort     string
		expected string
	}{
		{name: "alpha", sort: "alpha", expected: "a-dependent\nb-dependency\n"},
		{name: "dag", sort: "dag", expected: "b-dependency\na-dependent\n"},
	}

	for _, tt := range tc {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureFindDAG)

			cmd := "terragrunt find --experiment cli-redesign --no-color --working-dir " + testFixtureFindDAG

			if tt.sort == "dag" {
				cmd += " --dag"
			}

			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err)

			assert.Empty(t, stderr)
			assert.Equal(t, tt.expected, stdout)
		})
	}
}

func TestFindExternalDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindInternalVExternal)

	internalDir := filepath.Join(testFixtureFindInternalVExternal, "internal")

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+internalDir+" --dependencies --external")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "../external/c-dependency\na-dependent\nb-dependency\n", stdout)

	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+internalDir+" --dependencies")
	require.NoError(t, err)

	assert.Empty(t, stderr)
	assert.Equal(t, "a-dependent\nb-dependency\n", stdout)
}
