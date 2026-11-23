package config_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

func TestResolvePartialConfigReportsTheVersionConstraintsOfAConfigWithNewerSyntax(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, config.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		config.DefaultTerragruntConfigPath: `
terragrunt_version_constraint = ">= 99.0"

future_block {
  a = 1
}

exclude {
  if      = future_function("x")
  actions = ["all"]
}
`,
	})(t)

	var errBuf bytes.Buffer

	v := venv.MemVenv().WithFS(fsys).WithErrWriter(&errBuf)
	ctx, run := newTestUnitParsingContext(t, path)
	run.Parser.Diagnostics = config.DiagnosticsSuppressed

	s := &config.Store{}

	cfg, err := s.ResolvePartialConfig(
		ctx,
		logger.CreateLogger(),
		v,
		run.WithDecodeList(config.TerragruntVersionConstraints, config.FeatureFlagsBlock, config.EngineBlock),
		path,
	)
	require.NoError(t, err)
	assert.Equal(t, ">= 99.0", cfg.TerragruntVersionConstraint)
	assert.Zero(t, errBuf.Len(), "no diagnostic was written")

	_, err = s.ResolvePartialConfig(ctx, logger.CreateLogger(), v, run.WithDecodeList(config.ExcludeBlock), path)
	require.Error(t, err)
	assert.Equal(t, 1, strings.Count(errBuf.String(), "Call to unknown function"))
	assert.NotContains(t, errBuf.String(), "Unsupported block type")
}

func TestResolveRunConfigSkipsTheBrokenBlocksOfADependency(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		"app/terragrunt.hcl": `
dependency "iam" {
  config_path  = "../iam"
  skip_outputs = true
}
`,
		"iam/terragrunt.hcl": `
exclude {
  if      = get_env("TG_TEST_UNSET_VAR") == "1"
  actions = ["all"]
}

errors {
  ignore "i" {
    ignorable_errors = [get_env("TG_TEST_UNSET_VAR")]
  }
}
`,
	})(t)

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	ctx, run := newTestUnitParsingContext(t, path)

	s := &config.Store{}

	c, err := s.ResolveUnitConfig(ctx, l, v, run, path)
	require.NoError(t, err)

	cfg, err := c.ResolveRunConfig(ctx, l, v, s, run)
	require.NoError(t, err)
	assert.Len(t, cfg.TerragruntDependencies, 1)
}

func TestResolveUnitQueueConfigWritesNoDiagnosticOfAStackFile(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, config.DefaultStackFile)

	fsys := memFS(root, map[string]string{
		config.DefaultStackFile: `
unit "a" {
  source = "../catalog/a"
  path   = "a"
}

stack "b" {
  source = "../catalog/b"
  path   = "b"
}
`,
	})(t)

	var errBuf bytes.Buffer

	v := venv.MemVenv().WithFS(fsys).WithErrWriter(&errBuf)
	ctx, run := newTestUnitParsingContext(t, path)
	run.Parser.Diagnostics = config.DiagnosticsSuppressed
	run = run.WithDecodeList(config.DependenciesBlock, config.DependencyBlock, config.ExcludeBlock)

	_, err := (&config.Store{}).ResolveUnitQueueConfig(ctx, logger.CreateLogger(), v, run, path)
	require.NoError(t, err)
	assert.Zero(t, errBuf.Len(), "no diagnostic was written")
}

func TestResolveRunConfigWritesAnUnsupportedArgumentOnce(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, config.DefaultTerragruntConfigPath)

	fsys := memFS(root, map[string]string{
		config.DefaultTerragruntConfigPath: `
inptus = {
  name = "c"
}
`,
	})(t)

	var errBuf bytes.Buffer

	v := venv.MemVenv().WithFS(fsys).WithErrWriter(&errBuf)
	l := logger.CreateLogger()
	ctx, run := newTestUnitParsingContext(t, path)
	run.Parser.Diagnostics = config.DiagnosticsSuppressed

	s := &config.Store{}

	_, err := s.ResolvePartialConfig(ctx, l, v, run.WithDecodeList(config.TerragruntVersionConstraints), path)
	require.NoError(t, err)

	c, err := s.ResolveUnitConfig(ctx, l, v, run, path)
	require.NoError(t, err)

	_, err = c.ResolveRunConfig(ctx, l, v, s, run)
	require.Error(t, err)
	assert.Equal(t, 1, strings.Count(errBuf.String(), "Unsupported argument"))
}
