package config_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// valuesExperiment is whether a test parses with the values-block experiment on.
type valuesExperiment int

const (
	valuesExperimentOff valuesExperiment = iota
	valuesExperimentOn
)

const unitValuesBlock = `
values {
  value "instance_count" {
    type = number
  }

  value "tags" {
    type    = map(string)
    default = {}
  }

  value "settings" {
    type = object({
      name = string
      size = optional(number, 3)
    })
    default = { name = "app" }
  }
}
`

func TestParseConfigResolvesValuesBlock(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
	v := writeValuesFixture(t, root, map[string]string{
		"unit/terragrunt.hcl": unitValuesBlock + `
locals {
  name = values.settings.name
}

inputs = {
  instance_count = values.instance_count
  tags           = values.tags
  size           = values.settings.size
  name           = local.name
}
`,
		"unit/terragrunt.values.hcl": `instance_count = "2"`,
	})

	ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)

	cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
	require.NoError(t, err)

	assert.Equal(t, json.Number("2"), cfg.Inputs["instance_count"])
	assert.Equal(t, map[string]any{}, cfg.Inputs["tags"])
	assert.Equal(t, json.Number("3"), cfg.Inputs["size"])
	assert.Equal(t, "app", cfg.Inputs["name"])
}

func TestParseConfigValuesBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
	v := writeValuesFixture(t, root, map[string]string{
		"unit/terragrunt.hcl":        unitValuesBlock,
		"unit/terragrunt.values.hcl": `instance_count = 2`,
	})

	l := logger.CreateLogger()

	ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOff)

	_, err := config.ParseConfigFile(ctx, l, v, pctx, cfgPath, nil)

	var experimentErr config.ValuesBlockRequiresExperimentError

	require.ErrorAs(t, err, &experimentErr)
	assert.Equal(t, cfgPath, experimentErr.ConfigPath)

	ctx, pctx = newValuesTestContext(t, cfgPath, valuesExperimentOff)

	_, err = config.PartialParseConfigFile(ctx, l, v, pctx.WithDecodeList(config.TerragruntFlags), cfgPath, nil)
	require.ErrorAs(t, err, &experimentErr)
}

