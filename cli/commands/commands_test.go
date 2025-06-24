package commands

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupNativeProviderCache_RequiresOpenTofu(t *testing.T) {
	t.Parallel()

	opts := &options.TerragruntOptions{
		TerraformImplementation: options.TerraformImpl,
		TerraformVersion:        mustParseVersion("1.11.0"),
	}

	l := logger.CreateLogger()
	err := setupNativeProviderCache(l, opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "native provider cache requires OpenTofu")
}

func TestSetupNativeProviderCache_RequiresVersion1_10_Plus(t *testing.T) {
	t.Parallel()

	opts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
		TerraformVersion:        mustParseVersion("1.9.0"),
	}

	l := logger.CreateLogger()
	err := setupNativeProviderCache(l, opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "native provider cache requires OpenTofu version > 1.10")
}

func TestSetupNativeProviderCache_Success(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	opts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
		TerraformVersion:        mustParseVersion("1.11.0"),
		ProviderCacheDir:        tmpDir,
		Env:                     make(map[string]string),
	}

	l := logger.CreateLogger()
	err := setupNativeProviderCache(l, opts)

	require.NoError(t, err)
	assert.Equal(t, tmpDir, opts.Env[tf.EnvNameTFPluginCacheDir])
}

func TestSetupNativeProviderCache_DefaultCacheDir(t *testing.T) {
	t.Parallel()

	opts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
		TerraformVersion:        mustParseVersion("1.11.0"),
		Env:                     make(map[string]string),
	}

	l := logger.CreateLogger()
	err := setupNativeProviderCache(l, opts)

	require.NoError(t, err)
	assert.Contains(t, opts.Env[tf.EnvNameTFPluginCacheDir], "providers")
}

func TestSetupNativeProviderCache_NilVersion(t *testing.T) {
	t.Parallel()

	opts := &options.TerragruntOptions{
		TerraformImplementation: options.OpenTofuImpl,
		TerraformVersion:        nil,
	}

	l := logger.CreateLogger()
	err := setupNativeProviderCache(l, opts)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot determine OpenTofu version")
}

func mustParseVersion(v string) *version.Version {
	ver, err := version.NewVersion(v)
	if err != nil {
		panic(err)
	}
	return ver
}
