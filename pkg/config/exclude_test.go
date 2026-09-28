package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPartialParseExcludeWithWrongTypeErrors pins that an exclude block
// whose attributes do not decode fails the parse, where it used to vanish
// from the parsed config.
func TestPartialParseExcludeWithWrongTypeErrors(t *testing.T) {
	t.Parallel()

	cfgPath := writeExcludeUnit(t, `
exclude {
  if      = true
  actions = "plan"
}
`)

	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), cfgPath)
	pctx = pctx.WithDecodeList(config.ExcludeBlock)

	_, err := config.PartialParseConfigFile(ctx, pctx, logger.CreateLogger(), cfgPath, nil)
	require.ErrorAs(t, err, new(config.InvalidExcludeBlockError))
}

// TestPartialParseExcludeReadingDependencyOutputDuringDiscovery pins that
// discovery, which parses without dependency outputs, skips an exclude block
// that reads one and still parses the rest of the config.
func TestPartialParseExcludeReadingDependencyOutputDuringDiscovery(t *testing.T) {
	t.Parallel()

	cfgPath := writeExcludeUnit(t, `
dependency "dep" {
  config_path  = "../dep"
  mock_outputs = { flag = true }
}

exclude {
  if      = dependency.dep.outputs.flag
  actions = ["plan"]
}
`)

	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), cfgPath)
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.ExcludeBlock)
	pctx.SkipOutputsResolution = true

	parsed, err := config.PartialParseConfigFile(ctx, pctx, logger.CreateLogger(), cfgPath, nil)
	require.NoError(t, err)
	assert.Nil(t, parsed.Exclude)
	assert.Len(t, parsed.TerragruntDependencies, 1)
}

// writeExcludeUnit writes body as a unit config beside an empty dependency
// unit named dep, and returns the unit's config path.
func writeExcludeUnit(t *testing.T, body string) string {
	t.Helper()

	tmpDir := t.TempDir()

	depDir := filepath.Join(tmpDir, "dep")
	require.NoError(t, os.MkdirAll(depDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(depDir, config.DefaultTerragruntConfigPath), nil, 0644))

	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))

	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(cfgPath, []byte(body), 0644))

	return cfgPath
}

func TestExcludeConfig_ShouldPreventRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  string
		exclude config.ExcludeConfig
		want    bool
	}{
		{
			name: "output in actions with if=true prevents output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan", "apply", "destroy", "output"},
			},
			action: "output",
			want:   true,
		},
		{
			name: "output in actions with if=false does not prevent output",
			exclude: config.ExcludeConfig{
				If:      false,
				Actions: []string{"output"},
			},
			action: "output",
			want:   false,
		},
		{
			name: "output not in actions does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan", "apply"},
			},
			action: "output",
			want:   false,
		},
		{
			name: "all actions prevents output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"all"},
				NoRun:   new(true),
			},
			action: "output",
			want:   true,
		},
		{
			name: "all_except_output does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"all_except_output"},
				NoRun:   new(true),
			},
			action: "output",
			want:   false,
		},
		{
			name: "no_run=false never prevents",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"output"},
				NoRun:   new(false),
			},
			action: "output",
			want:   false,
		},
		{
			name: "empty actions does not prevent",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{},
			},
			action: "output",
			want:   false,
		},
		{
			name: "plan action does not prevent output",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"plan"},
			},
			action: "output",
			want:   false,
		},
		{
			name: "all actions with no_run nil uses legacy Contains",
			exclude: config.ExcludeConfig{
				If:      true,
				Actions: []string{"all"},
			},
			action: "output",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.exclude.ShouldPreventRun(tt.action)
			assert.Equal(t, tt.want, got)
		})
	}
}
