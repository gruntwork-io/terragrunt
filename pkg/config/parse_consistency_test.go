package config_test

import (
	"bytes"
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

var mergeStrategies = []string{"shallow", "deep", "no_merge"}

func TestUnitExcludeReplacesIncludedExcludeInBothParses(t *testing.T) {
	t.Parallel()

	for _, strategy := range mergeStrategies {
		t.Run(strategy, func(t *testing.T) {
			t.Parallel()

			cfgPath := writeUnitWithRoot(
				t,
				`
exclude {
  if      = true
  actions = ["plan"]
}
`,
				fmt.Sprintf(`
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = %q
}

exclude {
  if      = true
  actions = []
}
`, strategy),
			)

			for name, cfg := range parseBothWays(t, cfgPath) {
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					require.NotNil(t, cfg.Exclude)
					assert.Empty(t, cfg.Exclude.Actions)
					assert.False(t, cfg.Exclude.ShouldPreventRun("plan"))
				})
			}
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
			assert.False(t, dependencyEnabled(&cfg.TerragruntDependencies[0]))
			assert.Nil(t, cfg.Dependencies)
		})
	}

	assert.Equal(t, "mock", configs["full"].Inputs["db_id"])
}

func TestUnitDependencyBlockReplacesIncludedBlockInBothParses(t *testing.T) {
	t.Parallel()

	const includedBlock = `
dependency "db" {
  config_path  = "${get_terragrunt_dir()}/../db"
  skip_outputs = true
}
`

	tests := []struct {
		name        string
		unitBlock   string
		wantPath    string
		wantEnabled bool
	}{
		{
			name: "path override",
			unitBlock: `
dependency "db" {
  config_path  = "../other"
  skip_outputs = true
}
`,
			wantPath:    "other",
			wantEnabled: true,
		},
		{
			name: "disabled with a path",
			unitBlock: `
dependency "db" {
  config_path = "../db"
  enabled     = false
}
`,
			wantPath: "db",
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

						require.Len(t, cfg.TerragruntDependencies, 1)

						dep := cfg.TerragruntDependencies[0]
						assert.Equal(t, tt.wantEnabled, dependencyEnabled(&dep))

						if tt.wantPath != "" {
							assert.Equal(t, tt.wantPath, filepath.Base(dep.ConfigPath.AsString()))
						}

						assert.Nil(t, cfg.Dependencies)

						wantPaths := []string{}
						if tt.wantEnabled {
							wantPaths = []string{tt.wantPath}
						}

						assert.Equal(t, wantPaths, dependencyPathBases(t, cfg))
					})
				}
			})
		}
	}
}

func TestDeepMergedUnitBlockInheritsIncludedEnabledInBothParses(t *testing.T) {
	t.Parallel()

	cfgPath := writeUnitWithRoot(t, `
dependency "db" {
  config_path  = null
  enabled      = false
  mock_outputs = { id = "mock" }
}
`, `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "deep"
}

dependency "db" {
  config_path = "../db"
}
`)

	for name, cfg := range parseBothWays(t, cfgPath) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			require.Len(t, cfg.TerragruntDependencies, 1)
			assert.False(t, dependencyEnabled(&cfg.TerragruntDependencies[0]))
		})
	}
}

