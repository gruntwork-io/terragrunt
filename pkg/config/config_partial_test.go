package config_test

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestPartialParseResolvesLocals(t *testing.T) {
	t.Parallel()

	cfg := `
locals {
  app1 = "../app1"
}

dependencies {
  paths = [local.app1]
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.DependenciesBlock)
	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(t, err)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
	assert.Equal(t, "../app1", terragruntConfig.Dependencies.Paths[0])
	assert.Equal(t, map[string]any{"app1": "../app1"}, terragruntConfig.Locals)

	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
}

// TestPartialParseFailsOnABrokenBlockOutsideTheDecodeList pins that a partial parse decodes every block discovery
// reads, so a block that fails to decode fails the parse whatever the decode list names.
func TestPartialParseFailsOnABrokenBlockOutsideTheDecodeList(t *testing.T) {
	t.Parallel()

	cfg := `
dependencies {
  paths = [file("i-am-a-file-that-does-not-exist")]
}

prevent_destroy = false
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.TerragruntFlags)

	s := new(config.Store)

	_, err := resolvePartialConfigString(t, ctx, l, v, s, pctx, config.DefaultTerragruntConfigPath, cfg)
	require.Error(t, err)
}

func TestPartialParseMultipleItems(t *testing.T) {
	t.Parallel()

	cfg := `
dependencies {
  paths = ["../app1"]
}

prevent_destroy = true
skip = true
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.DependenciesBlock, config.TerragruntFlags)
	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(t, err)

	assert.NotNil(t, terragruntConfig.Dependencies)
	assert.Len(t, terragruntConfig.Dependencies.Paths, 1)
	assert.Equal(t, "../app1", terragruntConfig.Dependencies.Paths[0])

	assert.True(t, *terragruntConfig.PreventDestroy)

	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

func TestPartialParseOmittedItems(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.DependenciesBlock, config.TerragruntFlags)
	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		"",
	)

	require.NoError(t, err)
	assert.Nil(t, terragruntConfig.Dependencies)
	assert.Nil(t, terragruntConfig.PreventDestroy)
	assert.Nil(t, terragruntConfig.Terraform)
	assert.Nil(t, terragruntConfig.RemoteState)
	assert.Nil(t, terragruntConfig.Inputs)
	assert.Nil(t, terragruntConfig.Locals)
}

// TestPartialParseFailsOnABrokenBlockOutsideTheDecodeListInParent pins that a block of an included file that fails
// to decode fails a partial parse whatever the decode list names.
func TestPartialParseFailsOnABrokenBlockOutsideTheDecodeListInParent(t *testing.T) {
	t.Parallel()

	configPath, err := filepath.Abs(
		filepath.Join(
			"../..",
			"test",
			"fixtures",
			"partial-parse",
			"ignore-bad-block-in-parent",
			"child",
			config.DefaultTerragruntConfigPath,
		),
	)
	require.NoError(t, err)

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, configPath)
	pctx = pctx.WithDecodeList(config.TerragruntFlags)
	s := new(config.Store)

	_, err = s.ResolvePartialConfig(ctx, l, v, config.NewParseContext(v, pctx), configPath)
	require.Error(t, err)
}

// Regression test for https://github.com/gruntwork-io/terragrunt/issues/5949: various invalid
// include / locals combinations must not panic out of PartialParseConfig.
func TestPartialParseDoesNotPanicWithIncludeAndReadTerragruntConfigLocals(t *testing.T) {
	t.Parallel()

	testCases := map[string]string{
		"read-target-missing": `
include "root" {
  path = find_in_parent_folders("parent.hcl")
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"include-path-empty-string": `
include "root" {
  path = ""
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"include-path-undefined-local": `
include "root" {
  path = local.undefined
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"include-points-to-missing-file": `
include "root" {
  path = find_in_parent_folders("does-not-exist.hcl")
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"include-points-to-malformed-file": `
include "root" {
  path = find_in_parent_folders("malformed.hcl")
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"multiple-includes-same-name": `
include "root" {
  path = find_in_parent_folders("parent.hcl")
}

include "root" {
  path = find_in_parent_folders("parent.hcl")
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"include-parent-also-has-include": `
include "root" {
  path = find_in_parent_folders("nested-parent.hcl")
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"bare-include-no-label": `
include {
  path = find_in_parent_folders("parent.hcl")
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"include-expression-calling-missing-env": `
include "root" {
  path = find_in_parent_folders(get_env("TG_TEST_NONEXISTENT_ENV_VAR_FOR_5949"))
}

locals {
  env_vars = read_terragrunt_config(find_in_parent_folders(get_env("TG_TEST_NONEXISTENT_ENV_VAR_FOR_5949")))
}
`,
		"feature-default-undefined-local": `
include "root" {
  path = find_in_parent_folders("parent.hcl")
}

feature "broken" {
  default = local.never_set
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
		"feature-default-undefined-function": `
include "root" {
  path = find_in_parent_folders("parent.hcl")
}

feature "broken" {
  default = not_a_real_function("x")
}

locals {
  env_vars = read_terragrunt_config("this-file-does-not-exist.hcl")
}
`,
	}

	for name, childHCL := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "parent.hcl"), []byte(`
locals {
  shared = "value"
}
`), 0644))

			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "malformed.hcl"), []byte(`
locals {
  broken = {{{
`), 0644))

			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "nested-parent.hcl"), []byte(`
include "grandparent" {
  path = find_in_parent_folders("parent.hcl")
}
`), 0644))

			childDir := filepath.Join(tmpDir, "child")
			require.NoError(t, os.MkdirAll(childDir, 0755))

			childPath := filepath.Join(childDir, config.DefaultTerragruntConfigPath)
			require.NoError(t, os.WriteFile(childPath, []byte(childHCL), 0644))

			l := logger.CreateLogger()

			v := venvtest.NewWithOSFS()
			ctx, pctx := newTestParsingContext(t, childPath)
			pctx = pctx.WithDecodeList(config.TerragruntFlags)

			s := new(config.Store)

			_, err := s.ResolvePartialConfig(ctx, l, v, config.NewParseContext(v, pctx), childPath)
			require.Error(
				t,
				err,
				"expected malformed fixture to surface an error rather than silently succeed",
			)
		})
	}
}

