package config_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctyjson "github.com/zclconf/go-cty/cty/json"
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

// TestRenderJSONExpansionQuotesBlockAsHCL pins that a JSON config renders the same way an HCL
// one does. The block comes back as the HCL that means the same thing, references unresolved,
// and the elements follow as comments.
func TestRenderJSONExpansionQuotesBlockAsHCL(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, jsonDependencyWithExpansion)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `dependency "aurora" {
  expansion {
    count = 2
  }

  config_path = "../aurora-${count.index}"
}`)

	assert.Contains(t, rendered, `# Expands to:
#
# dependency "aurora" {
#   config_path = "../aurora-0"
# }
#
# dependency "aurora" {
#   config_path = "../aurora-1"
# }`)

	assert.Equal(t, 1, blocksOpening(rendered, `"aurora"`), "only the written block renders as configuration")
}

// TestRenderJSONExpansionRendersFixedPoint pins that rendering a JSON config produces a
// configuration that renders back to itself. Quoting the elements instead would repeat one
// label, and that would not read back at all.
func TestRenderJSONExpansionRendersFixedPoint(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, jsonDependencyWithExpansion)
	require.NoError(t, err)

	first := renderConfig(t, cfg)

	reparsed, err := parseDependencyStringStrict(t, first)
	require.NoError(t, err)

	assert.Equal(t, first, renderConfig(t, reparsed))
}

// TestRenderJSONExpansionGroupsEachBlockSeparately pins that two expanded blocks in one JSON
// config each get a block and a preview of their own, rather than folding into one.
func TestRenderJSONExpansionGroupsEachBlockSeparately(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {
  "aurora": {"expansion": {"count": 1}, "config_path": "../aurora-${count.index}"},
  "shard": {"expansion": {"count": 1}, "config_path": "../shard-${count.index}"}
}}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Equal(t, 1, blocksOpening(rendered, `"aurora"`))
	assert.Equal(t, 1, blocksOpening(rendered, `"shard"`))

	assert.Contains(t, rendered, `  config_path = "../aurora-${count.index}"`)
	assert.Contains(t, rendered, `  config_path = "../shard-${count.index}"`)
	assert.Equal(t, 2, strings.Count(rendered, "# Expands to:"))
}

// TestRenderJSONExpansionQuotesValuesAsHCL pins the value spellings that differ between the two
// syntaxes. JSON allows the escape \/, which HCL rejects, and writes objects with colons.
func TestRenderJSONExpansionQuotesValuesAsHCL(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {"aurora": {
  "expansion": {"count": 1},
  "config_path": "../aurora-${count.index}",
  "mock_outputs": {
    "url": "https:\/\/example.com\/${count.index}",
    "tags": ["a", "b"],
    "nested": {"on": false, "none": null, "size": 3},
    "quote": "say \"hi\"",
    "unicode": "café"
  }
}}}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `  mock_outputs = {
    url  = "https://example.com/${count.index}"
    tags = ["a", "b"]
    nested = {
      on   = false
      none = null
      size = 3
    }
    quote   = "say \"hi\""
    unicode = "café"
  }`)
}

// renderJSONDependencies renders cfg's dependency blocks the way `render --format json` does.
func renderJSONDependencies(t *testing.T, cfg *config.TerragruntConfig) map[string]any {
	t.Helper()

	asCty, err := config.TerragruntConfigAsCty(cfg)
	require.NoError(t, err)

	encoded, err := ctyjson.Marshal(asCty, asCty.Type())
	require.NoError(t, err)

	var rendered struct {
		Dependency map[string]any `json:"dependency"`
	}

	require.NoError(t, json.Unmarshal(encoded, &rendered))

	return rendered.Dependency
}

// TestRenderJSONFormatKeepsExpansionUnexpanded pins that the JSON format serializes the block
// that was written rather than the elements it expanded into. The map is keyed by label, which
// every element shares, so serializing the elements would keep only the last of them.
func TestRenderJSONFormatKeepsExpansionUnexpanded(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
dependency "shard" {
  expansion {
    count = 2
  }

  config_path  = "../shard-${count.index}"
  skip_outputs = true
}
`)
	require.NoError(t, err)

	deps := renderJSONDependencies(t, cfg)
	require.Contains(t, deps, "shard")

	assert.Equal(t, map[string]any{
		"config_path":  "../shard-${count.index}",
		"expansion":    map[string]any{"count": float64(2)},
		"skip_outputs": true,
	}, deps["shard"])
}

// TestRenderJSONFormatKeepsForEachExpression pins that a for_each is written back as the call
// that built it. Serializing the set it evaluated to would emit a JSON array, which for_each
// rejects, so the output would no longer read back.
func TestRenderJSONFormatKeepsForEachExpression(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
dependency "region" {
  expansion {
    for_each = toset(["web", "api"])
  }

  config_path = "../${each.key}"
}
`)
	require.NoError(t, err)

	deps := renderJSONDependencies(t, cfg)
	require.Contains(t, deps, "region")

	assert.Equal(t, map[string]any{
		"config_path": "../${each.key}",
		"expansion":   map[string]any{"for_each": `${toset(["web", "api"])}`},
	}, deps["region"])
}

