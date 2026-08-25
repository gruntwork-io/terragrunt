package test_test

import (
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