func TestPartialParseDependencyBlockSetsTerragruntDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(t, err)

	assert.NotNil(t, terragruntConfig.TerragruntDependencies)
	assert.Len(t, terragruntConfig.TerragruntDependencies, 1)
	assert.Equal(t, "vpc", terragruntConfig.TerragruntDependencies[0].Name)
	assert.Equal(t, cty.StringVal("../app1"), terragruntConfig.TerragruntDependencies[0].ConfigPath)
}

func TestPartialParseMultipleDependencyBlockSetsTerragruntDependencies(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../app1"
}

dependency "sql" {
  config_path = "../db1"
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.DependencyBlock)
	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(t, err)

	assert.NotNil(t, terragruntConfig.TerragruntDependencies)
	assert.Len(t, terragruntConfig.TerragruntDependencies, 2)
	assert.Equal(t, "vpc", terragruntConfig.TerragruntDependencies[0].Name)
	assert.Equal(t, cty.StringVal("../app1"), terragruntConfig.TerragruntDependencies[0].ConfigPath)
	assert.Equal(t, "sql", terragruntConfig.TerragruntDependencies[1].Name)
	assert.Equal(t, cty.StringVal("../db1"), terragruntConfig.TerragruntDependencies[1].ConfigPath)
}

func TestPartialParseOnlyParsesTerraformSource(t *testing.T) {
	t.Parallel()

	cfg := `
dependency "vpc" {
  config_path = "../vpc"
}

terraform {
  source = "../../modules/app"
  before_hook "before" {
    commands = ["apply"]
	execute  = ["echo", dependency.vpc.outputs.vpc_id]
  }
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.TerraformSource)
	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(t, err)

	assert.NotNil(t, terragruntConfig.Terraform)
	assert.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "../../modules/app", *terragruntConfig.Terraform.Source)
}

func TestPartialParseCacheWithInvalidFile(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	configPath := filepath.Join(tmpDir, "terragrunt.hcl")
	invalidContent := `invalid hcl syntax {`
	require.NoError(t, os.WriteFile(configPath, []byte(invalidContent), 0644))

	hclCache := cache.NewCache[*config.CachedHCLFile]("test-hcl-cache")
	l := logger.CreateLogger()
	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	ctx = context.WithValue(ctx, config.HclCacheContextKey, hclCache)
	pctx = pctx.WithDecodeList(config.DependenciesBlock)

	// Parse should fail and not cache an invalid file
	s := new(config.Store)

	_, err := s.ResolvePartialConfig(ctx, l, v, config.NewParseContext(v, pctx), configPath)
	require.Error(t, err, "parsing invalid HCL should fail")

	// Verify nothing was cached
	fileInfo, err := os.Stat(configPath)
	require.NoError(t, err)

	cacheKey := fmt.Sprintf(
		"configPath-%v-modTime-%v-size-%v",
		configPath,
		fileInfo.ModTime().UnixMicro(),
		fileInfo.Size(),
	)

	_, found := hclCache.Get(ctx, cacheKey)
	require.False(t, found, "invalid file should not be cached")
}

// TestPartialParseFeatureFlagDefaultInExcludeBlock tests that feature flag default values
// are correctly resolved when used in exclude blocks in the same file (no includes).
// This reproduces the bug from https://github.com/gruntwork-io/terragrunt/issues/4395
func TestPartialParseFeatureFlagDefaultInExcludeBlock(t *testing.T) {
	t.Parallel()

	cfg := `
feature "skip_ci" {
  default = false
}

exclude {
  if                   = feature.skip_ci.value
  actions              = ["plan", "apply"]
  exclude_dependencies = false
}
`

	l := logger.CreateLogger()
	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.FeatureFlagsBlock, config.ExcludeBlock)

	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(t, err, "parsing feature flag default in exclude block should not fail")

	// Feature flag default is false, so the exclude block should have if=false
	assert.NotNil(t, terragruntConfig.Exclude)

	if terragruntConfig.Exclude != nil {
		assert.False(
			t,
			terragruntConfig.Exclude.If,
			"exclude.if should be false when feature default is false",
		)
	}
}

// TestPartialParseFeatureFlagDefaultWithTerragruntFlagsOnly tests the decode list
// used by decodeDependencies - TerragruntFlags only (no ExcludeBlock).
// This is the path triggered when resolving dependency configs.
func TestPartialParseFeatureFlagDefaultWithTerragruntFlagsOnly(t *testing.T) {
	t.Parallel()

	cfg := `
feature "skip_ci" {
  default = false
}

exclude {
  if                   = feature.skip_ci.value
  actions              = ["plan", "apply"]
  exclude_dependencies = false
}
`

	l := logger.CreateLogger()
	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.TerragruntFlags)

	_, err := resolvePartialConfigString(t, ctx, l, v, new(config.Store), pctx, config.DefaultTerragruntConfigPath, cfg)
	require.NoError(
		t,
		err,
		"parsing with TerragruntFlags decode list should not fail when feature flags and exclude blocks are present",
	)
}

// TestPartialParseFeatureFlagDefaultTrueInExcludeBlock tests that when the feature
// flag default is true, the exclude block correctly evaluates to true.
func TestPartialParseFeatureFlagDefaultTrueInExcludeBlock(t *testing.T) {
	t.Parallel()

	cfg := `
feature "skip_ci" {
  default = true
}

exclude {
  if                   = feature.skip_ci.value
  actions              = ["plan", "apply"]
  exclude_dependencies = false
}
`

	l := logger.CreateLogger()
	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.FeatureFlagsBlock, config.ExcludeBlock)

	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(t, err, "parsing feature flag default in exclude block should not fail")

	assert.NotNil(t, terragruntConfig.Exclude)

	if terragruntConfig.Exclude != nil {
		assert.True(
			t,
			terragruntConfig.Exclude.If,
			"exclude.if should be true when feature default is true",
		)
	}
}

func TestPartialParseTerraformExtraArgsDecodesSourceAndExtraArgs(t *testing.T) {
	t.Parallel()

	cfg := `
terraform {
  source = "./module"

  extra_arguments "secrets" {
    commands = ["output", "plan"]
    env_vars = {
      TF_WORKSPACE = "custom"
    }
  }
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.TerraformExtraArgs)
	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(t, err)

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "./module", *terragruntConfig.Terraform.Source)

	require.Len(t, terragruntConfig.Terraform.ExtraArgs, 1)
	assert.Equal(t, "secrets", terragruntConfig.Terraform.ExtraArgs[0].Name)
	require.NotNil(t, terragruntConfig.Terraform.ExtraArgs[0].EnvVars)
	assert.Equal(
		t,
		map[string]string{"TF_WORKSPACE": "custom"},
		*terragruntConfig.Terraform.ExtraArgs[0].EnvVars,
	)
}

