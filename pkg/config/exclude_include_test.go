package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression test for https://github.com/gruntwork-io/terragrunt/issues/5089:
// an exclude block defined in an included parent config must survive into the
// child's merged config when the child does not define its own exclude block.
func TestParseConfig_InheritsExcludeFromIncludedConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		includeBody string
	}{
		{
			name:        "default merge",
			includeBody: ``,
		},
		{
			name:        "shallow merge",
			includeBody: `merge_strategy = "shallow"`,
		},
		{
			name:        "deep merge",
			includeBody: `merge_strategy = "deep"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()

			parentPath := filepath.Join(tmpDir, "root.hcl")
			require.NoError(t, os.WriteFile(parentPath, []byte(`
exclude {
  if      = true
  actions = ["plan", "apply"]
  no_run  = true
}
`), 0644))

			childDir := filepath.Join(tmpDir, "unit")
			require.NoError(t, os.MkdirAll(childDir, 0755))

			childPath := filepath.Join(childDir, config.DefaultTerragruntConfigPath)
			require.NoError(t, os.WriteFile(childPath, []byte(`
include "root" {
  path = "`+parentPath+`"
  `+tt.includeBody+`
}
`), 0644))

			ctx, pctx := newTestParsingContext(t, childPath)

			l := logger.CreateLogger()

			parsed, err := config.ParseConfigFile(ctx, pctx, l, childPath, nil)
			require.NoError(t, err)
			require.NotNil(t, parsed)

			require.NotNil(t, parsed.Exclude, "expected exclude block to be inherited from included parent")
			assert.True(t, parsed.Exclude.If)
			assert.Equal(t, []string{"plan", "apply"}, parsed.Exclude.Actions)
			require.NotNil(t, parsed.Exclude.NoRun)
			assert.True(t, *parsed.Exclude.NoRun)
		})
	}
}

// Child-defined exclude blocks must still take precedence over the included
// parent's exclude block, regardless of merge strategy.
func TestParseConfig_ChildExcludeOverridesIncludedConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		includeBody string
	}{
		{
			name:        "default merge",
			includeBody: ``,
		},
		{
			name:        "shallow merge",
			includeBody: `merge_strategy = "shallow"`,
		},
		{
			name:        "deep merge",
			includeBody: `merge_strategy = "deep"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()

			parentPath := filepath.Join(tmpDir, "root.hcl")
			require.NoError(t, os.WriteFile(parentPath, []byte(`
exclude {
  if      = true
  actions = ["plan"]
  no_run  = false
}
`), 0644))

			childDir := filepath.Join(tmpDir, "unit")
			require.NoError(t, os.MkdirAll(childDir, 0755))

			childPath := filepath.Join(childDir, config.DefaultTerragruntConfigPath)
			require.NoError(t, os.WriteFile(childPath, []byte(`
include "root" {
  path = "`+parentPath+`"
  `+tt.includeBody+`
}

exclude {
  if      = true
  actions = ["destroy"]
  no_run  = true
}
`), 0644))

			ctx, pctx := newTestParsingContext(t, childPath)

			l := logger.CreateLogger()

			parsed, err := config.ParseConfigFile(ctx, pctx, l, childPath, nil)
			require.NoError(t, err)
			require.NotNil(t, parsed)

			require.NotNil(t, parsed.Exclude)
			assert.Equal(t, []string{"destroy"}, parsed.Exclude.Actions)
			require.NotNil(t, parsed.Exclude.NoRun)
			assert.True(t, *parsed.Exclude.NoRun)
		})
	}
}
