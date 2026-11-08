package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestUnitExcludeReplacesIncludedExcludeInBothParses(t *testing.T) {
	t.Parallel()

	cfgPath := writeUnitWithRoot(
		t,
		`
exclude {
  if      = true
  actions = ["plan"]
}
`,
		`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

exclude {
  if      = true
  actions = []
}
`,
	)

	for name, cfg := range parseBothWays(t, cfgPath) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.NotNil(t, cfg.Exclude)
			assert.Empty(t, cfg.Exclude.Actions)
			assert.False(t, cfg.Exclude.ShouldPreventRun("plan"))
		})
	}
}

func TestIncludedExcludeAppliesInBothParsesWhenTheUnitHasNone(t *testing.T) {
	t.Parallel()

	cfgPath := writeUnitWithRoot(
		t,
		`
exclude {
  if      = true
  actions = ["plan"]
}
`,
		`
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
	)

	for name, cfg := range parseBothWays(t, cfgPath) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.NotNil(t, cfg.Exclude)
			assert.True(t, cfg.Exclude.ShouldPreventRun("plan"))
		})
	}
}

func TestNullConfigPathIsRejectedInBothParses(t *testing.T) {
	t.Parallel()

	const (
		include = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`
		nullPath = `
dependency "db" {
  config_path = null
}
`
		realPath = `
dependency "db" {
  config_path  = "${get_terragrunt_dir()}/../db"
  skip_outputs = true
}
`
	)

	tests := []struct {
		name         string
		root         string
		unit         string
		declaredFile string
	}{
		{
			name:         "unit block",
			unit:         nullPath,
			declaredFile: config.DefaultTerragruntConfigPath,
		},
		{
			name:         "unit block over an included path",
			root:         realPath,
			unit:         include + nullPath,
			declaredFile: config.DefaultTerragruntConfigPath,
		},
		{
			name:         "included block",
			root:         nullPath,
			unit:         include,
			declaredFile: "root.hcl",
		},
		{
			name:         "included block under a unit path",
			root:         nullPath,
			unit:         include + realPath,
			declaredFile: "root.hcl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfgPath := writeUnitWithRoot(t, tt.root, tt.unit)

			for name, parse := range bothParses() {
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					_, err := parse(t, cfgPath)

					var nullErr config.DependencyNullConfigPathError

					require.ErrorAs(t, err, &nullErr)
					assert.Equal(t, "db", nullErr.Address)
					assert.Equal(t, tt.declaredFile, filepath.Base(nullErr.ConfigPath))
				})
			}
		})
	}
}

func TestNullConfigPathComputedFromAConditionIsRejectedInBothParses(t *testing.T) {
	t.Parallel()

	cfgPath := writeUnitWithRoot(t, "", `
locals {
  use_db = false
}

dependency "db" {
  config_path = local.use_db ? "../db" : null
}
`)

	for name, parse := range bothParses() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := parse(t, cfgPath)

			require.ErrorAs(t, err, new(config.DependencyNullConfigPathError))
		})
	}
}

func TestDisabledDependencyWithNullConfigPathKeepsItsBlockInBothParses(t *testing.T) {
	t.Parallel()

	cfgPath := writeUnitWithRoot(t, "", `
dependency "db" {
  config_path  = null
  enabled      = false
  mock_outputs = { id = "mock" }
}

inputs = {
  db_id = dependency.db.outputs.id
}
`)

	configs := parseBothWays(t, cfgPath)

	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Len(t, cfg.TerragruntDependencies, 1)
			require.NotNil(t, cfg.TerragruntDependencies[0].Enabled)
			assert.False(t, *cfg.TerragruntDependencies[0].Enabled)
			assert.Empty(t, modulePathBases(cfg))
		})
	}

	assert.Equal(t, "mock", configs["full"].Inputs["db_id"])
}

func TestUnitDependencyBlockReplacesIncludedPathInBothParses(t *testing.T) {
	t.Parallel()

	const includedBlock = `
dependency "db" {
  config_path  = "${get_terragrunt_dir()}/../db"
  skip_outputs = true
}
`

	tests := []struct {
		name      string
		unitBlock string
		wantPaths []string
	}{
		{
			name: "path override",
			unitBlock: `
dependency "db" {
  config_path  = "../other"
  skip_outputs = true
}
`,
			wantPaths: []string{"other"},
		},
		{
			name: "disabled with a path",
			unitBlock: `
dependency "db" {
  config_path = "../db"
  enabled     = false
}
`,
		},
		{
			name: "disabled with a null path",
			unitBlock: `
dependency "db" {
  config_path = null
  enabled     = false
}
`,
		},
	}

	for _, tt := range tests {
		for _, strategy := range []string{"shallow", "deep"} {
			t.Run(tt.name+" "+strategy, func(t *testing.T) {
				t.Parallel()

				cfgPath := writeUnitWithRoot(t, includedBlock, fmt.Sprintf(`
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = %q
}
%s`, strategy, tt.unitBlock))

				for name, cfg := range parseBothWays(t, cfgPath) {
					t.Run(name, func(t *testing.T) {
						t.Parallel()

						assert.ElementsMatch(t, tt.wantPaths, modulePathBases(cfg))
					})
				}
			})
		}
	}
}