func TestPartialParseTerraformExtraArgsIgnoresHooksReferencingDependency(t *testing.T) {
	t.Parallel()

	cfg := `
terraform {
  source = "./module"

  extra_arguments "secrets" {
    commands = ["output"]
    env_vars = {
      TF_WORKSPACE = "custom"
    }
  }

  before_hook "use_dep" {
    commands = ["init", "apply", "plan"]
    execute  = ["echo", "${dependency.upstream.outputs.cluster_id}"]
  }

  after_hook "use_dep_after" {
    commands = ["apply"]
    execute  = ["echo", "${dependency.upstream.outputs.cluster_id}"]
  }
}

dependency "upstream" {
  config_path                             = "../upstream"
  mock_outputs                            = { cluster_id = "m" }
  mock_outputs_allowed_terraform_commands = ["init", "validate", "plan", "output", "state"]
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.DependencyBlock, config.TerraformExtraArgs).
		WithDiagnosticsSuppressed()
	terragruntConfig, err := resolvePartialConfigString(
		t,
		ctx,
		l,
		v,
		new(config.Store),
		pctx,
		config.DefaultTerragruntConfigPath,
		cfg,
	)
	require.NoError(
		t,
		err,
		"hooks referencing a dependency must not be evaluated during the narrow terraform decode",
	)

	require.NotNil(t, terragruntConfig.Terraform)
	require.NotNil(t, terragruntConfig.Terraform.Source)
	assert.Equal(t, "./module", *terragruntConfig.Terraform.Source)

	require.Len(t, terragruntConfig.Terraform.ExtraArgs, 1)
	require.NotNil(t, terragruntConfig.Terraform.ExtraArgs[0].EnvVars)
	assert.Equal(
		t,
		map[string]string{"TF_WORKSPACE": "custom"},
		*terragruntConfig.Terraform.ExtraArgs[0].EnvVars,
	)

	// hooks are intentionally left undecoded so their dependency references are never evaluated
	assert.Empty(t, terragruntConfig.Terraform.BeforeHooks)
	assert.Empty(t, terragruntConfig.Terraform.AfterHooks)

	// the dependency block itself is still decoded for downstream resolution
	require.Len(t, terragruntConfig.TerragruntDependencies, 1)
	assert.Equal(t, "upstream", terragruntConfig.TerragruntDependencies[0].Name)
}

func TestPartialParseTerraformSourceReferencingDependencyReturnsClearError(t *testing.T) {
	t.Parallel()

	cfg := `
terraform {
  source = "./module-${dependency.upstream.outputs.cluster_id}"
}

dependency "upstream" {
  config_path = "../upstream"
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)

	pctx = pctx.WithDecodeList(config.TerraformSource)

	_, err := resolvePartialConfigString(t, ctx, l, v, new(config.Store), pctx, config.DefaultTerragruntConfigPath, cfg)

	var srcErr config.TerraformSourceReferencesDependencyError
	require.ErrorAs(
		t,
		err,
		&srcErr,
		"source referencing a dependency should return a clear typed error, not a cryptic decode error",
	)
}

