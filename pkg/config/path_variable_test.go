package config_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

func TestPathVariableInUnitIncludeAndRead(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "live", "app")

	inputs := parsePathUnit(t, v, unitDir)

	assert.Equal(t, map[string]any{
		"unit":         unitDir,
		"include":      "not-included",
		"read":         "not-read",
		"stack":        "no-stack",
		"env_read":     filepath.Join(root, "env.hcl"),
		"env_unit":     unitDir,
		"env_include":  "not-included",
		"shared_read":  filepath.Join(root, "shared.hcl"),
		"root_unit":    unitDir,
		"root_include": filepath.Join(root, "root.hcl"),
		"root_local":   filepath.Join(root, "root.hcl"),
		"root_read":    "not-read",
	}, inputs)
}

func TestPathVariableIncludeOfAbsolutePathIsClean(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)

	inputs := parsePathUnit(t, v, filepath.Join(root, "live", "absolute"))

	assert.Equal(t, filepath.Join(root, "root.hcl"), inputs["root_include"])
	assert.Equal(t, filepath.Join(root, "root.hcl"), inputs["root_local"])
}

func TestPathVariableReadIsScopedToTheReadingUnit(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	written := time.Now().Add(-time.Hour)

	for _, name := range []string{"shared.hcl", "shared.txt", "env.hcl", "env.txt"} {
		require.NoError(t, v.FS.Chtimes(filepath.Join(root, name), written, written))
	}

	counting := &countingFS{FS: v.FS, opens: map[string]int{}}
	v = v.WithFS(counting)
	ctx := config.WithCaches(t.Context())
	sharedData := filepath.Join(root, "shared.txt")
	envData := filepath.Join(root, "env.txt")

	firstDir := filepath.Join(root, "cache", "one")
	firstPath := filepath.Join(firstDir, config.DefaultTerragruntConfigPath)
	firstInputs := parsePathInputs(ctx, t, v, firstPath)
	sharedOpens, envOpens := counting.opensOf(sharedData), counting.opensOf(envData)

	secondDir := filepath.Join(root, "cache", "two")
	secondPath := filepath.Join(secondDir, config.DefaultTerragruntConfigPath)
	secondInputs := parsePathInputs(ctx, t, v, secondPath)

	assert.Equal(t, firstDir, firstInputs["env_unit"])
	assert.Equal(t, secondDir, secondInputs["env_unit"])
	assert.Equal(t, firstInputs["shared_read"], secondInputs["shared_read"])
	assert.Positive(t, sharedOpens)
	assert.Equal(t, sharedOpens, counting.opensOf(sharedData))
	assert.Greater(t, counting.opensOf(envData), envOpens)
}

func TestPathVariableInNestedRead(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "nested-read", "app")

	inputs := parsePathUnit(t, v, unitDir)

	assert.Equal(t, map[string]any{
		"outer_read": filepath.Join(root, "nested-read", "outer.hcl"),
		"inner_read": filepath.Join(root, "nested-read", "inner.hcl"),
		"inner_unit": unitDir,
		"read":       "not-read",
	}, inputs)
}

func TestPathVariableModuleMatchesGetWorkingDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		unit string
	}{
		{name: "source in terraform block", unit: "remote"},
		{name: "no source", unit: "local"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			inputs := parsePathUnit(t, v, filepath.Join(root, "module", tc.unit))

			require.NotEmpty(t, inputs["working_dir"])
			assert.Equal(t, inputs["working_dir"], inputs["module"])
		})
	}
}

func TestPathVariableModuleInConfiguredDownloadDir(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		unit        string
		source      string
		option      string
		downloadDir string
	}{
		{
			name:        "download_dir in the unit",
			unit:        "download-dir",
			source:      "../modules//app",
			downloadDir: filepath.Join("module", "custom-cache"),
		},
		{
			name:        "download_dir in an included file",
			unit:        "download-dir-include",
			source:      ".",
			downloadDir: filepath.Join("module", "custom-cache"),
		},
		{
			name:        "download directory option overrides download_dir",
			unit:        "download-dir",
			source:      "../modules//app",
			option:      "option-cache",
			downloadDir: "option-cache",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			unitDir := filepath.Join(root, "module", tc.unit)
			cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
			l := logger.CreateLogger()

			ctx, pctx := newPathParsingContext(t, cfgPath)
			if tc.option != "" {
				pctx.DownloadDir = filepath.Join(root, tc.option)
			}

			cfg, err := new(config.Store).ResolveRunConfig(ctx, l, v, pctx, cfgPath)
			require.NoError(t, err)

			want, err := tf.NewSource(l, v.FS, tc.source, filepath.Join(root, tc.downloadDir), unitDir, false)
			require.NoError(t, err)

			assert.Equal(t, want.WorkingDir, cfg.Inputs["module"])
		})
	}
}

