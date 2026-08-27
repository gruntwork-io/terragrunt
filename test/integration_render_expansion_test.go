package test_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/test/helpers"
)

const testFixtureRenderExpansion = "fixtures/render-expansion"

// TestRenderPreviewsExpandedDependencies pins the preview the render command prints for a
// config whose dependencies expand: each block as it was written, followed by the elements
// it expanded into, commented out and resolved.
func TestRenderPreviewsExpandedDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRenderExpansion)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderExpansion)
	appPath := filepath.Join(tmpEnvPath, testFixtureRenderExpansion, "app")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format hcl --experiment block-iteration"+
			" --non-interactive --working-dir "+appPath,
	)
	require.NoError(t, err)

	assert.Contains(t, stdout, `dependency "aurora" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path  = "../aurora-${each.key}"
  skip_outputs = true
}

# Expands to:
#
# dependency "aurora" {
#   config_path  = "../aurora-api"
#   skip_outputs = true
# }
#
# dependency "aurora" {
#   config_path  = "../aurora-web"
#   skip_outputs = true
# }`)

	assert.Contains(t, stdout, `dependency "shard" {
  expansion {
    count = 2
  }

  config_path  = "../shard-${count.index}"
  skip_outputs = true
}

# Expands to:
#
# dependency "shard" {
#   config_path  = "../shard-0"
#   skip_outputs = true
# }
#
# dependency "shard" {
#   config_path  = "../shard-1"
#   skip_outputs = true
# }`)

	// The unexpanded dependency renders on its own, with no preview attached.
	assert.Contains(t, stdout, `dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}`)
}

// TestRenderPreviewsExpandedJSONDependencies pins that a JSON config previews the same way an
// HCL one does, and that the file render writes reads back. Rendering the elements as
// configuration instead would repeat one label, which Terragrunt rejects under the strict
// control.
func TestRenderPreviewsExpandedJSONDependencies(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRenderExpansion)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderExpansion)
	appPath := filepath.Join(tmpEnvPath, testFixtureRenderExpansion, "json-app")

	_, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format hcl --write --experiment block-iteration"+
			" --non-interactive --working-dir "+appPath,
	)
	require.NoError(t, err)

	rendered, err := os.ReadFile(filepath.Join(appPath, "terragrunt.rendered.hcl"))
	require.NoError(t, err)

	assert.Contains(t, string(rendered), `dependency "aurora" {
  expansion {
    count = 2
  }

  config_path  = "../aurora-${count.index}"
  skip_outputs = true
}

# Expands to:
#
# dependency "aurora" {
#   config_path  = "../aurora-0"
#   skip_outputs = true
# }
#
# dependency "aurora" {
#   config_path  = "../aurora-1"
#   skip_outputs = true
# }`)

	// JSON writes repeated blocks of one type as an array, so a block written as a single-element
	// array is the same block as one written as an object.
	assert.Contains(t, string(rendered), `dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}`)

	reparsePath := filepath.Join(tmpEnvPath, testFixtureRenderExpansion, "reparse")
	require.NoError(t, os.MkdirAll(reparsePath, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(reparsePath, "terragrunt.hcl"), rendered, 0644))

	// Without the strict control a repeated label is only a warning, so the read-back would
	// pass on output that no future version of Terragrunt will accept.
	reparsed, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format hcl --experiment block-iteration"+
			" --strict-control duplicate-dependency-labels"+
			" --non-interactive --working-dir "+reparsePath,
	)
	require.NoError(t, err)

	assert.Equal(t, string(rendered), reparsed, "rendering the rendered config returns it unchanged")
}

// TestRenderJSONFormatKeepsExpandedDependenciesWhole pins that the JSON format serializes the
// block that was written rather than the elements it expanded into. Its dependency map is keyed
// by label, which every element shares, and JSON has no comment to preview them in.
func TestRenderJSONFormatKeepsExpandedDependenciesWhole(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureRenderExpansion)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRenderExpansion)
	appPath := filepath.Join(tmpEnvPath, testFixtureRenderExpansion, "app")

	stdout, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt render --format json --experiment block-iteration"+
			" --non-interactive --working-dir "+appPath,
	)
	require.NoError(t, err)

	var rendered struct {
		Dependency map[string]map[string]any `json:"dependency"`
	}

	require.NoError(t, json.Unmarshal([]byte(stdout), &rendered))

	assert.Equal(t, map[string]any{
		"config_path":  "../shard-${count.index}",
		"expansion":    map[string]any{"count": float64(2)},
		"skip_outputs": true,
	}, rendered.Dependency["shard"])

	assert.Equal(t, map[string]any{
		"config_path":  "../aurora-${each.key}",
		"expansion":    map[string]any{"for_each": `${toset(["web", "api"])}`},
		"skip_outputs": true,
	}, rendered.Dependency["aurora"])

	// The unexpanded dependency serializes as it always has, resolved.
	assert.Equal(t, "../vpc", rendered.Dependency["vpc"]["config_path"])
	assert.Equal(t, true, rendered.Dependency["vpc"]["skip"])
}