// TestPartialParseTerraformSourceDependencyInUntakenBranchIsRejected covers a source whose dependency reference sits
// in an untaken conditional branch. The source is consumed during discovery and queue construction, before any
// dependency output exists, so the reference is rejected outright even though the branch never evaluates.
func TestPartialParseTerraformSourceDependencyInUntakenBranchIsRejected(t *testing.T) {
	t.Parallel()

	cfg := `
terraform {
  source = true ? "./module" : dependency.upstream.outputs.source
}

dependency "upstream" {
  config_path = "../upstream"
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.TerraformSource)

	_, err := resolvePartialConfigString(t, ctx, l, v, new(config.Store), pctx, config.DefaultTerragruntConfigPath, cfg)

	var srcErr config.TerraformSourceReferencesDependencyError
	require.ErrorAs(
		t,
		err,
		&srcErr,
		"a dependency reference anywhere in the source, even an untaken branch, must be rejected",
	)
}

// TestPartialParseTerraformSourceUnrelatedDecodeErrorIsNotRebranded guards against over-eager rebranding: a source
// that does not reference the dependency namespace but fails to decode for another reason must keep its own precise
// diagnostic rather than surfacing TerraformSourceReferencesDependencyError. Here the source is a tuple, so it fails
// to decode into a string.
func TestPartialParseTerraformSourceUnrelatedDecodeErrorIsNotRebranded(t *testing.T) {
	t.Parallel()

	cfg := `
terraform {
  source = [1, 2, 3]
}
`

	l := logger.CreateLogger()

	v := venvtest.NewWithOSFS()
	ctx, pctx := newTestParsingContext(t, config.DefaultTerragruntConfigPath)
	pctx = pctx.WithDecodeList(config.TerraformSource)
	_, err := resolvePartialConfigString(t, ctx, l, v, new(config.Store), pctx, config.DefaultTerragruntConfigPath, cfg)

	require.Error(t, err)

	var srcErr config.TerraformSourceReferencesDependencyError
	require.NotErrorAs(
		t,
		err,
		&srcErr,
		"an unrelated source decode error must not be rebranded as a dependency reference error",
	)
}

// TestPartialParseIncludeRejectsDeepMapOnly pins that deep_map_only, which applies to
// dependency mock outputs rather than includes, is reported as unsupported.
func TestPartialParseIncludeRejectsDeepMapOnly(t *testing.T) {
	t.Parallel()

	v, tmpDir := newMemTestDir(t)

	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(tmpDir, "root.hcl"), []byte(`
feature "skip_ci" {
  default = true
}
`), 0644))

	childPath := filepath.Join(tmpDir, "child", config.DefaultTerragruntConfigPath)
	require.NoError(t, vfs.WriteFile(v.FS, childPath, []byte(`
include "root" {
  path           = "../root.hcl"
  merge_strategy = "deep_map_only"
}
`), 0644))

	l := logger.CreateLogger()
	ctx, pctx := newTestParsingContext(t, childPath)
	pctx = pctx.WithDecodeList(config.FeatureFlagsBlock, config.ExcludeBlock)

	s := new(config.Store)

	_, err := s.ResolvePartialConfig(ctx, l, v, config.NewParseContext(v, pctx), childPath)

	var strategyErr config.IncludeMergeStrategyNotSupportedError

	require.ErrorAs(t, err, &strategyErr)
}

// TestPartialParseFeatureFlagDefaultsFromIncludes verifies included feature defaults are available during partial parsing.
func TestPartialParseFeatureFlagDefaultsFromIncludes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		parentHCL   string
		childHCL    string
		cliFlags    map[string]string
		expectedErr string
		expected    bool
	}{
		{
			name: "parent default visible",
			parentHCL: `
feature "skip_ci" {
  default = true
}
`,
			childHCL: `
include "root" {
  path = "../root.hcl"
}

exclude {
  if                   = feature.skip_ci.value
  actions              = ["plan", "apply"]
  exclude_dependencies = false
}
`,
			expected: true,
		},
		{
			name: "child default overrides parent default",
			parentHCL: `
feature "skip_ci" {
  default = true
}
`,
			childHCL: `
include "root" {
  path = "../root.hcl"
}

feature "skip_ci" {
  default = false
}

exclude {
  if                   = feature.skip_ci.value
  actions              = ["plan", "apply"]
  exclude_dependencies = false
}
`,
			expected: false,
		},
		{
			name: "cli override wins over included default",
			parentHCL: `
feature "skip_ci" {
  default = true
}
`,
			childHCL: `
include "root" {
  path = "../root.hcl"
}

exclude {
  if                   = feature.skip_ci.value
  actions              = ["plan", "apply"]
  exclude_dependencies = false
}
`,
			cliFlags: map[string]string{"skip_ci": "false"},
			expected: false,
		},
		{
			name: "no merge does not import parent default",
			parentHCL: `
feature "skip_ci" {
  default = true
}
`,
			childHCL: `
include "root" {
  path           = "../root.hcl"
  merge_strategy = "no_merge"
}

exclude {
  if                   = feature.skip_ci.value
  actions              = ["plan", "apply"]
  exclude_dependencies = false
}
`,
			expectedErr: "Unsupported attribute",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := t.TempDir()
			childDir := filepath.Join(tmpDir, "child")
			require.NoError(t, os.MkdirAll(childDir, 0755))

			require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.hcl"), []byte(tc.parentHCL), 0644))

			childPath := filepath.Join(childDir, config.DefaultTerragruntConfigPath)
			require.NoError(t, os.WriteFile(childPath, []byte(tc.childHCL), 0644))

			l := logger.CreateLogger()
			v := venvtest.NewWithOSFS()
			ctx, pctx := newTestParsingContext(t, childPath)
			pctx = pctx.WithDecodeList(config.FeatureFlagsBlock, config.ExcludeBlock)

			maps.Copy(pctx.FeatureFlags, tc.cliFlags)

			s := new(config.Store)

			terragruntConfig, err := s.ResolvePartialConfig(ctx, l, v, config.NewParseContext(v, pctx), childPath)
			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)

				return
			}

			require.NoError(t, err)
			require.NotNil(t, terragruntConfig.Exclude)
			assert.Equal(t, tc.expected, terragruntConfig.Exclude.If)
		})
	}
}

func TestPartialParseDeepMergesIncludedFeatureDefaultsOnce(t *testing.T) {
	t.Parallel()

	v, rootDir := newMemTestDir(t)

	files := map[string]string{
		"root.hcl": `
feature "tags" {
  default = {
    names = ["root"]
  }
}
`,
		filepath.Join("unit", "terragrunt.hcl"): `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "deep"
}

