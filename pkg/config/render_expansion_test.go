package config_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// renderConfig renders cfg the way `terragrunt render` does.
func renderConfig(t *testing.T, cfg *config.TerragruntConfig) string {
	t.Helper()

	rendered := &bytes.Buffer{}
	_, err := cfg.WriteTo(rendered)
	require.NoError(t, err)

	return rendered.String()
}

// renderDependencies parses cfgHCL and renders it back out.
func renderDependencies(t *testing.T, cfgHCL string) string {
	t.Helper()

	cfg, err := parseDependencyString(t, cfgHCL)
	require.NoError(t, err)

	return renderConfig(t, cfg)
}

// blocksOpening counts the dependency blocks rendered as configuration, so a test can tell
// the block that was written from the elements previewing what it expanded to.
func blocksOpening(rendered, label string) int {
	opening := "dependency " + label + " {"
	count := 0

	for line := range strings.SplitSeq(rendered, "\n") {
		if line == opening {
			count++
		}
	}

	return count
}

// TestRenderPreviewsForEachExpansion pins the preview shape for for_each: the block as it
// was written, its references still unresolved, followed by one commented element per
// iteration with its body resolved.
func TestRenderPreviewsForEachExpansion(t *testing.T) {
	t.Parallel()

	rendered := renderDependencies(t, `
dependency "region" {
  expansion {
    for_each = {
      use1 = "us-east-1"
      usw2 = "us-west-2"
    }
  }

  config_path = "../${each.value}"
}
`)

	assert.Contains(t, rendered, `for_each = {`, "the block keeps the collection it was written with")
	assert.Contains(t, rendered, `config_path = "../${each.value}"`, "the block keeps its unresolved body")

	assert.Contains(t, rendered, `# Expands to:
#
# dependency "region" {
#   config_path = "../us-east-1"
# }
#
# dependency "region" {
#   config_path = "../us-west-2"
# }`)

	assert.Equal(t, 1, blocksOpening(rendered, `"region"`), "only the written block renders as configuration")
}

// TestRenderPreviewsCountExpansion pins the preview shape for count.
func TestRenderPreviewsCountExpansion(t *testing.T) {
	t.Parallel()

	rendered := renderDependencies(t, `
dependency "shard" {
  expansion {
    count = 2
  }

  config_path = "../shard-${count.index}"
}
`)

	assert.Contains(t, rendered, "count = 2", "the block keeps the count it was written with")
	assert.Contains(t, rendered, `config_path = "../shard-${count.index}"`)

	assert.Contains(t, rendered, `# Expands to:
#
# dependency "shard" {
#   config_path = "../shard-0"
# }
#
# dependency "shard" {
#   config_path = "../shard-1"
# }`)

	assert.Equal(t, 1, blocksOpening(rendered, `"shard"`))
}

// TestRenderLeavesUnexpandedBlocksAlone pins that a block that declares no expansion
// renders exactly as it did before previews existed, with no comment bolted on.
func TestRenderLeavesUnexpandedBlocksAlone(t *testing.T) {
	t.Parallel()

	rendered := renderDependencies(t, `
dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}
`)

	assert.Contains(t, rendered, `"../vpc"`)
	assert.Contains(t, rendered, "skip_outputs = true")
	assert.NotContains(t, rendered, "expansion")
	assert.NotContains(t, rendered, "#")
}

// TestRenderJSONExpansionOmitsPreview pins what a JSON config renders as. Its blocks carry
// no HCL text to quote back, so the elements render as configuration, which is the one
// case where render repeats a label outside a comment.
func TestRenderJSONExpansionOmitsPreview(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, jsonDependencyWithExpansion)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.NotContains(t, rendered, "Expands to")
	assert.Contains(t, rendered, `"../aurora-0"`)
	assert.Contains(t, rendered, `"../aurora-1"`)
	assert.Equal(t, 2, blocksOpening(rendered, `"aurora"`))
}
