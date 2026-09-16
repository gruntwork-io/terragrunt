package config_test

import (
	"maps"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// memUnitDir is the directory the configuration each test parses sits in,
	// and the directory the vendored functions resolve relative paths against.
	memUnitDir = "/mem/unit"

	// memConfigPath is the configuration file each test parses.
	memConfigPath = memUnitDir + "/terragrunt.hcl"
)

// memUnitFS returns an in-memory filesystem holding a unit directory with
// files, each path relative to [memUnitDir].
func memUnitFS(t *testing.T, files map[string]string) vfs.FS {
	t.Helper()

	withConfig := map[string]string{"terragrunt.hcl": ""}
	maps.Copy(withConfig, files)

	return venvtest.NewFS(t, memUnitDir, withConfig)
}

func TestHCLResolvesAVendoredOnlyFunction(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithFS(memUnitFS(t, nil))
	ctx, pctx := newTestParsingContext(t, v, memConfigPath)
	ctx = config.WithConfigValues(ctx)

	const hcl = `locals {
  encoded = urlencode("foo/bar")
}`

	out, err := config.ParseConfigString(ctx, pctx, l, memConfigPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "foo%2Fbar", out.Locals["encoded"])
}

func TestHCLFileReadsThroughTheVenv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithFS(memUnitFS(t, map[string]string{
		"data.txt": "from the in-memory filesystem\n",
	}))
	ctx, pctx := newTestParsingContext(t, v, memConfigPath)
	ctx = config.WithConfigValues(ctx)

	const hcl = `locals {
  data = file("data.txt")
}`

	out, err := config.ParseConfigString(ctx, pctx, l, memConfigPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "from the in-memory filesystem\n", out.Locals["data"])
}

func TestHCLTemplateFileReadsThroughTheVenv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithFS(memUnitFS(t, map[string]string{
		"greeting.tmpl": "Hello, ${name}! ${urlencode(\"a/b\")}",
	}))
	ctx, pctx := newTestParsingContext(t, v, memConfigPath)
	ctx = config.WithConfigValues(ctx)

	const hcl = `locals {
  greeting = templatefile("greeting.tmpl", { name = "world" })
}`

	out, err := config.ParseConfigString(ctx, pctx, l, memConfigPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "Hello, world! a%2Fb", out.Locals["greeting"])
}

func TestHCLBase64GzipKeepsTheLegacyOutput(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.New().WithFS(memUnitFS(t, nil))
	ctx, pctx := newTestParsingContext(t, v, memConfigPath)
	ctx = config.WithConfigValues(ctx)

	const hcl = `locals {
  encoded = base64gzip("test")
}`

	out, err := config.ParseConfigString(ctx, pctx, l, memConfigPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "H4sIAAAAAAAA/ypJLS4BAAAA//8BAAD//wx+f9gEAAAA", out.Locals["encoded"])
}