feature "tags" {
  default = {
    names = ["unit"]
  }
}
`,
	}

	for name, contents := range files {
		path := filepath.Join(rootDir, name)
		require.NoError(t, v.FS.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, vfs.WriteFile(v.FS, path, []byte(contents), 0o644))
	}

	unitPath := filepath.Join(rootDir, "unit", config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, unitPath)
	pctx = pctx.WithDecodeList(config.FeatureFlagsBlock, config.DependencyBlock)

	s := new(config.Store)

	cfg, err := s.ResolvePartialConfig(ctx, logger.CreateLogger(), v, config.NewParseContext(v, pctx), unitPath)
	require.NoError(t, err)

	require.Len(t, cfg.FeatureFlags, 1)
	require.NotNil(t, cfg.FeatureFlags[0].Default)

	names := cfg.FeatureFlags[0].Default.GetAttr("names")

	var got []string

	for it := names.ElementIterator(); it.Next(); {
		_, name := it.Element()
		got = append(got, name.AsString())
	}

	assert.Equal(t, []string{"root", "unit"}, got)
}

func TestPartialParseEvaluatesEachIncludeOnce(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		decodeList []config.PartialDecodeSectionType
		wantOpens  int
	}{
		{
			name:       "decode list with feature blocks",
			decodeList: []config.PartialDecodeSectionType{config.FeatureFlagsBlock, config.DependencyBlock, config.ExcludeBlock},
			wantOpens:  1,
		},
		{
			name:       "decode list without feature blocks",
			decodeList: []config.PartialDecodeSectionType{config.DependencyBlock},
			wantOpens:  1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, rootDir := newMemTestDir(t)

			files := map[string]string{
				"accounts.yml": "dev: \"111111111111\"\n",
				"root.hcl": `