func TestLiteralDependenciesPathSurvivesIncludeMergeInBothParses(t *testing.T) {
	t.Parallel()

	const literalPath = `
dependencies {
  paths = ["${get_terragrunt_dir()}/../db"]
}
`

	tests := []struct {
		name string
		root string
		unit string
	}{
		{
			name: "beside a unit block",
			root: literalPath,
			unit: `
dependency "other" {
  config_path  = "../other"
  skip_outputs = true
}
`,
		},
		{
			name: "matching an included block the unit overrides",
			root: literalPath + `
dependency "db" {
  config_path  = "${get_terragrunt_dir()}/../db"
  skip_outputs = true
}
`,
			unit: `
dependency "db" {
  config_path  = "../other"
  skip_outputs = true
}
`,
		},
		{
			name: "matching an included block the unit disables",
			root: literalPath + `
dependency "db" {
  config_path  = "${get_terragrunt_dir()}/../db"
  skip_outputs = true
}
`,
			unit: `
dependency "db" {
  config_path = "../db"
  enabled     = false
}
`,
		},
	}

	for _, tt := range tests {
		for _, strategy := range []string{"shallow", "deep"} {
			t.Run(tt.name+" "+strategy, func(t *testing.T) {
				t.Parallel()

				cfgPath := writeUnitWithRoot(t, tt.root, fmt.Sprintf(`
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = %q
}
%s`, strategy, tt.unit))

				for name, cfg := range parseBothWays(t, cfgPath) {
					t.Run(name, func(t *testing.T) {
						t.Parallel()

						require.NotNil(t, cfg.Dependencies)
						require.Len(t, cfg.Dependencies.Paths, 1)
						assert.Equal(t, "db", filepath.Base(cfg.Dependencies.Paths[0]))
						assert.Contains(t, dependencyPathBases(t, cfg), "db")
					})
				}
			})
		}
	}
}

func TestRenderOmitsDependenciesBlockForIncludedDependencyBlock(t *testing.T) {
	t.Parallel()

	cfgPath := writeUnitWithRoot(t, `
dependency "db" {
  config_path  = "${get_terragrunt_dir()}/../db"
  skip_outputs = true
}
`, `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`)

	cfg, err := parseFull(t, cfgPath)
	require.NoError(t, err)

	var buf bytes.Buffer

	_, err = cfg.WriteTo(&buf)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `dependency "db"`)
	assert.NotContains(t, buf.String(), "dependencies {")

	cfgAsCty, err := config.TerragruntConfigAsCty(cfg)
	require.NoError(t, err)
	assert.True(t, cfgAsCty.GetAttr("dependencies").IsNull())
}

func TestMissingConfigPathIsNotReportedAsNullWhenDiagnosticsAreIgnored(t *testing.T) {
	t.Parallel()

	cfgPath := writeUnitWithRoot(t, "", `
dependency "db" {
}
`)

	t.Run("partial", func(t *testing.T) {
		t.Parallel()

		ctx, pctx := newTestParsingContext(t, cfgPath)
		pctx = pctx.WithDecodeList(config.DependencyBlock).WithSkipOutputsResolution()
		pctx.SkipOutput = true
		pctx.Parser.IgnoreDiagnostics = true

		cfg, err := config.PartialParseConfigFile(
			ctx,
			logger.CreateLogger(),
			venvtest.NewWithOSFS(),
			pctx,
			cfgPath,
			nil,
		)
		require.NoError(t, err)
		assert.Empty(t, cfg.TerragruntDependencies)
	})

	t.Run("full", func(t *testing.T) {
		t.Parallel()

		ctx, pctx := newTestParsingContext(t, cfgPath)
		pctx.SkipOutput = true
		pctx.Parser.IgnoreDiagnostics = true

		cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), venvtest.NewWithOSFS(), pctx, cfgPath, nil)
		require.NoError(t, err)
		assert.Empty(t, cfg.TerragruntDependencies)
	})
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

// dependencyEnabled reports whether dep is enabled, which it is unless it sets enabled to false.
func dependencyEnabled(dep *config.Dependency) bool {
	return dep.Enabled == nil || *dep.Enabled
}

// dependencyPathBases returns the final element of each path [config.TerragruntConfig.DependencyPaths] returns for
// cfg, and fails the test when a dependency block's config_path is unresolved.
func dependencyPathBases(t *testing.T, cfg *config.TerragruntConfig) []string {
	t.Helper()

	paths, unresolved := cfg.DependencyPaths()
	require.Empty(t, unresolved)

	bases := make([]string, 0, len(paths))
	for _, path := range paths {
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
