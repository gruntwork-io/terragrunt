package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unitsVenv builds a venv over an in-memory tree of the given files, rooted at a directory named
// for the test, and returns it with the root. The exec handle refuses to spawn, so a parse that
// reaches for `tofu output` fails at the spawn rather than on whatever the machine running the
// suite happens to have installed.
func unitsVenv(t *testing.T, files map[string]string) (*venv.Venv, string) {
	t.Helper()

	root := filepath.Join("/units", t.Name())

	return venvtest.New().WithFS(venvtest.NewFS(t, root, files)), root
}

// unitPairVenv builds a venv over a `bar` unit and a `foo` unit beside it, and returns it with the
// path to foo's config.
func unitPairVenv(t *testing.T, barHCL, fooHCL string) (*venv.Venv, string) {
	t.Helper()

	v, root := unitsVenv(t, map[string]string{
		filepath.Join("bar", config.DefaultTerragruntConfigPath): barHCL,
		filepath.Join("foo", config.DefaultTerragruntConfigPath): fooHCL,
	})

	return v, filepath.Join(root, "foo", config.DefaultTerragruntConfigPath)
}

func TestParseRemoteStateIgnoresUnappliedDependency(t *testing.T) {
	t.Parallel()

	v, fooPath := unitPairVenv(t, `inputs = {}`, `
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
	ctx, pctx := newTestParsingContext(t, v, fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "s3", remoteState.BackendName)
	assert.Equal(t, "foo-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateResolvesDependencyOutputs(t *testing.T) {
	t.Parallel()

	v, fooPath := unitPairVenv(t, `inputs = {}`, `
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
	ctx, pctx := newTestParsingContext(t, v, fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "bar-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateAsAttribute(t *testing.T) {
	t.Parallel()

	v, fooPath := unitPairVenv(t, `inputs = {}`, `
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
	ctx, pctx := newTestParsingContext(t, v, fooPath)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "s3", remoteState.BackendName)
	assert.Equal(t, "foo-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateFromJSONConfig(t *testing.T) {
	t.Parallel()

	v, root := unitsVenv(t, map[string]string{
		config.DefaultTerragruntJSONConfigPath: `{
  "remote_state": {
    "backend": "s3",
    "config": {
      "bucket": "foo-state",
      "key": "foo/tofu.tfstate",
      "region": "us-east-1"
    }
  }
}
`,
	})

	l := createLogger()
	ctx, pctx := newTestParsingContext(
		t,
		v,
		filepath.Join(root, config.DefaultTerragruntJSONConfigPath),
	)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "s3", remoteState.BackendName)
	assert.Equal(t, "foo-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateFromExposedInclude(t *testing.T) {
	t.Parallel()

	v, root := unitsVenv(t, map[string]string{
		"root.hcl": `
locals {
  bucket = "root-state"
}
`,
		filepath.Join("foo", config.DefaultTerragruntConfigPath): `
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
`,
	})

	l := createLogger()
	ctx, pctx := newTestParsingContext(
		t,
		v,
		filepath.Join(root, "foo", config.DefaultTerragruntConfigPath),
	)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "root-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateFromIncludeReadingDependencyOutputs(t *testing.T) {
	t.Parallel()

	v, root := unitsVenv(t, map[string]string{
		"root.hcl": `
remote_state {
  backend = "s3"

  config = {
    bucket = dependency.bar.outputs.bucket
    key    = "foo/tofu.tfstate"
    region = "us-east-1"
  }
}
`,
		filepath.Join("bar", config.DefaultTerragruntConfigPath): `inputs = {}`,
		filepath.Join("foo", config.DefaultTerragruntConfigPath): `
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
`,
	})

	l := createLogger()
	ctx, pctx := newTestParsingContext(
		t,
		v,
		filepath.Join(root, "foo", config.DefaultTerragruntConfigPath),
	)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "bar-state", remoteState.BackendConfig["bucket"])
}

func TestParseRemoteStateFromIncludedConfig(t *testing.T) {
	t.Parallel()

	v, root := unitsVenv(t, map[string]string{
		"root.hcl": `
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
`,
		filepath.Join("bar", config.DefaultTerragruntConfigPath): `inputs = {}`,
		filepath.Join("foo", config.DefaultTerragruntConfigPath): `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

dependency "bar" {
  config_path = "../bar"
}

inputs = {
  name = dependency.bar.outputs.name
}
`,
	})

	l := createLogger()
	ctx, pctx := newTestParsingContext(
		t,
		v,
		filepath.Join(root, "foo", config.DefaultTerragruntConfigPath),
	)

	remoteState, err := config.ParseRemoteState(ctx, l, pctx)
	require.NoError(t, err)
	require.NotNil(t, remoteState)

	assert.Equal(t, "root-state", remoteState.BackendConfig["bucket"])
}
