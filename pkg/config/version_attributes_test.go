package config_test

import (
	"maps"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const versionAttributeDependencyBlock = `
dependency "dep" {
  config_path  = "../dep"
  mock_outputs = { value = ">= 1.0" }
}
`

// versionAttributeCase is a unit tree in which a version attribute references `dependency`.
type versionAttributeCase struct {
	files     map[string]string
	name      string
	cfgFile   string
	errFile   string
	attribute string
}

func versionAttributeCases() []versionAttributeCase {
	unitPath := filepath.Join("unit", config.DefaultTerragruntConfigPath)

	return []versionAttributeCase{
		directVersionAttributeCase(config.MetadataTerraformBinary),
		directVersionAttributeCase(config.MetadataTerraformVersionConstraint),
		directVersionAttributeCase(config.MetadataTerragruntVersionConstraint),
		{
			name:      "untaken branch",
			cfgFile:   unitPath,
			errFile:   unitPath,
			attribute: config.MetadataTerraformVersionConstraint,
			files: map[string]string{
				unitPath: versionAttributeDependencyBlock + `
locals {
  use = false
}

terraform_version_constraint = local.use ? dependency.dep.outputs.value : ">= 1.0"
`,
			},
		},
		{
			name:      "json",
			cfgFile:   filepath.Join("unit", config.DefaultTerragruntJSONConfigPath),
			errFile:   filepath.Join("unit", config.DefaultTerragruntJSONConfigPath),
			attribute: config.MetadataTerraformBinary,
			files: map[string]string{
				filepath.Join("unit", config.DefaultTerragruntJSONConfigPath): `{
  "dependency": {"dep": {"config_path": "../dep", "mock_outputs": {"value": "tofu"}}},
  "terraform_binary": "${dependency.dep.outputs.value}"
}`,
			},
		},
		{
			name:      "included",
			cfgFile:   unitPath,
			errFile:   "root.hcl",
			attribute: config.MetadataTerragruntVersionConstraint,
			files: map[string]string{
				"root.hcl": versionAttributeDependencyBlock + `
terragrunt_version_constraint = dependency.dep.outputs.value
`,
				unitPath: `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
			},
		},
	}
}

// TestParseConfigFileVersionAttributeReferencingDependencyErrors pins that a full parse rejects a version attribute
// that references `dependency`, even when the output resolves or the reference never evaluates.
func TestParseConfigFileVersionAttributeReferencingDependencyErrors(t *testing.T) {
	t.Parallel()

	for _, tt := range versionAttributeCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root, v := writeVersionAttributeTree(t, tt.files)
			cfgPath := filepath.Join(root, tt.cfgFile)
			ctx, pctx := newTestParsingContext(t, cfgPath)

			s := new(config.Store)

			_, err := s.ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)

			assertVersionAttributeError(t, err, filepath.Join(root, tt.errFile), tt.attribute)
		})
	}
}

// TestPartialParseVersionAttributeReferencingDependencyErrors pins that discovery and the version check of a single
// run reject a version attribute that references `dependency`, so `run` and `run --all` agree.
func TestPartialParseVersionAttributeReferencingDependencyErrors(t *testing.T) {
	t.Parallel()

	parses := map[string]func(*config.ParsingContext) *config.ParsingContext{
		"discovery": func(pctx *config.ParsingContext) *config.ParsingContext {
			return pctx.WithDecodeList(
				config.DependencyBlock,
				config.TerragruntVersionConstraints,
				config.EngineBlock,
			).WithSkipOutputsResolution()
		},
		"version check": func(pctx *config.ParsingContext) *config.ParsingContext {
			return pctx.WithDecodeList(
				config.TerragruntVersionConstraints,
				config.FeatureFlagsBlock,
				config.EngineBlock,
			)
		},
	}

	for parseName, configure := range parses {
		for _, tt := range versionAttributeCases() {
			t.Run(parseName+"/"+tt.name, func(t *testing.T) {
				t.Parallel()

				root, v := writeVersionAttributeTree(t, tt.files)
				cfgPath := filepath.Join(root, tt.cfgFile)
				ctx, pctx := newTestParsingContext(t, cfgPath)

				s := new(config.Store)

				_, err := s.ResolvePartialConfig(ctx, logger.CreateLogger(), v, configure(pctx), cfgPath)

				assertVersionAttributeError(t, err, filepath.Join(root, tt.errFile), tt.attribute)
			})
		}
	}
}

// directVersionAttributeCase is a unit whose attribute reads a dependency output directly.
func directVersionAttributeCase(attribute string) versionAttributeCase {
	unitPath := filepath.Join("unit", config.DefaultTerragruntConfigPath)

	return versionAttributeCase{
		name:      attribute,
		cfgFile:   unitPath,
		errFile:   unitPath,
		attribute: attribute,
		files: map[string]string{
			unitPath: versionAttributeDependencyBlock + attribute + " = dependency.dep.outputs.value\n",
		},
	}
}

func writeVersionAttributeTree(t *testing.T, files map[string]string) (string, *venv.Venv) {
	t.Helper()

	root := venvtest.Root("/live")
	tree := map[string]string{filepath.Join("dep", config.DefaultTerragruntConfigPath): ""}
	maps.Copy(tree, files)

	return root, venv.MemVenv().WithFS(venvtest.NewFS(t, root, tree))
}

func assertVersionAttributeError(t *testing.T, err error, configPath, attribute string) {
	t.Helper()

	var diags hcl.Diagnostics
	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)

	assert.Equal(t, "Dependency reference not allowed", diags[0].Summary)
	assert.Contains(t, diags[0].Detail, "The "+attribute+" attribute")
	require.NotNil(t, diags[0].Subject)
	assert.Equal(t, configPath, diags[0].Subject.Filename)
}