func TestPathVariableModuleInSharedStoreFollowsParsingContext(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		option      string
		source      string
		downloadDir string
	}{
		{name: "download directory", source: ".", downloadDir: "option-cache"},
		{
			name:        "source option",
			option:      "../modules//app",
			source:      "../modules//app",
			downloadDir: filepath.Join("module", "hook", ".terragrunt-cache"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			l := logger.CreateLogger()
			unitDir := filepath.Join(root, "module", "hook")
			cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

			ctx, pctx := newPathParsingContext(t, cfgPath)
			pctx = pctx.WithDecodeList(config.TerraformBlock)
			s := new(config.Store)

			_, err := s.ResolvePartialConfig(ctx, l, v, pctx, cfgPath)
			require.NoError(t, err)

			changed := pctx.Clone()
			changed.Source = tc.option
			changed.DownloadDir = filepath.Join(root, tc.downloadDir)

			cfg, err := s.ResolvePartialConfig(ctx, l, v, changed, cfgPath)
			require.NoError(t, err)

			want, err := tf.NewSource(l, v.FS, tc.source, changed.DownloadDir, unitDir, false)
			require.NoError(t, err)

			hooks := cfg.Terraform.GetBeforeHooks()
			require.Len(t, hooks, 2)
			assert.Equal(t, []string{"ls", want.WorkingDir}, hooks[0].Execute)
		})
	}
}

func TestPathVariableStackIsNotDefinedInUnitBesideStackFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	l := logger.CreateLogger()
	unitDir := filepath.Join(root, "module", "hook")
	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

	ctx, pctx := newPathParsingContext(t, cfgPath)
	pctx = pctx.WithDecodeList(config.TerraformBlock)
	s := new(config.Store)

	for range 2 {
		cfg, err := s.ResolvePartialConfig(ctx, l, v, pctx, cfgPath)
		require.NoError(t, err)

		hooks := cfg.Terraform.GetBeforeHooks()
		require.Len(t, hooks, 2)
		assert.Equal(t, []string{"ls", "no-stack"}, hooks[1].Execute)

		require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(unitDir, config.DefaultStackFile), nil, 0o644))
	}
}

func TestPathVariableModuleOfInPlaceUnit(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "module", "in-place")
	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

	ctx, pctx := newPathParsingContext(t, cfgPath)
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))

	cfg, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
	require.NoError(t, err)

	assert.Equal(t, unitDir, cfg.Inputs["module"])
}

func TestPathVariableModuleOfInPlaceUnitWithDependencyInTerraformBlock(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/fixture")
	appDir := filepath.Join(root, "app")

	result := resolveUnit(t, unitCase{
		cfgPath: filepath.Join(appDir, config.DefaultTerragruntConfigPath),
		newFS: memFS(root, map[string]string{
			"vpc/terragrunt.hcl": "",
			"vpc/main.tf":        "",
			"app/main.tf":        "",
			"app/terragrunt.hcl": `
dependency "vpc" {
  config_path = "../vpc"
}

terraform {
  no_dot_terragrunt_cache = true
  include_in_copy         = [dependency.vpc.outputs.id]
}

inputs = {
  module = path.download
}
`,
		}),
		configure: func(pctx *config.ParsingContext) {
			pctx.OriginalTerragruntConfigPath = pctx.TerragruntConfigPath

			require.NoError(t, pctx.Experiments.EnableExperiment(experiment.PathVariable))
			require.NoError(t, pctx.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))
		},
	})

	require.NoError(t, result.runConfigErr)
	require.NotNil(t, result.cfg)
	assert.Equal(t, appDir, result.cfg.Inputs["module"])
}