func TestLiteralDependenciesPathSurvivesIncludeMergeInBothParses(t *testing.T) {
	t.Parallel()

	const included = `
dependencies {
  paths = ["${get_terragrunt_dir()}/../db"]
}
`

	for _, strategy := range []string{"shallow", "deep"} {
		t.Run(strategy, func(t *testing.T) {
			t.Parallel()

			cfgPath := writeUnitWithRoot(t, included, fmt.Sprintf(`
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = %q
}

dependency "other" {
  config_path  = "../other"
  skip_outputs = true
}
`, strategy))

			for name, cfg := range parseBothWays(t, cfgPath) {
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					assert.ElementsMatch(t, []string{"db", "other"}, modulePathBases(cfg))
				})
			}
		})
	}
}

func TestMissingConfigPathIsNotReportedAsNullWhenDiagnosticsAreIgnored(t *testing.T) {
	t.Parallel()

	cfgPath := writeUnitWithRoot(t, "", `
dependency "db" {
}
`)

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx = pctx.WithDecodeList(config.DependencyBlock).WithSkipOutputsResolution()
	pctx.SkipOutput = true
	pctx.Parser.IgnoreDiagnostics = true

	_, err := config.PartialParseConfigFile(ctx, logger.CreateLogger(), venvtest.NewWithOSFS(), pctx, cfgPath, nil)
	require.NoError(t, err)
}

// parseFunc returns the config at cfgPath from one kind of parse.
type parseFunc func(t *testing.T, cfgPath string) (*config.TerragruntConfig, error)

// bothParses returns the full and partial parses, by parse name.
func bothParses() map[string]parseFunc {
	return map[string]parseFunc{"full": parseFull, "partial": parsePartial}
}

// parseBothWays returns the config at cfgPath from each of [bothParses], by parse name.
func parseBothWays(t *testing.T, cfgPath string) map[string]*config.TerragruntConfig {
	t.Helper()

	configs := map[string]*config.TerragruntConfig{}

	for name, parse := range bothParses() {
		cfg, err := parse(t, cfgPath)
		require.NoError(t, err)

		configs[name] = cfg
	}

	return configs
}

// parseFull returns the config at cfgPath from a full parse.
func parseFull(t *testing.T, cfgPath string) (*config.TerragruntConfig, error) {
	t.Helper()

	ctx, pctx := newTestParsingContext(t, cfgPath)

	return config.ParseConfigFile(ctx, logger.CreateLogger(), venvtest.NewWithOSFS(), pctx, cfgPath, nil)
}

// parsePartial returns the config at cfgPath from a partial parse with discovery's decode list and settings.
func parsePartial(t *testing.T, cfgPath string) (*config.TerragruntConfig, error) {
	t.Helper()

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx = pctx.WithDecodeList(
		config.TerraformSource,
		config.DependenciesBlock,
		config.DependencyBlock,
		config.TerragruntFlags,
		config.FeatureFlagsBlock,
		config.ExcludeBlock,
		config.ErrorsBlock,
		config.TerragruntVersionConstraints,
		config.EngineBlock,
	).WithSkipOutputsResolution()
	pctx.SkipOutput = true

	return config.PartialParseConfigFile(ctx, logger.CreateLogger(), venvtest.NewWithOSFS(), pctx, cfgPath, nil)
}

// modulePathBases returns the final path element of each module dependency of cfg.
func modulePathBases(cfg *config.TerragruntConfig) []string {
	if cfg.Dependencies == nil {
		return nil
	}

	bases := make([]string, 0, len(cfg.Dependencies.Paths))
	for _, path := range cfg.Dependencies.Paths {
		bases = append(bases, filepath.Base(path))
	}

	return bases
}

// writeUnitWithRoot writes a root.hcl, a unit below it, and an empty db unit beside it, and returns the unit's config
// path.
func writeUnitWithRoot(t *testing.T, root, unit string) string {
	t.Helper()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "app", config.DefaultTerragruntConfigPath)

	files := map[string]string{
		filepath.Join(dir, "root.hcl"):                               root,
		filepath.Join(dir, "db", config.DefaultTerragruntConfigPath): "",
		cfgPath: unit,
	}

	for path, content := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	return cfgPath
}