func TestParseConfigValuesBlockWithoutValuesFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		config  string
		missing string
	}{
		{
			name:    "required value",
			config:  unitValuesBlock,
			missing: `Missing required value "instance_count"`,
		},
		{
			name: "defaults only",
			config: `
values {
  value "tags" {
    type    = map(string)
    default = { team = "core" }
  }
}

inputs = {
  tags = values.tags
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := writeValuesFixture(t, root, map[string]string{"unit/terragrunt.hcl": tc.config})

			ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)

			cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)

			if tc.missing == "" {
				require.NoError(t, err)
				assert.Equal(t, map[string]any{"team": "core"}, cfg.Inputs["tags"])

				return
			}

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			require.Len(t, diags, 1)
			assert.Equal(t, tc.missing, diags[0].Summary)
			assert.Equal(t, cfgPath, diags[0].Subject.Filename)
			assert.Equal(t, 2, diags[0].Subject.Start.Line)
		})
	}
}

func TestParseConfigValuesBlockReportsAtValuesFile(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
	valuesPath := filepath.Join(root, "unit", "terragrunt.values.hcl")
	v := writeValuesFixture(t, root, map[string]string{
		"unit/terragrunt.hcl":        unitValuesBlock,
		"unit/terragrunt.values.hcl": "instance_count = 2\n\ntag = { team = \"core\" }\n",
	})

	ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)

	_, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)
	assert.Equal(t, `Undeclared value "tag"`, diags[0].Summary)
	assert.Contains(t, diags[0].Detail, `Did you mean "tags"?`)
	assert.Equal(t, valuesPath, diags[0].Subject.Filename)
	assert.Equal(t, 3, diags[0].Subject.Start.Line)
}

func TestParseConfigValuesBlockHandlesDiagnosticsThroughTheirFile(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
	valuesPath := filepath.Join(root, "unit", "terragrunt.values.hcl")
	v := writeValuesFixture(t, root, map[string]string{
		"unit/terragrunt.hcl":        unitValuesBlock,
		"unit/terragrunt.values.hcl": "instance_count = 2\ntag = {}\n",
	})

	handledBy := map[string]string{}

	ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)
	pctx = pctx.WithParserSettings(config.ParserSettings{
		DiagnosticsHandler: func(file *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
			for _, diag := range diags {
				handledBy[diag.Summary] = file.Body.MissingItemRange().Filename
			}

			return nil, nil
		},
	})

	_, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
	require.NoError(t, err)
	assert.Equal(t, valuesPath, handledBy[`Undeclared value "tag"`])
}

func TestReadValuesWithIgnoredDiagnosticsLeavesFailedValuesUnknown(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
	v := writeValuesFixture(t, root, map[string]string{
		"unit/terragrunt.hcl":        unitValuesBlock,
		"unit/terragrunt.values.hcl": `instance_count = "many"`,
	})

	l := logger.CreateLogger()

	ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)
	pctx.Parser.IgnoreDiagnostics = true

	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, cfgPath)
	require.NoError(t, err)

	values, err := config.ReadValues(ctx, l, v, pctx, file, config.ComponentConfig)
	require.NoError(t, err)
	require.NotNil(t, values)

	assert.False(t, values.GetAttr("instance_count").IsKnown())
	assert.Equal(t, cty.MapValEmpty(cty.String), values.GetAttr("tags"))
}

func TestParseConfigRejectsValuesBlockInIncludedFile(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files    map[string]string
		name     string
		declarer string
	}{
		{
			name: "include",
			files: map[string]string{
				"root.hcl": unitValuesBlock,
				"unit/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
			},
			declarer: "root.hcl",
		},
		{
			name: "autoinclude",
			files: map[string]string{
				"unit/terragrunt.hcl":             "",
				"unit/terragrunt.autoinclude.hcl": unitValuesBlock,
			},
			declarer: "unit/terragrunt.autoinclude.hcl",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := writeValuesFixture(t, root, tc.files)

			ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)

			_, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)

			var includeErr config.ValuesBlockInIncludeError

			require.ErrorAs(t, err, &includeErr)
			assert.Equal(t, filepath.Join(root, filepath.FromSlash(tc.declarer)), includeErr.ConfigPath)
		})
	}
}

func TestParseConfigWithoutValuesBlockIgnoresExperiment(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		exp  valuesExperiment
	}{
		{name: "experiment off", exp: valuesExperimentOff},
		{name: "experiment on", exp: valuesExperimentOn},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := writeValuesFixture(t, root, map[string]string{
				"unit/terragrunt.hcl":        `inputs = { tag = values.tag }`,
				"unit/terragrunt.values.hcl": `tag = "raw"`,
			})

			ctx, pctx := newValuesTestContext(t, cfgPath, tc.exp)

			cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
			require.NoError(t, err)
			assert.Equal(t, "raw", cfg.Inputs["tag"])
		})
	}
}

func TestParseConfigResolvesValuesBlockInJSON(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntJSONConfigPath)
	v := writeValuesFixture(t, root, map[string]string{
		"unit/terragrunt.hcl.json": `{
  "values": {
    "value": {
      "region": {"type": "string", "default": "us-east-1"},
      "zones": {"type": "list(string)"}
    }
  },
  "inputs": {
    "region": "${values.region}",
    "zones": "${values.zones}"
  }
}`,
		"unit/terragrunt.values.hcl": `zones = ["a"]`,
	})

	ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)

	cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", cfg.Inputs["region"])
	assert.Equal(t, []any{"a"}, cfg.Inputs["zones"])
}

func TestParseConfigResolvesValuesForFilesBesideTheUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		unit string
	}{
		{
			name: "include",
			unit: `
include "common" {
  path = "common.hcl"
}
`,
		},
		{
			name: "read_terragrunt_config",
			unit: `
locals {
  common = read_terragrunt_config("common.hcl")
}

inputs = local.common.inputs
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := writeValuesFixture(t, root, map[string]string{
				"unit/terragrunt.hcl": unitValuesBlock + tc.unit,
				"unit/common.hcl": `
inputs = {
  instance_count = values.instance_count
  size           = values.settings.size
}
`,
				"unit/terragrunt.values.hcl": `instance_count = "2"`,
			})

			ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)

			cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
			require.NoError(t, err)
			assert.Equal(t, json.Number("2"), cfg.Inputs["instance_count"])
			assert.Equal(t, json.Number("3"), cfg.Inputs["size"])
		})
	}
}

