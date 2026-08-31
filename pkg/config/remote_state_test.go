package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeUnitPair writes a `bar` unit and a `foo` unit beside it, and returns the path to foo's config.
func writeUnitPair(t *testing.T, barHCL, fooHCL string) string {
	t.Helper()

	tmpDir := t.TempDir()

	for name, contents := range map[string]string{"bar": barHCL, "foo": fooHCL} {
		unitDir := filepath.Join(tmpDir, name)
		require.NoError(t, os.MkdirAll(unitDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(unitDir, config.DefaultTerragruntConfigPath),
			[]byte(contents),
			0644,
		))
	}

	return filepath.Join(tmpDir, "foo", config.DefaultTerragruntConfigPath)
}

func TestParseRemoteStateIgnoresUnappliedDependency(t *testing.T) {
	t.Parallel()

	fooPath := writeUnitPair(t, `inputs = {}`, `
dependency "bar" {
  config_path = "../bar"
}

remote_state {
  backend = "s3"

  config = {
    bucket = "foo-state"
    key    = "foo/tofu.tfstate"
    region = "us-east-1"
  }
}

inputs = {
  name = dependency.bar.outputs.name
}
`)

	l := createLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "s3", remoteState.BackendName)
	assert.Equal(t, "foo-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateResolvesDependencyOutputs(t *testing.T) {
	t.Parallel()

	fooPath := writeUnitPair(t, `inputs = {}`, `
dependency "bar" {
  config_path  = "../bar"
  skip_outputs = true

  mock_outputs = {
    bucket = "bar-state"
  }
}

remote_state {
  backend = "s3"

  config = {
    bucket = dependency.bar.outputs.bucket
    key    = "foo/tofu.tfstate"
    region = "us-east-1"
  }
}
`)

	l := createLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "bar-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateAsAttribute(t *testing.T) {
	t.Parallel()

	fooPath := writeUnitPair(t, `inputs = {}`, `
dependency "bar" {
  config_path = "../bar"
}

remote_state = {
  backend = "s3"

  config = {
    bucket = "foo-state"
    key    = "foo/tofu.tfstate"
    region = "us-east-1"
  }
}

inputs = {
  name = dependency.bar.outputs.name
}
`)

	l := createLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "s3", remoteState.BackendName)
	assert.Equal(t, "foo-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateFromJSONConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	fooPath := filepath.Join(tmpDir, config.DefaultTerragruntJSONConfigPath)
	require.NoError(t, os.WriteFile(fooPath, []byte(`{
  "remote_state": {
    "backend": "s3",
    "config": {
      "bucket": "foo-state",
      "key": "foo/tofu.tfstate",
      "region": "us-east-1"
    }
  }
}
`), 0644))

	l := createLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "s3", remoteState.BackendName)
	assert.Equal(t, "foo-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateFromExposedInclude(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.hcl"), []byte(`
locals {
  bucket = "root-state"
}
`), 0644))

	unitDir := filepath.Join(tmpDir, "foo")
	require.NoError(t, os.MkdirAll(unitDir, 0755))

	fooPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(fooPath, []byte(`
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

remote_state {
  backend = "s3"

  config = {
    bucket = include.root.locals.bucket
    key    = "foo/tofu.tfstate"
    region = "us-east-1"
  }
}
`), 0644))

	l := createLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "root-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateFromIncludeReadingDependencyOutputs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.hcl"), []byte(`
remote_state {
  backend = "s3"

  config = {
    bucket = dependency.bar.outputs.bucket
    key    = "foo/tofu.tfstate"
    region = "us-east-1"
  }
}
`), 0644))

	for _, name := range []string{"foo", "bar"} {
		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, name), 0755))
	}

	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, "bar", config.DefaultTerragruntConfigPath),
		[]byte(`inputs = {}`),
		0644,
	))

	fooPath := filepath.Join(tmpDir, "foo", config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(fooPath, []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "bar" {
  config_path  = "../bar"
  skip_outputs = true

  mock_outputs = {
    bucket = "bar-state"
  }
}
`), 0644))

	l := createLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "bar-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateFromIncludedConfig(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "root.hcl"), []byte(`
locals {
  bucket = "root-state"
}

remote_state {
  backend = "s3"

  config = {
    bucket = local.bucket
    key    = "tofu.tfstate"
    region = "us-east-1"
  }
}
`), 0644))

	unitDir := filepath.Join(tmpDir, "foo")
	require.NoError(t, os.MkdirAll(unitDir, 0755))

	fooPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(fooPath, []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "bar" {
  config_path = "../bar"
}

inputs = {
  name = dependency.bar.outputs.name
}
`), 0644))

	barDir := filepath.Join(tmpDir, "bar")
	require.NoError(t, os.MkdirAll(barDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(barDir, config.DefaultTerragruntConfigPath),
		[]byte(`inputs = {}`),
		0644,
	))

	l := createLogger()
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "root-state", remoteState.BackendConfig["bucket"])
}
