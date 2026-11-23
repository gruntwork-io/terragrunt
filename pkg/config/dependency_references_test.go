package config_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// dependencyReferenceCase is a unit with one part that reads `dependency` where no dependency has run.
type dependencyReferenceCase struct {
	assertErr     func(t *testing.T, err error)
	name          string
	part          string
	otherSections []config.PartialDecodeSectionType
	section       config.PartialDecodeSectionType
}

func dependencyReferenceCases() []dependencyReferenceCase {
	return []dependencyReferenceCase{
		{
			name:          "exclude",
			section:       config.ExcludeBlock,
			otherSections: versionCheckSections(),
			part: `
exclude {
  if      = dependency.dep.outputs.a == "x"
  actions = ["plan"]
}
`,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorAs(t, err, new(config.ExcludeReferencesDependencyError))
			},
		},
		{
			name:          "terraform source",
			section:       config.TerraformSource,
			otherSections: versionCheckSections(),
			part: `
terraform {
  source = "../${dependency.dep.outputs.a}"
}
`,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorAs(t, err, new(config.TerraformSourceReferencesDependencyError))
			},
		},
		{
			name:          "version attribute",
			section:       config.TerragruntVersionConstraints,
			otherSections: []config.PartialDecodeSectionType{config.TerraformSource},
			part: `
terraform_version_constraint = dependency.dep.outputs.a
`,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var diags hcl.Diagnostics
				require.ErrorAs(t, err, &diags)
				require.Len(t, diags, 1)
				assert.Equal(t, "Dependency reference not allowed", diags[0].Summary)
			},
		},
	}
}

// versionCheckSections is the decode list of the version check of a single run.
func versionCheckSections() []config.PartialDecodeSectionType {
	return []config.PartialDecodeSectionType{
		config.TerragruntVersionConstraints,
		config.FeatureFlagsBlock,
		config.EngineBlock,
	}
}

// TestResolvePartialConfigRejectsDependencyReferencesOnlyInDecodedSections pins that a partial parse rejects a part
// that reads `dependency` only when its decode list names that part, and that a parse which names it, or a full
// parse, still rejects it after the same Store resolved the file without it.
func TestResolvePartialConfigRejectsDependencyReferencesOnlyInDecodedSections(t *testing.T) {
	t.Parallel()

	for _, tt := range dependencyReferenceCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/live")
			cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
				filepath.Join("dep", config.DefaultTerragruntConfigPath): "",
				filepath.Join("unit", config.DefaultTerragruntConfigPath): `
dependency "dep" {
  config_path  = "../dep"
  mock_outputs = { a = "mod" }
}
` + tt.part,
			}))
			ctx, pctx := newTestParsingContext(t, cfgPath)
			l := logger.CreateLogger()
			s := new(config.Store)

			_, err := s.ResolvePartialConfig(ctx, l, v, pctx.WithDecodeList(tt.otherSections...), cfgPath)
			require.NoError(t, err)

			_, err = s.ResolvePartialConfig(ctx, l, v, pctx.WithDecodeList(config.DependencyBlock), cfgPath)
			require.NoError(t, err)

			_, err = s.ResolvePartialConfig(ctx, l, v, pctx.WithDecodeList(config.DependencyBlock, tt.section), cfgPath)
			tt.assertErr(t, err)

			_, err = s.ResolveUnitConfig(ctx, l, v, pctx, cfgPath)
			tt.assertErr(t, err)
		})
	}
}

// sourceReadsDependencyTree is a unit, module-c, whose dependency module-b sets its terraform source from an output
// of its own dependency, module-a. dependencyBlock is the body of module-c's dependency block on module-b.
func sourceReadsDependencyTree(t *testing.T, dependencyBlock string) (string, *venv.Venv) {
	t.Helper()

	root := venvtest.Root("/live")
	cfgPath := filepath.Join(root, "module-c", config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
		filepath.Join("module-a", config.DefaultTerragruntConfigPath): "",
		filepath.Join("module-b", config.DefaultTerragruntConfigPath): `
dependency "module_a" {
  config_path = "../module-a"
}

terraform {
  source = "../modules//${dependency.module_a.outputs.module_name}"
}
`,
		filepath.Join("module-c", config.DefaultTerragruntConfigPath): `
dependency "module_b" {
  config_path = "../module-b"
` + dependencyBlock + `
}

inputs = {
  ns = dependency.module_b.outputs.ns
}
`,
	}))

	return cfgPath, v
}

// TestResolveRunConfigSkipsTheSourceOfADependencyWhoseOutputsItSkips pins that a unit resolves when the terraform
// source of a dependency with skip_outputs reads `dependency`, since the unit never reads that dependency's outputs.
func TestResolveRunConfigSkipsTheSourceOfADependencyWhoseOutputsItSkips(t *testing.T) {
	t.Parallel()

	cfgPath, v := sourceReadsDependencyTree(t, `
  skip_outputs = true
  mock_outputs = { ns = "mocked" }
`)
	ctx, pctx := newTestParsingContext(t, cfgPath)

	cfg, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
	require.NoError(t, err)

	assert.Equal(t, "mocked", cfg.Inputs["ns"])
}

// TestResolveRunConfigRejectsTheSourceOfADependencyWhoseOutputsItReads pins that a render of a unit fails, with or
// without mock outputs, when it reads the outputs of a dependency whose terraform source reads `dependency`.
func TestResolveRunConfigRejectsTheSourceOfADependencyWhoseOutputsItReads(t *testing.T) {
	t.Parallel()

	blocks := map[string]string{
		"no mocks":     "",
		"mock outputs": `  mock_outputs = { ns = "mocked" }`,
	}

	for name, block := range blocks {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cfgPath, v := sourceReadsDependencyTree(t, block)
			ctx, pctx := newTestParsingContext(t, cfgPath)
			pctx.TerraformCommand = "render"
			pctx.TerraformCliArgs = iacargs.New().SetCommand("render")

			_, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
			require.ErrorAs(t, err, new(config.TerraformSourceReferencesDependencyError))
		})
	}
}

// TestResolveConfigReportsANestedIncludeOnce pins that a unit whose included file includes another reports the
// error once, in a partial parse and in a full one.
func TestResolveConfigReportsANestedIncludeOnce(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/live")
	cfgPath := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(venvtest.NewFS(t, root, map[string]string{
		"root.hcl": "",
		"mid.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
		filepath.Join("unit", config.DefaultTerragruntConfigPath): `
include "mid" {
  path = "../mid.hcl"
}
`,
	}))
	ctx, pctx := newTestParsingContext(t, cfgPath)
	l := logger.CreateLogger()

	_, err := new(config.Store).ResolvePartialConfig(
		ctx,
		l,
		v,
		pctx.WithDecodeList(versionCheckSections()...),
		cfgPath,
	)
	assert.Len(t, joinedErrorsAs[config.TooManyLevelsOfInheritanceError](err), 1)

	_, err = new(config.Store).ResolveRunConfig(ctx, l, v, pctx, cfgPath)
	assert.Len(t, joinedErrorsAs[config.TooManyLevelsOfInheritanceError](err), 1)
}

// joinedErrorsAs returns each error of type T that err joins, through every level of [errors.Join].
func joinedErrorsAs[T error](err error) []T {
	var found []T

	pending := []error{err}

	for len(pending) > 0 {
		last := len(pending) - 1
		next := pending[last]
		pending = pending[:last]

		if joined, ok := next.(interface{ Unwrap() []error }); ok {
			pending = append(pending, joined.Unwrap()...)

			continue
		}

		if target, ok := errors.AsType[T](next); ok {
			found = append(found, target)
		}
	}

	return found
}
