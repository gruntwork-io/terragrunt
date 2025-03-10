package test_test

import (
	"bytes"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureFindBasic  = "fixtures/find/basic"
	testFixtureFindHidden = "fixtures/find/hidden"
)

func TestFindBasic(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindBasic)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+testFixtureFindBasic, stdout, stderr)
	require.NoError(t, err)

	assert.Equal(t, "", stderr.String())
	assert.Equal(t, "stack\nunit\n", stdout.String())
}

func TestFindBasicJSON(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindBasic)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	err := helpers.RunTerragruntCommand(t, "terragrunt find --experiment cli-redesign --no-color --working-dir "+testFixtureFindBasic+" --json", stdout, stderr)
	require.NoError(t, err)

	assert.Equal(t, "", stderr.String())
	assert.JSONEq(t, `[{"type": "stack", "path": "stack"}, {"type": "unit", "path": "unit"}]`, stdout.String())
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

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			cmd := "terragrunt find --experiment cli-redesign --no-color --working-dir " + testFixtureFindHidden

			if tt.hidden {
				cmd += " --hidden"
			}

			err := helpers.RunTerragruntCommand(t, cmd, stdout, stderr)
			require.NoError(t, err)

			assert.Equal(t, "", stderr.String())
			assert.Equal(t, tt.expected, stdout.String())
		})
	}
}
