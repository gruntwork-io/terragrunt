package config_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const enabledStackDir = "/virtual/stack"

const enabledUnitSource = "./units/app"

const enabledStackSource = "./stacks/team"

const unitWithEnabledHCL = `
unit "app" {
  enabled = false

  source = "` + enabledUnitSource + `"
  path   = "app"
}
`

const stackWithEnabledHCL = `
stack "team" {
  enabled = true

  source = "` + enabledStackSource + `"
  path   = "team"
}
`

// unitWithEnabledJSON is the JSON encoding of unitWithEnabledHCL. Stack files are only ever
// HCL, but an include block may point at a JSON file, so the gate has to read one.
const unitWithEnabledJSON = `{
  "unit": {
    "app": {
      "enabled": false,
      "source": "` + enabledUnitSource + `",
      "path": "app"
    }
  }
}`

// generatedStack is one in-memory generation run, so a test can ask what landed under
// .terragrunt-stack without touching disk.
type generatedStack struct {
	fs  vfs.FS
	dir string
}

// generated reports whether the named path exists under the generated stack directory.
func (gen generatedStack) generated(path ...string) bool {
	return vfs.Exists(gen.fs, filepath.Join(slices.Concat([]string{gen.dir}, path)...))
}

// generateEnabledStack writes stackHCL alongside a local unit source and a local nested
// stack source, then generates into an in-memory filesystem.
func generateEnabledStack(t *testing.T, stackHCL string) generatedStack {
	t.Helper()

	v := venvtest.New()

	for path, body := range map[string]string{
		filepath.Join("units", "app", "main.tf"):                                               "",
		filepath.Join("units", "app", config.DefaultTerragruntConfigPath):                      "",
		filepath.Join("stacks", "team", "units", "member", "main.tf"):                          "",
		filepath.Join("stacks", "team", "units", "member", config.DefaultTerragruntConfigPath): "",
		filepath.Join("stacks", "team", config.DefaultStackFile): `
unit "member" {
  source = "./units/member"
  path   = "member"
}
`,
		config.DefaultStackFile: stackHCL,
	} {
		require.NoError(
			t,
			vfs.WriteFile(v.FS, filepath.Join(enabledStackDir, path), []byte(body), 0o644),
		)
	}

	stackPath := filepath.Join(enabledStackDir, config.DefaultStackFile)

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, v, stackPath)
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.BlockIteration))

	pctx.TerragruntStackConfigPath = stackPath
	// CAS shells out to git, which the no-spawn venv refuses, and a local source has
	// nothing to clone anyway.
	pctx.NoCAS = true

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	require.NoError(t, config.GenerateStackFile(ctx, l, pctx, pool, stackPath))
	require.NoError(t, pool.Wait())

	return generatedStack{fs: v.FS, dir: filepath.Join(enabledStackDir, config.StackDir)}
}

func TestGenerateStackSkipsDisabledComponents(t *testing.T) {
	t.Parallel()

	gen := generateEnabledStack(t, `
unit "kept_unit" {
  source = "`+enabledUnitSource+`"
  path   = "kept-unit"
}

unit "dropped_unit" {
  enabled = false

  source = "`+enabledUnitSource+`"
  path   = "dropped-unit"
}

stack "kept_stack" {
  source = "`+enabledStackSource+`"
  path   = "kept-stack"
}

stack "dropped_stack" {
  enabled = false

  source = "`+enabledStackSource+`"
  path   = "dropped-stack"
}
`)

	assert.True(t, gen.generated("kept-unit", config.DefaultTerragruntConfigPath))
	assert.False(t, gen.generated("dropped-unit"))

	assert.True(t, gen.generated("kept-stack", config.DefaultStackFile))
	assert.False(t, gen.generated("dropped-stack"))
}

// TestGenerateStackDisabledEverywhereGeneratesNothing pins that a stack whose every
// component is disabled generates nothing and succeeds, rather than failing the way a
// stack file declaring no components at all does.
func TestGenerateStackDisabledEverywhereGeneratesNothing(t *testing.T) {
	t.Parallel()

	gen := generateEnabledStack(t, `
unit "app" {
  enabled = false

  source = "`+enabledUnitSource+`"
  path   = "app"
}

stack "team" {
  enabled = false

  source = "`+enabledStackSource+`"
  path   = "team"
}
`)

	assert.False(t, gen.generated("app"))
	assert.False(t, gen.generated("team"))
}

// TestGenerateStackEnabledKeepsBareAddress pins that a conditional unit generates at its
// declared path. The count = cond ? 1 : 0 workaround it replaces would put the unit under
// an index segment instead.
func TestGenerateStackEnabledKeepsBareAddress(t *testing.T) {
	t.Parallel()

	gen := generateEnabledStack(t, `
unit "vpc" {
  enabled = true

  source = "`+enabledUnitSource+`"
  path   = "vpc"
}
`)

	assert.True(t, gen.generated("vpc", config.DefaultTerragruntConfigPath))
	assert.False(t, gen.generated("vpc", "0"))
}

func TestGenerateStackEnabledGatesWholeExpandedSet(t *testing.T) {
	t.Parallel()

	gen := generateEnabledStack(t, `
unit "shard" {
  expansion {
    for_each = toset(["web", "api"])
  }

  enabled = false

  source = "`+enabledUnitSource+`"
  path   = "shard/${each.key}"
}
`)

	assert.False(t, gen.generated("shard"))
}