locals {
  accounts = yamldecode(file("accounts.yml"))
}

feature "skip" {
  default = true
}

dependency "vpc" {
  config_path = "${get_terragrunt_dir()}/../vpc"
}
`,
				filepath.Join("unit", "terragrunt.hcl"): `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

exclude {
  if      = feature.skip.value
  actions = ["all"]
}
`,
			}

			for name, contents := range files {
				path := filepath.Join(rootDir, name)
				require.NoError(t, v.FS.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, vfs.WriteFile(v.FS, path, []byte(contents), 0o644))
			}

			fsys := &countingFS{FS: v.FS, opens: map[string]int{}}
			v = v.WithFS(fsys)

			unitPath := filepath.Join(rootDir, "unit", config.DefaultTerragruntConfigPath)
			ctx, pctx := newTestParsingContext(t, unitPath)
			pctx = pctx.WithDecodeList(tc.decodeList...).WithSkipOutputsResolution()

			s := new(config.Store)

			cfg, err := s.ResolvePartialConfig(ctx, logger.CreateLogger(), v, config.NewParseContext(v, pctx), unitPath)
			require.NoError(t, err)

			require.Len(t, cfg.TerragruntDependencies, 1)
			assert.Equal(t, "vpc", cfg.TerragruntDependencies[0].Name)
			assert.Equal(t, tc.wantOpens, fsys.opensOf(filepath.Join(rootDir, "accounts.yml")))

			if slices.Contains(tc.decodeList, config.ExcludeBlock) {
				require.NotNil(t, cfg.Exclude)
				assert.True(t, cfg.Exclude.If)
			}
		})
	}
}

func TestParseDoesNotParseNoMergeInclude(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		parse func(ctx context.Context, v *venv.Venv, pctx *config.ParsingContext, path string) (*config.TerragruntConfig, error)
		name  string
	}{
		{
			name: "full parse",
			parse: func(ctx context.Context, v *venv.Venv, pctx *config.ParsingContext, path string) (*config.TerragruntConfig, error) {
				s := new(config.Store)

				return s.ResolveRunConfig(ctx, logger.CreateLogger(), v, config.NewParseContext(v, pctx), path)
			},
		},
		{
			name: "partial parse",
			parse: func(ctx context.Context, v *venv.Venv, pctx *config.ParsingContext, path string) (*config.TerragruntConfig, error) {
				pctx = pctx.WithDecodeList(config.DependencyBlock, config.FeatureFlagsBlock)

				s := new(config.Store)

				return s.ResolvePartialConfig(ctx, logger.CreateLogger(), v, config.NewParseContext(v, pctx), path)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, rootDir := newMemTestDir(t)

			files := map[string]string{
				"root.hcl": `locals { value = "root" }`,
				filepath.Join("unit", "terragrunt.hcl"): `
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "no_merge"
}
`,
			}

			for name, contents := range files {
				path := filepath.Join(rootDir, name)
				require.NoError(t, v.FS.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, vfs.WriteFile(v.FS, path, []byte(contents), 0o644))
			}

			fsys := &countingFS{FS: v.FS, opens: map[string]int{}}
			v = v.WithFS(fsys)

			unitPath := filepath.Join(rootDir, "unit", config.DefaultTerragruntConfigPath)
			ctx, pctx := newTestParsingContext(t, unitPath)

			_, err := tc.parse(ctx, v, pctx, unitPath)
			require.NoError(t, err)

			assert.Equal(t, 0, fsys.opensOf(filepath.Join(rootDir, "root.hcl")))
		})
	}
}
