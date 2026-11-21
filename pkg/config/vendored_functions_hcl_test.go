package config_test

import (
	"maps"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/upstream/lang/funcs"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/sync/errgroup"
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
	v := venv.MemVenv().WithFS(memUnitFS(t, nil))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithCaches(ctx)

	const hcl = `locals {
  encoded = urlencode("foo/bar")
}`

	out, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, hcl)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "foo%2Fbar", out.Locals["encoded"])
}

func TestHCLFileReadsThroughTheVenv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venv.MemVenv().WithFS(memUnitFS(t, map[string]string{
		"data.txt": "from the in-memory filesystem\n",
	}))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithCaches(ctx)

	const hcl = `locals {
  data = file("data.txt")
}`

	out, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, hcl)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "from the in-memory filesystem\n", out.Locals["data"])
}

func TestHCLTemplateFileReadsThroughTheVenv(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venv.MemVenv().WithFS(memUnitFS(t, map[string]string{
		"greeting.tmpl": "Hello, ${name}! ${urlencode(\"a/b\")}",
	}))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithCaches(ctx)

	const hcl = `locals {
  greeting = templatefile("greeting.tmpl", { name = "world" })
}`

	out, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, hcl)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "Hello, world! a%2Fb", out.Locals["greeting"])
}

func TestHCLTemplateStringRendersWithOpenTofuFunctions(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venv.MemVenv().WithFS(memUnitFS(t, nil))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithCaches(ctx)

	const hcl = `locals {
  greeting = templatestring("Hello, $${upper(name)}!", { name = "world" })
}`

	out, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, hcl)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, "Hello, WORLD!", out.Locals["greeting"])
}

func TestHCLTemplateFileCannotCallTerragruntFunctions(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venv.MemVenv().WithFS(memUnitFS(t, map[string]string{
		"dir.tmpl": "${get_terragrunt_dir()}",
	}))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithCaches(ctx)

	const hcl = `locals {
  dir = templatefile("dir.tmpl", {})
}`

	_, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, hcl)
	require.Error(t, err)
}

func TestHCLTemplatesCallFunctionsTheConfigDoesNotCall(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		hcl  string
	}{
		{name: "templatefile", hcl: `locals {
  data = templatefile("data.tmpl", {})
}`},
		{name: "templatestring", hcl: `locals {
  data = templatestring(file("data.tmpl"), {})
}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l := logger.CreateLogger()
			v := venv.MemVenv().WithFS(memUnitFS(t, map[string]string{
				"data.tmpl": `${filebase64("data.txt")}`,
				"data.txt":  "from a file the config never names",
			}))
			ctx, pctx := newTestParsingContext(t, memConfigPath)
			ctx = config.WithCaches(ctx)

			out, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, tc.hcl)
			require.NoError(t, err)
			require.NotNil(t, out.Locals)
			assert.Equal(t, "ZnJvbSBhIGZpbGUgdGhlIGNvbmZpZyBuZXZlciBuYW1lcw==", out.Locals["data"])
		})
	}
}

func TestHCLUnknownFunctionSuggestsATerragruntFunction(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venv.MemVenv().WithFS(memUnitFS(t, nil))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithCaches(ctx)

	const content = `locals {
  dir = get_terragrunt_dri()
}`

	_, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, content)

	var diags hcl.Diagnostics
	require.ErrorAs(t, err, &diags)
	require.NotEmpty(t, diags)
	assert.Equal(t, "Call to unknown function", diags[0].Summary)
	assert.Contains(t, diags[0].Detail, `Did you mean "get_terragrunt_dir"?`)
}

func TestHCLConcurrentParsesCallSharedAndBoundFunctionsWithRacing(t *testing.T) {
	t.Parallel()

	const parses = 8

	l := logger.CreateLogger()
	v := venv.MemVenv().WithFS(memUnitFS(t, map[string]string{
		"greeting.tmpl": "Hello, ${upper(name)}!",
	}))

	const hcl = `locals {
  greeting = templatefile("greeting.tmpl", { name = "world" })
  encoded  = urlencode(get_terragrunt_dir())
}`

	g, gctx := errgroup.WithContext(t.Context())

	for range parses {
		ctx, pctx := newTestParsingContext(t, memConfigPath)
		ctx = config.WithCaches(ctx)

		g.Go(func() error {
			if err := gctx.Err(); err != nil {
				return err
			}

			out, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, hcl)
			if err != nil {
				return err
			}

			assert.Equal(t, "Hello, WORLD!", out.Locals["greeting"])
			assert.Equal(t, url.QueryEscape(filepath.FromSlash(memUnitDir)), out.Locals["encoded"])

			return nil
		})
	}

	require.NoError(t, g.Wait())
}

func TestHCLBase64GzipUsesTheCurrentEncoder(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venv.MemVenv().WithFS(memUnitFS(t, nil))
	ctx, pctx := newTestParsingContext(t, memConfigPath)
	ctx = config.WithCaches(ctx)

	const hcl = `locals {
  encoded = base64gzip("test")
}`

	// Computed rather than written out, so that a Go release changing the bytes
	// its gzip encoder produces does not turn into a failure here.
	expected, err := funcs.Base64Gzip(cty.StringVal("test"))
	require.NoError(t, err)

	out, err := resolveRunConfigString(t, ctx, l, v, new(config.Store), pctx, memConfigPath, hcl)
	require.NoError(t, err)
	require.NotNil(t, out.Locals)
	assert.Equal(t, expected.AsString(), out.Locals["encoded"])
}