// TestRenderJSONFormatLeavesUnexpandedBlocksAlone pins that a block that declares no expansion
// serializes as it always has, resolved and under the names the rest of the format uses.
func TestRenderJSONFormatLeavesUnexpandedBlocksAlone(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}
`)
	require.NoError(t, err)

	deps := renderJSONDependencies(t, cfg)
	require.Contains(t, deps, "vpc")

	vpc, ok := deps["vpc"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "../vpc", vpc["config_path"])
	assert.Equal(t, true, vpc["skip"])
	assert.Equal(t, "vpc", vpc["name"])
}

// TestRenderJSONExpansionQuotesInterpolationsAsWritten pins the escaping around a quote inside
// an interpolation. JSON escapes every quote in a string, and HCL reads what is inside ${ } as
// an expression, where an escaped quote does not parse.
func TestRenderJSONExpansionQuotesInterpolationsAsWritten(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {"shard": {
  "expansion": {"count": 1},
  "config_path": "../${lower(\"SHARD\")}-${count.index}",
  "mock_outputs": {"literal": "costs $${100}", "quoted": "say \"hi\""}
}}}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `config_path = "../${lower("SHARD")}-${count.index}"`)
	assert.Contains(t, rendered, `literal = "costs $${100}"`)
	assert.Contains(t, rendered, `quoted  = "say \"hi\""`)

	reparsed, err := parseDependencyStringStrict(t, rendered)
	require.NoError(t, err)

	assert.Equal(t, rendered, renderConfig(t, reparsed))
}

// TestRenderJSONFormatKeepsExpressionsWhereTheyAre pins the shapes that a single interpolation
// around the whole value would get wrong. A collection keeps its shape, and only the values
// holding a reference become templates. An expression already written as one interpolation is
// not wrapped in another. A directive stays in the template syntax it is already written in.
func TestRenderJSONFormatKeepsExpressionsWhereTheyAre(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
locals {
  on = true
}

dependency "shard" {
  expansion {
    count = 2
  }

  config_path = "../shard-${count.index}"
  enabled     = "${local.on}"
  mock_outputs = {
    directive = "%{if local.on}yes%{else}no%{endif}"
    wrapped   = "${count.index}"
    settled   = "plain"
    tags      = ["a", "${count.index}"]
  }
}
`)
	require.NoError(t, err)

	deps := renderJSONDependencies(t, cfg)
	require.Contains(t, deps, "shard")

	assert.Equal(t, map[string]any{
		"config_path": "../shard-${count.index}",
		"enabled":     "${local.on}",
		"expansion":   map[string]any{"count": float64(2)},
		"mock_outputs": map[string]any{
			"directive": "%{if local.on}yes%{else}no%{endif}",
			"wrapped":   "${count.index}",
			"settled":   "plain",
			"tags":      []any{"a", "${count.index}"},
		},
	}, deps["shard"])
}

// TestRenderJSONExpansionEscapesValuesForHCL pins the characters the two syntaxes spell
// differently, and the keys HCL cannot write bare. A miss here renders a block that no longer
// parses, so the round-trip at the end is the assertion that matters most.
func TestRenderJSONExpansionEscapesValuesForHCL(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {"shard": {
  "expansion": {"count": 1},
  "config_path": "../shard-${count.index}",
  "mock_outputs": {
    "backslash": "a\\b",
    "tab": "a\tb",
    "newline": "a\nb",
    "control": "a\u0001b",
    "quote-in-call": "${lower(\"A\\\"B\")}",
    "not an identifier": 1,
    "empty_object": {},
    "empty_list": []
  }
}}}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `  mock_outputs = {
    backslash           = "a\\b"
    tab                 = "a\tb"
    newline             = "a\nb"
    control             = "a\u0001b"
    quote-in-call       = "${lower("A\"B")}"
    "not an identifier" = 1
    empty_object        = {}
    empty_list          = []
  }`)

	reparsed, err := parseDependencyStringStrict(t, rendered)
	require.NoError(t, err)

	assert.Equal(t, rendered, renderConfig(t, reparsed))
}