func TestParseConfigBesideUnitThatFailsToParseReadsValuesFile(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, "unit", "common.hcl")
	v := writeValuesFixture(t, root, map[string]string{
		"unit/terragrunt.hcl":        "broken hcl file",
		"unit/common.hcl":            "inputs = {\n  instance_count = values.instance_count\n}\n",
		"unit/terragrunt.values.hcl": `instance_count = "2"`,
	})

	ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)

	cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "2", cfg.Inputs["instance_count"])
}

func TestReadStackConfigFileResolvesValuesBlock(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	stackPath := filepath.Join(root, "stack", config.DefaultStackFile)
	v := writeValuesFixture(t, root, map[string]string{
		"stack/terragrunt.stack.hcl": `
values {
  value "env" {
    type = string
  }

  value "replicas" {
    type    = number
    default = 1
  }
}

locals {
  path = "${values.env}-app"
}

unit "app" {
  source = "../units/app"
  path   = local.path
  values = { replicas = values.replicas }
}
`,
		"stack/terragrunt.values.hcl": `env = "dev"`,
	})

	ctx, pctx := newValuesTestContext(t, stackPath, valuesExperimentOn)

	stackCfg, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, pctx, stackPath)
	require.NoError(t, err)

	want := cty.ObjectVal(map[string]cty.Value{"env": cty.StringVal("dev"), "replicas": cty.NumberIntVal(1)})

	require.NotNil(t, stackCfg.Values)
	assert.True(t, want.RawEquals(*stackCfg.Values))
	require.Len(t, stackCfg.Units, 1)
	assert.Equal(t, "dev-app", stackCfg.Units[0].Path)
	require.NotNil(t, stackCfg.Units[0].Values)
	assert.True(t, cty.NumberIntVal(1).RawEquals(stackCfg.Units[0].Values.GetAttr("replicas")))
}

func TestReadStackConfigFileResolvesValuesForFilesBesideTheStack(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	stackPath := filepath.Join(root, "stack", config.DefaultStackFile)
	v := writeValuesFixture(t, root, map[string]string{
		"stack/terragrunt.stack.hcl": `
values {
  value "env" {
    default = "dev"
  }
}

locals {
  common = read_terragrunt_config("common.hcl")
}

unit "app" {
  source = "../units/app"
  path   = local.common.locals.p
}
`,
		"stack/common.hcl": `locals { p = "app-${values.env}" }`,
	})

	ctx, pctx := newValuesTestContext(t, stackPath, valuesExperimentOn)

	stackCfg, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, pctx, stackPath)
	require.NoError(t, err)
	require.Len(t, stackCfg.Units, 1)
	assert.Equal(t, "app-dev", stackCfg.Units[0].Path)
}

func TestParseConfigResolvesValuesForFilesBesideAJSONUnit(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	commonPath := filepath.Join(root, "unit", "common.hcl")
	v := writeValuesFixture(t, root, map[string]string{
		"unit/terragrunt.hcl.json": `{"values": {"value": {"tag": {"default": "json"}}}}`,
		"unit/common.hcl":          `inputs = { tag = values.tag }`,
	})

	ctx, pctx := newValuesTestContext(t, commonPath, valuesExperimentOn)

	cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, commonPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "json", cfg.Inputs["tag"])
}

