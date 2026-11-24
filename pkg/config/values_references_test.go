package config_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestValueReferences(t *testing.T) {
	t.Parallel()

	const content = `values {
  value "tags" {
    default = values.skipped
  }
}

locals {
  everything = values
  dynamic    = values[local.key]
  scoped     = [for values in ["a"] : values.skipped]
  nested     = values.tags.name
}

inputs = {
  tags   = try(values.tgas, {})
  region = values["region"]
  name   = "app-${values.name}"
}

generate "provider" {
  path     = "provider.tf"
  contents = values.provider
}
`

	type reference struct {
		name string
		line int
	}

	refs := valueReferences(t, content, config.DefaultTerragruntConfigPath)

	found := make([]reference, 0, len(refs))
	for _, ref := range refs {
		found = append(found, reference{name: ref.Name, line: ref.Range.Start.Line})
	}

	assert.Equal(t, []reference{
		{name: "tags", line: 11},
		{name: "tgas", line: 15},
		{name: "region", line: 16},
		{name: "name", line: 17},
		{name: "provider", line: 22},
	}, found)
}

func TestValueReferencesSkipsJSON(t *testing.T) {
	t.Parallel()

	refs := valueReferences(t, `{"inputs": {"name": "${values.name}"}}`, config.DefaultTerragruntJSONConfigPath)

	assert.Empty(t, refs)
}

func TestValueReferencesBoundsBlockNesting(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		depth    int
		exceeded bool
	}{
		{name: "blocks nested to the limit are walked", depth: 32},
		{name: "blocks nested past the limit are an error", depth: 33, exceeded: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			content := strings.Repeat("a {\n", tc.depth) + "id = values.name\n" + strings.Repeat("}\n", tc.depth)

			file := parseValuesReferenceFile(t, content, config.DefaultTerragruntConfigPath)

			refs, err := config.ValueReferences(file)

			if !tc.exceeded {
				require.NoError(t, err)
				require.Len(t, refs, 1)
				assert.Equal(t, "name", refs[0].Name)

				return
			}

			var nestingErr config.BlockNestingExceededError

			require.ErrorAs(t, err, &nestingErr)
			assert.Equal(t, 32, nestingErr.Limit)
			assert.Equal(t, config.DefaultTerragruntConfigPath, nestingErr.ConfigPath)
		})
	}
}

// valueReferences returns the value references of content parsed as the file at path.
func valueReferences(t *testing.T, content, path string) []config.ValueReference {
	t.Helper()

	refs, err := config.ValueReferences(parseValuesReferenceFile(t, content, path))
	require.NoError(t, err)

	return refs
}

// parseValuesReferenceFile parses content as the file at path.
func parseValuesReferenceFile(t *testing.T, content, path string) *hclparse.File {
	t.Helper()

	_, pctx := newTestParsingContext(t, path)

	file, err := pctx.NewParser(logger.CreateLogger(), venv.MemVenv()).ParseFromString(content, path)
	require.NoError(t, err)

	return file
}