// TestRenderExpansionKeepsComputedObjectKeys pins a key JSON has no way to spell, such as one
// computed from the iteration. The object goes back whole as one interpolation rather than
// failing the render. The HCL format depends on this too, since it builds the same cty value
// even though it quotes the block from source.
func TestRenderExpansionKeepsComputedObjectKeys(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyString(t, `
dependency "shard" {
  expansion {
    count = 2
  }

  config_path = "../shard-${count.index}"
  mock_outputs = {
    (count.index) = "keyed by iteration"
  }
}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `  mock_outputs = {
    (count.index) = "keyed by iteration"
  }`)

	reparsed, err := parseDependencyStringStrict(t, rendered)
	require.NoError(t, err)
	assert.Equal(t, rendered, renderConfig(t, reparsed))

	shard, ok := renderJSONDependencies(t, cfg)["shard"].(map[string]any)
	require.True(t, ok)

	mocks, ok := shard["mock_outputs"].(string)
	require.True(t, ok, "a computed key leaves the object as one interpolation")

	assert.Contains(t, mocks, `(count.index) = "keyed by iteration"`)
	assert.True(t, strings.HasPrefix(mocks, "${"))
}

// TestRenderJSONArrayBlockRendersEachElement pins that a block a JSON config wrote as an array
// of objects renders as one HCL block per element. JSON has no way to repeat a property, so an
// array is how it writes a block once or several times over.
func TestRenderJSONArrayBlockRendersEachElement(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {
  "vpc": [{"config_path": "../vpc", "skip_outputs": true}],
  "shard": [
    {"config_path": "../shard-a"},
    {"config_path": "../shard-b"}
  ]
}}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `dependency "vpc" {
  config_path  = "../vpc"
  skip_outputs = true
}`)

	assert.Contains(t, rendered, `config_path = "../shard-a"`)
	assert.Contains(t, rendered, `config_path = "../shard-b"`)
	assert.Equal(t, 2, blocksOpening(rendered, `"shard"`))
}

// TestRenderJSONFormatReadsArrayBlocks pins that the JSON format serializes a block written as
// an array too, rather than failing the render.
func TestRenderJSONFormatReadsArrayBlocks(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {"shard": [{
  "//": "one of several",
  "expansion": {"count": 2},
  "config_path": "../shard-${count.index}",
  "skip_outputs": true
}]}}
`)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"config_path":  "../shard-${count.index}",
		"expansion":    map[string]any{"count": float64(2)},
		"skip_outputs": true,
	}, renderJSONDependencies(t, cfg)["shard"])
}

// TestRenderJSONArrayBlockPreviewsEachElementSeparately pins that two expanded blocks written in
// one array each get a block and a preview of their own, rather than folding into one.
func TestRenderJSONArrayBlockPreviewsEachElementSeparately(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {"shard": [
  {"expansion": {"count": 1}, "config_path": "../a-${count.index}"},
  {"expansion": {"count": 1}, "config_path": "../b-${count.index}"}
]}}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `  config_path = "../a-${count.index}"`)
	assert.Contains(t, rendered, `  config_path = "../b-${count.index}"`)

	assert.Contains(t, rendered, `# dependency "shard" {
#   config_path = "../a-0"
# }`)
	assert.Contains(t, rendered, `# dependency "shard" {
#   config_path = "../b-0"
# }`)

	assert.Equal(t, 2, blocksOpening(rendered, `"shard"`))
	assert.Equal(t, 2, strings.Count(rendered, "# Expands to:"))
}

// TestRenderJSONNestedBlockArray pins that a nested block written as an array renders the same
// way the object form does, since one element is one block.
func TestRenderJSONNestedBlockArray(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {"shard": {
  "expansion": [{"count": 2}],
  "config_path": "../shard-${count.index}"
}}}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `dependency "shard" {
  expansion {
    count = 2
  }

  config_path = "../shard-${count.index}"
}`)

	assert.Contains(t, rendered, `#   config_path = "../shard-1"`)
}

// TestRenderJSONNullNestedBlockRendersNothing pins that a nested block written as null renders
// as no block at all, which is how HCL reads it.
func TestRenderJSONNullNestedBlockRendersNothing(t *testing.T) {
	t.Parallel()

	cfg, err := parseDependencyJSONString(t, `
{"dependency": {"vpc": {"expansion": null, "config_path": "../vpc"}}}
`)
	require.NoError(t, err)

	rendered := renderConfig(t, cfg)

	assert.Contains(t, rendered, `dependency "vpc" {
  config_path = "../vpc"
}`)
	assert.NotContains(t, rendered, "expansion")
}