func TestReadStackConfigFileRejectsValuesBlockInIncludedFile(t *testing.T) {
	t.Parallel()

	const stackValuesBlock = `
values {
  value "env" {
    default = "dev"
  }
}
`

	const stackUnit = `
unit "app" {
  source = "../units/app"
  path   = "app"
}
`

	testCases := []struct {
		files    map[string]string
		name     string
		declarer string
	}{
		{
			name: "include",
			files: map[string]string{
				"stack/terragrunt.stack.hcl": `
include "common" {
  path = "common.stack.hcl"
}
` + stackUnit,
				"stack/common.stack.hcl": stackValuesBlock,
			},
			declarer: "stack/common.stack.hcl",
		},
		{
			name: "autoinclude",
			files: map[string]string{
				"stack/terragrunt.stack.hcl":             stackUnit,
				"stack/terragrunt.autoinclude.stack.hcl": stackValuesBlock,
			},
			declarer: "stack/terragrunt.autoinclude.stack.hcl",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			stackPath := filepath.Join(root, "stack", config.DefaultStackFile)
			v := writeValuesFixture(t, root, tc.files)

			ctx, pctx := newValuesTestContext(t, stackPath, valuesExperimentOn)

			_, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, pctx, stackPath)

			var includeErr config.ValuesBlockInIncludeError

			require.ErrorAs(t, err, &includeErr)
			assert.Equal(t, filepath.Join(root, filepath.FromSlash(tc.declarer)), includeErr.ConfigPath)
		})
	}
}

func TestReadStackConfigStringValuesBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	stackPath := filepath.Join(t.TempDir(), config.DefaultStackFile)
	ctx, pctx := newValuesTestContext(t, stackPath, valuesExperimentOff)

	_, err := config.ReadStackConfigString(ctx, logger.CreateLogger(), venvtest.NewWithOSFS(), pctx, stackPath, `
values {
  value "env" {}
}

unit "app" {
  source = "../units/app"
  path   = "app"
}
`)

	var experimentErr config.ValuesBlockRequiresExperimentError

	require.ErrorAs(t, err, &experimentErr)
	assert.Equal(t, stackPath, experimentErr.ConfigPath)
}

func TestReadTerragruntConfigOfStackResolvesValuesBlock(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
	v := writeValuesFixture(t, root, map[string]string{
		"stack/terragrunt.stack.hcl": `
values {
  value "env" {
    default = "dev"
  }
}

locals {
  env = values.env
}

unit "app" {
  source = "../units/app"
  path   = "app"
}
`,
		"unit/terragrunt.hcl": `
locals {
  stack = read_terragrunt_config("../stack/terragrunt.stack.hcl")
}

inputs = {
  env = local.stack.local.env
}
`,
	})

	ctx, pctx := newValuesTestContext(t, cfgPath, valuesExperimentOn)

	cfg, err := config.ParseConfigFile(ctx, logger.CreateLogger(), v, pctx, cfgPath, nil)
	require.NoError(t, err)
	assert.Equal(t, "dev", cfg.Inputs["env"])
}

// newValuesTestContext returns a parsing context for configPath, with the values-block experiment on when exp says so.
func newValuesTestContext(
	t *testing.T,
	configPath string,
	exp valuesExperiment,
) (context.Context, *config.ParsingContext) {
	t.Helper()

	ctx, pctx := newTestParsingContext(t, configPath)

	if exp == valuesExperimentOn {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.ValuesBlock))
	}

	return ctx, pctx
}

// writeValuesFixture returns an in-memory venv holding files, keyed by slash-separated paths under root.
func writeValuesFixture(t *testing.T, root string, files map[string]string) *venv.Venv {
	t.Helper()

	fsys := vfs.NewMemMapFS()

	for path, content := range files {
		require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, filepath.FromSlash(path)), []byte(content), 0o644))
	}

	return venv.MemVenv().WithFS(fsys)
}

// emptyUnitConfig returns an empty unit config at the default config path in dir, parsed with a parser from pctx.
func emptyUnitConfig(
	tb testing.TB,
	l log.Logger,
	v *venv.Venv,
	pctx *config.ParsingContext,
	dir string,
) *hclparse.File {
	tb.Helper()

	file, err := pctx.NewParser(l, v).ParseFromString("", filepath.Join(dir, config.DefaultTerragruntConfigPath))
	require.NoError(tb, err)

	return file
}