func TestGenerateStackEnabledResolvesPerExpandedElement(t *testing.T) {
	t.Parallel()

	gen := generateEnabledStack(t, `
unit "shard" {
  expansion {
    for_each = toset(["web", "api"])
  }

  enabled = each.key != "api"

  source = "`+enabledUnitSource+`"
  path   = "shard/${each.key}"
}
`)

	assert.True(t, gen.generated("shard", "web", config.DefaultTerragruntConfigPath))
	assert.False(t, gen.generated("shard", "api"))
}

// TestUnitAndStackDecodeEnabled pins that enabled reaches the decoded config rather than
// falling into the `hcl:",remain"` field that used to absorb it.
func TestUnitAndStackDecodeEnabled(t *testing.T) {
	t.Parallel()

	stackCfg, err := parseStackString(t, unitWithEnabledHCL+stackWithEnabledHCL)
	require.NoError(t, err)
	require.Len(t, stackCfg.Units, 1)
	require.Len(t, stackCfg.Stacks, 1)

	require.NotNil(t, stackCfg.Units[0].Enabled)
	assert.False(t, *stackCfg.Units[0].Enabled)

	require.NotNil(t, stackCfg.Stacks[0].Enabled)
	assert.True(t, *stackCfg.Stacks[0].Enabled)
}

// TestValidateBlockIterationExperimentGatesEnabled pins which block types reject a bare
// enabled attribute while the experiment is off. The dependency row expects no error,
// since that block has always accepted enabled.
func TestValidateBlockIterationExperimentGatesEnabled(t *testing.T) {
	t.Parallel()

	skipInExperimentMode(t)

	testCases := []struct {
		name          string
		configPath    string
		cfg           string
		wantBlockType string
		wantLabel     string
		wantErr       bool
	}{
		{
			name:          "unit",
			configPath:    config.DefaultStackFile,
			cfg:           unitWithEnabledHCL,
			wantBlockType: "unit",
			wantLabel:     "app",
			wantErr:       true,
		},
		{
			name:          "stack",
			configPath:    config.DefaultStackFile,
			cfg:           stackWithEnabledHCL,
			wantBlockType: "stack",
			wantLabel:     "team",
			wantErr:       true,
		},
		{
			name:          "unit in a json body",
			configPath:    "extra.json",
			cfg:           unitWithEnabledJSON,
			wantBlockType: "unit",
			wantLabel:     "app",
			wantErr:       true,
		},
		{
			name:       "dependency",
			configPath: config.DefaultTerragruntConfigPath,
			cfg: `
dependency "vpc" {
  enabled     = false
  config_path = "../vpc"
}
`,
		},
		{
			name:       "unit without enabled",
			configPath: config.DefaultStackFile,
			cfg: `
unit "app" {
  source = "./units/app"
  path   = "app"
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			file := parseHCLString(t, tc.cfg, tc.configPath)

			err := config.ValidateBlockIterationExperiment(experiment.NewExperiments(), file)

			if !tc.wantErr {
				require.NoError(t, err)

				return
			}

			var typed config.EnabledRequiresExperimentError
			require.ErrorAs(t, err, &typed)
			assert.Equal(t, tc.wantBlockType, typed.BlockType)
			assert.Equal(t, tc.wantLabel, typed.BlockLabel)
			assert.Equal(t, tc.configPath, typed.ConfigPath)
		})
	}
}

// TestReadStackConfigStringEnabledRequiresExperiment proves the gate is wired into the
// stack parse, not just callable on its own.
func TestReadStackConfigStringEnabledRequiresExperiment(t *testing.T) {
	t.Parallel()

	skipInExperimentMode(t)

	ctx, pctx := newTestParsingContext(t, venvtest.NewWithOSFS(), config.DefaultStackFile)

	_, err := config.ReadStackConfigString(
		ctx,
		logger.CreateLogger(),
		pctx,
		config.DefaultStackFile,
		unitWithEnabledHCL,
		nil,
	)

	var typed config.EnabledRequiresExperimentError
	require.ErrorAs(t, err, &typed)
	assert.Equal(t, "unit", typed.BlockType)
	assert.Equal(t, "app", typed.BlockLabel)
}

// TestEnabledRequiresExperimentErrorNamesTheFlag pins that the error a user reads names
// the experiment they need, with and without a block label.
func TestEnabledRequiresExperimentErrorNamesTheFlag(t *testing.T) {
	t.Parallel()

	labeled := config.EnabledRequiresExperimentError{
		ConfigPath: config.DefaultStackFile,
		BlockType:  "unit",
		BlockLabel: "app",
	}
	assert.Contains(t, labeled.Error(), experiment.BlockIteration)
	assert.Contains(t, labeled.Error(), `unit "app"`)

	unlabeled := config.EnabledRequiresExperimentError{
		ConfigPath: config.DefaultStackFile,
		BlockType:  "stack",
	}
	assert.Contains(t, unlabeled.Error(), experiment.BlockIteration)
	assert.NotContains(t, unlabeled.Error(), `""`)
}
