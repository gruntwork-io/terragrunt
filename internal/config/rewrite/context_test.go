package config_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	config "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestResolveUnitConfigLeavesParseContextUnchanged(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()
	root := t.TempDir()
	cfgPath := filepath.Join(root, "unit", pkgconfig.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(v.FS, filepath.Join(root, "root.hcl"), []byte(`
locals {
  region = "us-east-1"
}

inputs = {
  region = local.region
}
`), 0o644))
	require.NoError(t, vfs.WriteFile(v.FS, cfgPath, []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

locals {
  name = "unit"
}

inputs = {
  name = local.name
}
`), 0o644))

	ctx, run := newTestParsingContext(t, cfgPath)
	run.OriginalIAMRoleOptions = iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/cli"}
	runLocals := cty.ObjectVal(map[string]cty.Value{"from_run": cty.StringVal("run")})
	run.Locals = &runLocals

	want := run.Clone()
	pc := config.NewParseContext(run)
	l := logger.CreateLogger()

	first, err := new(config.Store).ResolveUnitConfig(ctx, l, v, pc, cfgPath)
	require.NoError(t, err)

	second, err := new(config.Store).ResolveUnitConfig(ctx, l, v, pc, cfgPath)
	require.NoError(t, err)

	firstCfg, err := first.ToV1(ctx, l, v, pc)
	require.NoError(t, err)

	secondCfg, err := second.ToV1(ctx, l, v, pc)
	require.NoError(t, err)

	assert.Equal(t, firstCfg, secondCfg)
	assert.Equal(t, map[string]any{"name": "unit", "region": "us-east-1"}, firstCfg.Inputs)

	assert.Equal(t, want.TrackInclude, run.TrackInclude)
	assert.Equal(t, want.Values, run.Values)
	assert.Equal(t, want.Features, run.Features)
	assert.Equal(t, want.Locals, run.Locals)
	assert.Equal(t, want.DecodedDependencies, run.DecodedDependencies)
	assert.Equal(t, want.IAMRoleOptions, run.IAMRoleOptions)
	assert.Equal(t, want.ParseDepth, run.ParseDepth)
	assert.Equal(t, want.SkipAutoIncludeMerge, run.SkipAutoIncludeMerge)
	assert.Equal(t, want.Parser, run.Parser)
}

func TestResolveUnitConfigAppliesRunScopedValues(t *testing.T) {
	t.Parallel()

	v := venv.MemVenv()
	cfgPath := filepath.Join(t.TempDir(), pkgconfig.DefaultTerragruntConfigPath)

	require.NoError(t, vfs.WriteFile(v.FS, cfgPath, []byte(`
inputs = {
  vpc_id = dependency.vpc.outputs.id
}
`), 0o644))

	ctx, run := newTestParsingContext(t, cfgPath)
	decodedDeps := cty.ObjectVal(map[string]cty.Value{
		"vpc": cty.ObjectVal(map[string]cty.Value{
			"outputs": cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("vpc-from-run")}),
		}),
	})
	run.DecodedDependencies = &decodedDeps

	l := logger.CreateLogger()

	pc := config.NewParseContext(run)

	parsed, err := new(config.Store).ResolveUnitConfig(ctx, l, v, pc, cfgPath)
	require.NoError(t, err)

	cfg, err := parsed.ToV1(ctx, l, v, pc)
	require.NoError(t, err)

	assert.Equal(t, "vpc-from-run", cfg.Inputs["vpc_id"])
}
