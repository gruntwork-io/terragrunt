//go:build tf

package test_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

func TestTFValuesBlockStackRunPassesResolvedValues(t *testing.T) {
	t.Parallel()

	stackPath := filepath.Join(copyValuesBlockFixture(t), "live", "valid")

	helpers.RunTerragrunt(
		t,
		"terragrunt stack run apply --non-interactive --experiment values-block --working-dir "+stackPath,
	)

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt output -json --non-interactive --experiment values-block --working-dir "+
			filepath.Join(stackPath, ".terragrunt-stack", "app"),
	)
	require.NoError(t, err)

	var outputs map[string]struct {
		Value any `json:"value"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &outputs))

	assert.Equal(t, "web", outputs["name"].Value)
	assert.InDelta(t, 3, outputs["instance_count"].Value, 0)
	assert.Equal(t, map[string]any{}, outputs["tags"].Value)
}