func TestPathVariableModuleFollowsTheMergeStrategyOfAnExposedInclude(t *testing.T) {
	t.Parallel()

	const (
		data = `
locals {
  label = "catalog"
}

dependency "vpc" {
  config_path = "{{root}}/vpc"
}

download_dir = dependency.vpc.outputs.id
`
		unit = `
include "data" {
  path           = "{{root}}/data.hcl"
  expose         = true
  merge_strategy = "%s"
}

inputs = {
  label  = include.data.locals.label
  module = path.download
}
`
	)

	testCases := []struct {
		name     string
		strategy string
		rejected bool
	}{
		{name: "no merge", strategy: "no_merge"},
		{name: "shallow merge", strategy: "shallow", rejected: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")

			result := resolveUnit(t, unitCase{
				cfgPath: filepath.Join(root, "app", config.DefaultTerragruntConfigPath),
				newFS: memFS(root, map[string]string{
					"vpc/terragrunt.hcl": "",
					"vpc/main.tf":        "",
					"app/main.tf":        "",
					"data.hcl":           data,
					"app/terragrunt.hcl": fmt.Sprintf(unit, tc.strategy),
				}),
				configure: func(pctx *config.ParsingContext) {
					pctx.OriginalTerragruntConfigPath = pctx.TerragruntConfigPath

					require.NoError(t, pctx.Experiments.EnableExperiment(experiment.PathVariable))
				},
			})

			if tc.rejected {
				var rejected config.ModuleDirReferencesDependencyError

				require.ErrorAs(t, result.runConfigErr, &rejected)

				return
			}

			require.NoError(t, result.runConfigErr)
			require.NotNil(t, result.cfg)
			assert.Equal(t, "catalog", result.cfg.Inputs["label"])
			assert.NotEmpty(t, result.cfg.Inputs["module"])
		})
	}
}

func TestPathVariableModuleEvaluatesTheSourceWithTheIAMRoleOfTheUnit(t *testing.T) {
	t.Parallel()

	const (
		role = `
iam_role               = "arn:aws:iam::222222222222:role/unit"
iam_web_identity_token = "unit-token"
`
		unit = `
terraform {
  source = "../modules/${get_aws_account_id()}"
}

inputs = {
  module = path.download
}
`
		include = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`
	)

	testCases := []struct {
		files map[string]string
		name  string
	}{
		{
			name: "role in the unit",
			files: map[string]string{
				"modules/111111111111/main.tf": "",
				"modules/222222222222/main.tf": "",
				"unit/terragrunt.hcl":          role + unit,
			},
		},
		{
			name: "role in an included file",
			files: map[string]string{
				"modules/111111111111/main.tf": "",
				"modules/222222222222/main.tf": "",
				"root.hcl":                     role,
				"unit/terragrunt.hcl":          include + unit,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := vfs.MemRoot
			unitDir := filepath.Join(root, "unit")
			cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
			l := logger.CreateLogger()

			sts, _ := memSTS()
			v := venv.MemVenv().
				WithFS(memFS(root, tc.files)(t)).
				WithHTTP(sts).
				WithEnv(map[string]string{"AWS_REGION": "us-east-1"})

			ctx, pctx := newPathParsingContext(t, cfgPath)
			pctx.IAMRoleOptions = iam.RoleOptions{
				RoleARN:          "arn:aws:iam::111111111111:role/run",
				WebIdentityToken: "run-token",
			}

			cfg, err := new(config.Store).ResolveRunConfig(ctx, l, v, pctx, cfgPath)
			require.NoError(t, err)

			want, err := tf.NewSource(l, v.FS, "../modules/222222222222", pctx.DownloadDir, unitDir, false)
			require.NoError(t, err)

			assert.Equal(t, want.WorkingDir, cfg.Inputs["module"])
		})
	}
}

func TestPathVariableModuleOfRejectedInPlaceUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		option     string
		source     string
		experiment bool
	}{
		{name: "experiment disabled", source: "."},
		{name: "source option", option: "../modules//app", source: "../modules//app", experiment: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			unitDir := filepath.Join(root, "module", "in-place")
			cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
			l := logger.CreateLogger()

			ctx, pctx := newPathParsingContext(t, cfgPath)
			pctx.Source = tc.option

			if tc.experiment {
				require.NoError(t, pctx.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))
			}

			cfg, err := new(config.Store).ResolveRunConfig(ctx, l, v, pctx, cfgPath)
			require.NoError(t, err)

			want, err := tf.NewSource(l, v.FS, tc.source, pctx.DownloadDir, unitDir, false)
			require.NoError(t, err)

			assert.Equal(t, want.WorkingDir, cfg.Inputs["module"])
		})
	}
}

func TestPathVariableModuleRejectsDependencyReference(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		unit      string
		config    string
		attribute string
	}{
		{
			name:      "download_dir",
			unit:      "download-dir",
			config:    config.DefaultTerragruntConfigPath,
			attribute: "download_dir",
		},
		{
			name:      "no_dot_terragrunt_cache",
			unit:      "in-place",
			config:    config.DefaultTerragruntConfigPath,
			attribute: "terraform.no_dot_terragrunt_cache",
		},
		{
			name:      "download_dir in JSON",
			unit:      "download-dir-json",
			config:    config.DefaultTerragruntJSONConfigPath,
			attribute: "download_dir",
		},
		{
			name:      "no_dot_terragrunt_cache in JSON",
			unit:      "in-place-json",
			config:    config.DefaultTerragruntJSONConfigPath,
			attribute: "terraform.no_dot_terragrunt_cache",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			cfgPath := filepath.Join(root, "module", "dependency", tc.unit, tc.config)
			ctx, pctx := newPathParsingContext(t, cfgPath)

			_, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)

			var got config.ModuleDirReferencesDependencyError

			require.ErrorAs(t, err, &got)
			assert.Equal(t, tc.attribute, got.Attribute)
			assert.Equal(t, cfgPath, got.ConfigPath)
		})
	}
}

func TestPathVariableModuleRejectsDependencyReferenceInJSONSource(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	cfgPath := filepath.Join(root, "module", "dependency", "source-json", config.DefaultTerragruntJSONConfigPath)
	ctx, pctx := newPathParsingContext(t, cfgPath)

	_, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)

	var got config.TerraformSourceReferencesDependencyError

	require.ErrorAs(t, err, &got)
	assert.Equal(t, cfgPath, got.ConfigPath)
}

func TestPathVariableAsWholeObject(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "module", "local")

	inputs := parsePathUnit(t, v, unitDir)

	require.NotEmpty(t, inputs["working_dir"])
	assert.Equal(t, map[string]any{"unit": unitDir, "download": inputs["working_dir"]}, inputs["whole"])
}

func TestPathVariableInValuesFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "values", "app")

	inputs := parsePathUnit(t, v, unitDir)

	assert.Equal(t, map[string]any{"from_values": unitDir}, inputs)
}

func TestPathVariableInJSONConfig(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "live", "json")

	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntJSONConfigPath)

	inputs := parsePathInputs(config.WithCaches(t.Context()), t, v, cfgPath)

	assert.Equal(t, map[string]any{"unit": unitDir, "indexed": unitDir}, inputs)
}

func TestPathVariableStackIsNotDefinedInUnitBelowStackFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "stack", "live", "manual")

	inputs := parsePathUnit(t, v, unitDir)

	assert.Equal(t, map[string]any{"unit": unitDir, "stack": "no-stack"}, inputs)
}

func TestPathVariableInStackFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack", "live")
	stackPath := filepath.Join(stackDir, config.DefaultStackFile)

	ctx, pctx := newPathParsingContext(t, stackPath)

	stack, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, new(config.Store), pctx, stackPath)
	require.NoError(t, err)
	require.Len(t, stack.Units, 1)
	require.NotNil(t, stack.Units[0].Values)

	values, err := ctyhelper.ParseCtyValueToMap(*stack.Units[0].Values)
	require.NoError(t, err)

	assert.Equal(t, map[string]any{
		"stack_dir":       stackDir,
		"unit_in_stack":   "no-unit",
		"module_in_stack": "no-module",
	}, values)
}

func TestPathVariableInFileReadByStackFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-read", "live")
	stackPath := filepath.Join(stackDir, config.DefaultStackFile)

	ctx, pctx := newPathParsingContext(t, stackPath)

	stack, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, new(config.Store), pctx, stackPath)
	require.NoError(t, err)
	require.Len(t, stack.Units, 1)
	require.NotNil(t, stack.Units[0].Values)

	values, err := ctyhelper.ParseCtyValueToMap(*stack.Units[0].Values)
	require.NoError(t, err)

	assert.Equal(t, stackReadValues(stackDir), values)
}

func TestPathVariableInFileReadByStackFileThatUnitReads(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-read", "live")

	inputs := parsePathUnit(t, v, filepath.Join(root, "stack-read", "consumer"))

	assert.Equal(t, stackReadValues(stackDir), inputs)
}

func TestPathVariableInFileReadByGeneratedStack(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-read", "live")

	require.NoError(t, generatePathStack(t, v, stackDir, experiment.PathVariable))

	want := stackReadValues(stackDir)
	want["auto_read_stack"] = stackDir
	want["auto_read_unit"] = "no-unit"

	assert.Equal(t, want, parsePathUnit(t, v, filepath.Join(stackDir, config.StackDir, "app")))
}

func TestPathVariableInDependencyThatIsTheOriginalConfigOfStackRun(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-read", "live")
	original := filepath.Join(root, "stack-read", "base", config.DefaultTerragruntConfigPath)

	require.NoError(t, generatePathStackFrom(t, v, stackDir, original, experiment.PathVariable))

	want := stackReadValues(stackDir)
	want["auto_read_stack"] = stackDir
	want["auto_read_unit"] = "no-unit"

	assert.Equal(t, want, parsePathUnit(t, v, filepath.Join(stackDir, config.StackDir, "app")))
}

func TestPathVariableStackBesideStackBlocks(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		want map[string]string
		name string
		dir  string
	}{
		{
			name: "stack block with another name",
			dir:  "live",
			want: map[string]string{"stack_dir": "", "net_dir": filepath.Join(config.StackDir, "net")},
		},
		{
			name: "stack block with the name path",
			dir:  "named",
			want: map[string]string{"named_dir": filepath.Join(config.StackDir, "named"), "stack_dir": ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			stackDir := filepath.Join(root, "stack-refs", tc.dir)
			stackPath := filepath.Join(stackDir, config.DefaultStackFile)

			ctx, pctx := newPathParsingContext(t, stackPath)

			stack, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, new(config.Store), pctx, stackPath)
			require.NoError(t, err)
			require.Len(t, stack.Units, 1)
			require.NotNil(t, stack.Units[0].Values)

			values, err := ctyhelper.ParseCtyValueToMap(*stack.Units[0].Values)
			require.NoError(t, err)

			want := map[string]any{}
			for name, dir := range tc.want {
				want[name] = filepath.Join(stackDir, dir)
			}

			assert.Equal(t, want, values)
		})
	}
}

func TestPathVariableUnitInStackLocalsNamesTheExperimentOnlyWhenDisabled(t *testing.T) {
	t.Parallel()

	const stackFile = `
locals {
  unit_dir = path.unit
}

unit "app" {
  source = "../units/app"
  path   = "app"
}
`

	testCases := []struct {
		name        string
		experiments []string
		hint        bool
	}{
		{name: "experiment disabled", hint: true},
		{name: "experiment enabled", experiments: []string{experiment.PathVariable}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/fixture")
			stackPath := filepath.Join(root, config.DefaultStackFile)
			v := venv.MemVenv().WithFS(memFS(root, map[string]string{config.DefaultStackFile: stackFile})(t))

			ctx, pctx := newTestParsingContext(t, stackPath)

			for _, name := range tc.experiments {
				require.NoError(t, pctx.Experiments.EnableExperiment(name))
			}

			_, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, new(config.Store), pctx, stackPath)

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			assert.Equal(t, tc.hint, namesPathExperiment(diags))
		})
	}
}

func TestPathVariableInStackFileReadFromUnit(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		stackDir string
	}{
		{name: "stack locals", stackDir: filepath.Join("stack", "live")},
		{name: "stack values file", stackDir: "stack-values"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			stackDir := filepath.Join(root, tc.stackDir)
			stackPath := filepath.Join(stackDir, config.DefaultStackFile)

			ctx, pctx := unitPathParsingContext(t, v, filepath.Join(root, "stack", "consumer"))
			l := logger.CreateLogger()

			stack, err := config.ReadStackConfigFile(ctx, l, v, new(config.Store), pctx, stackPath)
			require.NoError(t, err)
			require.Len(t, stack.Units, 1)
			require.NotNil(t, stack.Units[0].Values)

			unitValues, err := ctyhelper.ParseCtyValueToMap(*stack.Units[0].Values)
			require.NoError(t, err)

			assert.Equal(t, map[string]any{
				"stack_dir":       stackDir,
				"unit_in_stack":   "no-unit",
				"module_in_stack": "no-module",
			}, unitValues)
		})
	}
}

func TestPathVariableInStackValuesFileReadByUnit(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-values")

	inputs := parsePathUnit(t, v, filepath.Join(stackDir, "reader"))

	assert.Equal(t, map[string]any{
		"stack_dir":     stackDir,
		"unit_in_stack": "no-unit",
		"read_file":     filepath.Join(stackDir, "terragrunt.values.hcl"),
	}, inputs)
}

func TestPathVariableInAutoIncludeFile(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	unitDir := filepath.Join(root, "autoinclude", "unit")

	inputs := parsePathUnit(t, v, unitDir)

	require.NotEmpty(t, inputs["working_dir"])
	assert.Equal(t, map[string]any{
		"unit":        unitDir,
		"working_dir": inputs["working_dir"],
		"auto_unit":   unitDir,
		"auto_module": inputs["working_dir"],
		"auto_stack":  "no-stack",
	}, inputs)
}

func TestPathVariableInGeneratedAutoInclude(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "autoinclude", "live")
	unitDir := filepath.Join(stackDir, config.StackDir, "app")

	require.NoError(t, generatePathStack(t, v, stackDir, experiment.PathVariable))

	inputs := parsePathUnit(t, v, unitDir)

	require.NotEmpty(t, inputs["working_dir"])
	assert.Equal(t, map[string]any{
		"unit":        unitDir,
		"working_dir": inputs["working_dir"],
		"auto_stack":  stackDir,
		"auto_unit":   "no-unit",
		"auto_label":  "live:" + stackDir,
		"auto_loop":   []any{stackDir + ":" + stackDir},
		"db_consumer": stackDir,
	}, inputs)
}

func TestPathVariableInAutoIncludeBlockRequiresExperiment(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)

	err := generatePathStack(t, v, filepath.Join(root, "autoinclude", "live"))

	var unresolved inthclparse.PartialEvalUnresolvedError

	require.ErrorAs(t, err, &unresolved)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	assert.True(t, namesPathExperiment(diags))
}

func TestPathVariableInAutoIncludeDependencyConfigPath(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "autoinclude-dependency", "live")

	require.NoError(t, generatePathStack(t, v, stackDir, experiment.PathVariable, experiment.StackDependencies))

	paths, err := inthclparse.AutoIncludeDependencyPaths(v.FS, filepath.Join(stackDir, config.StackDir, "app"))
	require.NoError(t, err)

	assert.Equal(t, []string{filepath.Join(stackDir, config.StackDir, "db")}, paths)
}

func TestPathVariableInAutoIncludeDependencyConfigPathRequiresExperiment(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)

	err := generatePathStack(
		t,
		v,
		filepath.Join(root, "autoinclude-dependency", "live"),
		experiment.StackDependencies,
	)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
	assert.True(t, namesPathExperiment(diags))
}

func TestPathVariableInStackInclude(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-include", "live")
	catalogDir := filepath.Join(root, "stack-include", "catalog")
	stackPath := filepath.Join(stackDir, config.DefaultStackFile)

	ctx, pctx := newPathParsingContext(t, stackPath)

	stack, err := config.ReadStackConfigFile(ctx, logger.CreateLogger(), v, new(config.Store), pctx, stackPath)
	require.NoError(t, err)
	require.Len(t, stack.Units, 2)

	units := map[string]*config.Unit{}
	values := map[string]map[string]any{}

	for _, unit := range stack.Units {
		require.NotNil(t, unit.Values)

		units[unit.Name] = unit
		values[unit.Name], err = ctyhelper.ParseCtyValueToMap(*unit.Values)
		require.NoError(t, err)
	}

	require.Contains(t, units, "app")
	assert.Equal(t, filepath.ToSlash(filepath.Join(catalogDir, "units", "app")), filepath.ToSlash(units["app"].Source))
	assert.Equal(t, "catalog-app", units["app"].Path)
	assert.Equal(t, map[string]map[string]any{
		"root": {"include": "no-include", "stack_dir": stackDir},
		"app":  {"include": filepath.Join(catalogDir, "components.hcl"), "stack_dir": stackDir},
	}, values)
}

func TestPathVariableInGeneratedStackInclude(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	stackDir := filepath.Join(root, "stack-include", "live")
	includePath := filepath.Join(root, "stack-include", "catalog", "components.hcl")
	rootDir := filepath.Join(stackDir, config.StackDir, "root")
	appDir := filepath.Join(stackDir, config.StackDir, "catalog-app")

	require.NoError(t, generatePathStack(t, v, stackDir, experiment.PathVariable))

	assert.Equal(t, map[string]any{
		"unit":      rootDir,
		"include":   "no-include",
		"stack_dir": stackDir,
	}, parsePathUnit(t, v, rootDir))
	assert.Equal(t, map[string]any{
		"unit":         appDir,
		"include":      includePath,
		"stack_dir":    stackDir,
		"auto_include": includePath,
		"auto_index":   includePath,
		"auto_guard":   "components.hcl",
		"auto_label":   "components.hcl:live",
	}, parsePathUnit(t, v, appDir))
}

func TestPathVariableInStackFileEvalVariables(t *testing.T) {
	t.Parallel()

	stackDir := venvtest.Root("/repo/stack")

	testCases := []struct {
		want        map[string]cty.Value
		name        string
		experiments []string
	}{
		{name: "experiment disabled", want: map[string]cty.Value{}},
		{
			name:        "experiment enabled",
			experiments: []string{experiment.PathVariable},
			want: map[string]cty.Value{
				config.MetadataPath: cty.ObjectVal(map[string]cty.Value{"stack": cty.StringVal(stackDir)}),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stackPath := filepath.Join(stackDir, config.DefaultStackFile)
			ctx, pctx := newTestParsingContext(t, stackPath)

			for _, name := range tc.experiments {
				require.NoError(t, pctx.Experiments.EnableExperiment(name))
			}

			vars, err := config.TerragruntEvalVariables(ctx, logger.CreateLogger(), pctx, stackPath, nil)
			require.NoError(t, err)

			assert.Equal(t, tc.want, vars)
		})
	}
}

func TestPathVariableResolvesInEachFixtureUnit(t *testing.T) {
	t.Parallel()

	fixtures := loadFixtures(t, []string{"path-variable"}, map[string]struct{}{
		"path-variable/module/dependency":           {},
		"path-variable/stack/units":                 {},
		"path-variable/stack-read/units":            {},
		"path-variable/stack-read-dependency/units": {},
		"path-variable/stack-include/catalog/units": {},
	})
	require.NotEmpty(t, fixtures.configs)

	for _, fixture := range fixtures.configs {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			result := resolveUnit(t, unitCase{
				newFS:   fixtures.newFS,
				cfgPath: fixture.path,
				configure: func(pctx *config.ParsingContext) {
					pctx.SkipOutput = true
					pctx.OriginalTofuCommand = "plan"
					pctx.OriginalTerragruntConfigPath = fixture.path

					require.NoError(t, pctx.Experiments.EnableExperiment(experiment.PathVariable))
					require.NoError(t, pctx.Experiments.EnableExperiment(experiment.NoDotTerragruntCache))
				},
			})

			require.NoError(t, result.runConfigErr)
			require.NotNil(t, result.cfg)
			assert.NotEmpty(t, result.cfg.Inputs)
		})
	}
}

func TestPathVariableUnitOfDependencyConfig(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	l := logger.CreateLogger()
	consumerPath := filepath.Join(root, "live", "app", config.DefaultTerragruntConfigPath)
	dependencyDir := filepath.Join(root, "live", "db")
	dependencyPath := filepath.Join(dependencyDir, config.DefaultTerragruntConfigPath)

	ctx, pctx := newPathParsingContext(t, consumerPath)

	l, pctx, err := pctx.WithDependencyConfigPath(l, dependencyPath)
	require.NoError(t, err)

	cfg, err := new(config.Store).ResolveRunConfig(ctx, l, v, pctx, dependencyPath)
	require.NoError(t, err)

	assert.Equal(t, dependencyDir, cfg.Inputs["unit"])
	assert.Equal(t, dependencyDir, cfg.Inputs["env_unit"])
}

func TestPathVariableRequiresExperiment(t *testing.T) {
	t.Parallel()

	v, root := pathFixture(t)
	cfgPath := filepath.Join(root, "values", "app", config.DefaultTerragruntConfigPath)

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath

	_, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)

	var diags hcl.Diagnostics

	require.ErrorAs(t, err, &diags)
}

func TestPathVariableErrorNamesTheExperimentWhenDisabled(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		config string
	}{
		{name: "locals", config: filepath.Join("module", "local", config.DefaultTerragruntConfigPath)},
		{name: "inputs", config: filepath.Join("module", "remote", config.DefaultTerragruntConfigPath)},
		{name: "values file", config: filepath.Join("values", "app", config.DefaultTerragruntConfigPath)},
		{name: "JSON config", config: filepath.Join("live", "json", config.DefaultTerragruntJSONConfigPath)},
		{name: "path.unit in locals", config: filepath.Join("all", "app", config.DefaultTerragruntConfigPath)},
		{
			name:   "unit below a stack file",
			config: filepath.Join("stack", "live", "manual", config.DefaultTerragruntConfigPath),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, root := pathFixture(t)
			cfgPath := filepath.Join(root, tc.config)

			ctx, pctx := newTestParsingContext(t, cfgPath)
			pctx.OriginalTerragruntConfigPath = cfgPath

			_, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)

			var diags hcl.Diagnostics

			require.ErrorAs(t, err, &diags)
			assert.True(t, namesPathExperiment(diags))
		})
	}
}

// pathFixture mirrors the path-variable fixture into memory and returns the venv and the fixture root.
func pathFixture(t *testing.T) (*venv.Venv, string) {
	t.Helper()

	fsys, root := venvtest.LoadFS(t, filepath.Join("..", "..", "test", "fixtures", "path-variable"))

	return venv.MemVenv().WithFS(fsys), root
}

// newPathParsingContext returns a parsing context for the unit at cfgPath with the path-variable experiment enabled.
func newPathParsingContext(t *testing.T, cfgPath string) (context.Context, *config.ParsingContext) {
	t.Helper()

	ctx, pctx := newTestParsingContext(t, cfgPath)
	pctx.OriginalTerragruntConfigPath = cfgPath
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.PathVariable))

	return ctx, pctx
}

// unitPathParsingContext returns the parsing context of the unit in unitDir with its `path` value set.
func unitPathParsingContext(t *testing.T, v *venv.Venv, unitDir string) (context.Context, *config.ParsingContext) {
	t.Helper()

	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newPathParsingContext(t, cfgPath)
	l := logger.CreateLogger()

	file, err := pctx.NewParser(l, v).ParseFromFile(v.FS, cfgPath)
	require.NoError(t, err)

	pctx.Path, err = config.PathVariable(ctx, l, v, new(config.Store), pctx, file, nil)
	require.NoError(t, err)
	require.NotNil(t, pctx.Path)

	return ctx, pctx
}

// generatePathStack generates the stack in stackDir with the named experiments enabled.
func generatePathStack(t *testing.T, v *venv.Venv, stackDir string, experiments ...string) error {
	t.Helper()

	return generatePathStackFrom(t, v, stackDir, "", experiments...)
}

// generatePathStackFrom generates the stack in stackDir in a run that starts from the config at originalPath.
func generatePathStackFrom(t *testing.T, v *venv.Venv, stackDir, originalPath string, experiments ...string) error {
	t.Helper()

	stackPath := filepath.Join(stackDir, config.DefaultStackFile)
	ctx, pctx := newTestParsingContext(t, stackPath)
	pctx.TerragruntStackConfigPath = stackPath
	pctx.NoCAS = true

	if originalPath != "" {
		pctx.OriginalTerragruntConfigPath = originalPath
	}

	for _, name := range experiments {
		require.NoError(t, pctx.Experiments.EnableExperiment(name))
	}

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	if err := config.GenerateStackFile(
		ctx,
		logger.CreateLogger(),
		v,
		new(config.Store),
		pctx,
		pool,
		stackPath,
		func(config.TryDefault) {},
	); err != nil {
		return err
	}

	return pool.Wait()
}

// parsePathInputs parses the unit at cfgPath with the path-variable experiment enabled and returns its inputs.
func parsePathInputs(ctx context.Context, t *testing.T, v *venv.Venv, cfgPath string) map[string]any {
	t.Helper()

	_, pctx := newPathParsingContext(t, cfgPath)

	cfg, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
	require.NoError(t, err)

	return cfg.Inputs
}

// parsePathUnit parses the unit in unitDir under its own caches and returns its inputs.
func parsePathUnit(t *testing.T, v *venv.Venv, unitDir string) map[string]any {
	t.Helper()

	cfgPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

	return parsePathInputs(config.WithCaches(t.Context()), t, v, cfgPath)
}

// stackReadValues returns the values that the stack-read fixture gives to its unit for the stack in stackDir.
func stackReadValues(stackDir string) map[string]any {
	return map[string]any{
		"read_stack":      stackDir,
		"read_unit":       "no-unit",
		"read_download":   "no-download",
		"nested_stack":    stackDir,
		"dependent_stack": stackDir,
		"dependent_base":  "mock",
	}
}

// namesPathExperiment reports whether a diagnostic tells how to enable the path-variable experiment.
func namesPathExperiment(diags hcl.Diagnostics) bool {
	return slices.ContainsFunc(diags, func(diag *hcl.Diagnostic) bool {
		return strings.Contains(diag.Detail, "--experiment "+experiment.PathVariable)
	})
}
