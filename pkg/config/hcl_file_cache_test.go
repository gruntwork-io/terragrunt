package config_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

const (
	// hclFileCacheParent is the shared parent every unit in these tests includes.
	hclFileCacheParent = `
locals {
  account = "acme"
  region  = "us-east-1"
}

inputs = {
  bucket = "${local.account}-${local.region}-state"
}
`

	// hclFileCacheParentFile is the file name the parent is written to.
	hclFileCacheParentFile = "root.hcl"

	// hclFileCacheBrokenParent parses only when a diagnostics handler drops the error the
	// unterminated attribute raises.
	hclFileCacheBrokenParent = `
locals {
  good = "ok"
  bad  =
}
`
)

// TestHCLFileCacheSharedParentParsedOncePerRun pins the point of keying an entry on the file
// rather than on the unit reading it: a second unit that includes the same parent parses only
// its own config.
func TestHCLFileCacheSharedParentParsedOncePerRun(t *testing.T) {
	t.Parallel()

	_, unitPaths := writeHCLFileCacheTree(t, hclFileCacheParent, 2)

	ctx, hclCache, v := newHCLFileCacheContext(t, unitPaths[0])
	l := logger.CreateLogger()

	first, err := config.ParseConfigFile(ctx, newUnitParsingContext(t, v, unitPaths[0]), l, unitPaths[0], nil)
	require.NoError(t, err)

	afterFirst := hclCache.Misses()
	assert.Equal(t, int64(2), afterFirst, "the first unit parses its own config and the parent")

	second, err := config.ParseConfigFile(ctx, newUnitParsingContext(t, v, unitPaths[1]), l, unitPaths[1], nil)
	require.NoError(t, err)

	assert.Equal(
		t,
		afterFirst+1,
		hclCache.Misses(),
		"the second unit parses only its own config",
	)
	assert.Equal(t, first.Inputs["bucket"], second.Inputs["bucket"])
}

// TestHCLFileCacheSharedByFullAndPartialParse pins what unifying the key format bought: the
// entry a partial parse stores is the entry a full parse of the same file reads.
func TestHCLFileCacheSharedByFullAndPartialParse(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(helpers.TmpDirWOSymlinks(t), config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(configPath, []byte(hclFileCacheParent), 0644))

	ctx, hclCache, v := newHCLFileCacheContext(t, configPath)
	l := logger.CreateLogger()

	pctx := newUnitParsingContext(t, v, configPath)

	_, err := config.PartialParseConfigFile(ctx, pctx.WithDecodeList(config.DependenciesBlock), l, configPath, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), hclCache.Misses())

	parsed, err := config.ParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err)

	assert.Equal(t, int64(1), hclCache.Misses(), "a full parse reuses what a partial parse stored")
	assert.Positive(t, hclCache.Hits())
	assert.Equal(t, "acme-us-east-1-state", parsed.Inputs["bucket"])
}

// TestHCLFileCacheEditedParentReparsed verifies that an entry describes the content it was
// parsed from, so a unit reading an edited parent gets the edit.
func TestHCLFileCacheEditedParentReparsed(t *testing.T) {
	t.Parallel()

	parentPath, unitPaths := writeHCLFileCacheTree(t, hclFileCacheParent, 2)

	ctx, hclCache, v := newHCLFileCacheContext(t, unitPaths[0])
	l := logger.CreateLogger()

	before, err := config.ParseConfigFile(ctx, newUnitParsingContext(t, v, unitPaths[0]), l, unitPaths[0], nil)
	require.NoError(t, err)
	require.Equal(t, "acme-us-east-1-state", before.Inputs["bucket"])

	edited := `
locals {
  account = "acme"
  region  = "eu-west-2"
}

inputs = {
  bucket = "${local.account}-${local.region}-state"
}
`
	require.NoError(t, os.WriteFile(parentPath, []byte(edited), 0644))

	afterEdit := hclCache.Misses()

	after, err := config.ParseConfigFile(ctx, newUnitParsingContext(t, v, unitPaths[1]), l, unitPaths[1], nil)
	require.NoError(t, err)

	assert.Equal(t, "acme-eu-west-2-state", after.Inputs["bucket"])
	assert.Equal(t, afterEdit+2, hclCache.Misses(), "the edited parent and the second unit are both parsed")
}

// TestHCLFileCacheRefusesSuppressedParse pins the guard that lets one entry serve every reader:
// a parse that only succeeded because a diagnostics handler dropped the file's error is never
// stored, so a reader without that handler still sees the error.
func TestHCLFileCacheRefusesSuppressedParse(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(helpers.TmpDirWOSymlinks(t), config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(configPath, []byte(hclFileCacheBrokenParent), 0644))

	ctx, hclCache, v := newHCLFileCacheContext(t, configPath)
	l := logger.CreateLogger()

	pctx := newUnitParsingContext(t, v, configPath)

	suppressing := suppressingDiagnostics(pctx)

	_, err := config.PartialParseConfigFile(
		ctx,
		suppressing.WithDecodeList(config.DependenciesBlock),
		l,
		configPath,
		nil,
	)
	require.NoError(t, err, "the handler drops the parse error, which is the situation being guarded")

	_, err = config.ParseConfigFile(ctx, pctx, l, configPath, nil)
	require.Error(t, err, "a reader without the handler must still see the file's error")

	assert.Zero(t, hclCache.Hits(), "a suppressed parse must not be stored for anyone to hit")
}

// TestHCLFileCacheKeepsSoundParseUnderDiagnosticsHandler is the other half of the guard: a file
// that raises no diagnostics parses the same way for everyone, so discovery, which reads every
// unit under a handler of its own, still leaves the run's parses something to read.
func TestHCLFileCacheKeepsSoundParseUnderDiagnosticsHandler(t *testing.T) {
	t.Parallel()

	configPath := filepath.Join(helpers.TmpDirWOSymlinks(t), config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(configPath, []byte(hclFileCacheParent), 0644))

	ctx, hclCache, v := newHCLFileCacheContext(t, configPath)
	l := logger.CreateLogger()

	pctx := newUnitParsingContext(t, v, configPath)

	suppressing := suppressingDiagnostics(pctx)

	_, err := config.PartialParseConfigFile(
		ctx,
		suppressing.WithDecodeList(config.DependenciesBlock),
		l,
		configPath,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, int64(1), hclCache.Misses())

	_, err = config.ParseConfigFile(ctx, pctx, l, configPath, nil)
	require.NoError(t, err)

	assert.Equal(t, int64(1), hclCache.Misses(), "a sound file is parsed once for both readers")
}

// TestHCLFileCacheEntryNotRewrittenByDecode pins that decoding an entry leaves it alone. The
// first decode of a bare `include {}` rewrites the wrapper it holds into a labelled one, which
// would otherwise replace the AST every other unit is reading.
func TestHCLFileCacheEntryNotRewrittenByDecode(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpDir, hclFileCacheParentFile), []byte(hclFileCacheParent), 0644),
	)

	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))

	unitPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	unitContent := `
include {
  path = "../` + hclFileCacheParentFile + `"
}
`
	require.NoError(t, os.WriteFile(unitPath, []byte(unitContent), 0644))

	ctx, hclCache, v := newHCLFileCacheContext(t, unitPath)
	l := logger.CreateLogger()

	_, err := config.ParseConfigFile(ctx, newUnitParsingContext(t, v, unitPath), l, unitPath, nil)
	require.NoError(t, err)

	cached, found := hclCache.Get(ctx, config.HCLFileCacheKey(unitPath, []byte(unitContent)))
	require.True(t, found)
	assert.Equal(t, unitContent, cached.Content(), "decoding must not rewrite the cached AST")
}

// TestHCLFileCacheSharedParentWithRacing parses every unit of a shared parent at once, which is
// what a run does. Every unit reads the one entry the parent's first reader stored.
func TestHCLFileCacheSharedParentWithRacing(t *testing.T) {
	t.Parallel()

	const units = 8

	_, unitPaths := writeHCLFileCacheTree(t, hclFileCacheParent, units)

	ctx, _, v := newHCLFileCacheContext(t, unitPaths[0])
	l := logger.CreateLogger()

	parsed := make([]*config.TerragruntConfig, len(unitPaths))

	var group errgroup.Group

	for i, unitPath := range unitPaths {
		group.Go(func() error {
			cfg, err := config.ParseConfigFile(ctx, newUnitParsingContext(t, v, unitPath), l, unitPath, nil)
			if err != nil {
				return err
			}

			parsed[i] = cfg

			return nil
		})
	}

	require.NoError(t, group.Wait())

	for i, cfg := range parsed {
		require.NotNil(t, cfg)
		assert.Equal(t, "acme-us-east-1-state", cfg.Inputs["bucket"])
		assert.Equal(t, "unit-"+strconv.Itoa(i), cfg.Inputs["unit"])
	}
}

// TestHCLFileCacheBareIncludeWithRacing parses one unit carrying a bare `include {}` from many
// goroutines at once. The first decode to reach it rewrites the include into a labelled one, on
// a wrapper of its own, while the rest are reading the entry it was parsed from.
func TestHCLFileCacheBareIncludeWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	require.NoError(
		t,
		os.WriteFile(filepath.Join(tmpDir, hclFileCacheParentFile), []byte(hclFileCacheParent), 0644),
	)

	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0755))

	unitPath := filepath.Join(unitDir, config.DefaultTerragruntConfigPath)
	require.NoError(t, os.WriteFile(unitPath, []byte(`
include {
  path = "../`+hclFileCacheParentFile+`"
}
`), 0644))

	ctx, _, v := newHCLFileCacheContext(t, unitPath)
	l := logger.CreateLogger()

	const readers = 16

	parsed := make([]*config.TerragruntConfig, readers)

	var group errgroup.Group

	for i := range readers {
		group.Go(func() error {
			cfg, err := config.ParseConfigFile(ctx, newUnitParsingContext(t, v, unitPath), l, unitPath, nil)
			if err != nil {
				return err
			}

			parsed[i] = cfg

			return nil
		})
	}

	require.NoError(t, group.Wait())

	for _, cfg := range parsed {
		require.NotNil(t, cfg)
		assert.Equal(t, "acme-us-east-1-state", cfg.Inputs["bucket"])
	}
}

// writeHCLFileCacheTree writes units units that all include one parent config holding parent,
// and returns the parent's path and theirs.
func writeHCLFileCacheTree(t *testing.T, parent string, units int) (string, []string) {
	t.Helper()

	dir := helpers.TmpDirWOSymlinks(t)
	parentPath := filepath.Join(dir, hclFileCacheParentFile)
	require.NoError(t, os.WriteFile(parentPath, []byte(parent), 0644))

	unitPaths := make([]string, units)

	for i := range units {
		unitDir := filepath.Join(dir, "unit-"+strconv.Itoa(i))
		require.NoError(t, os.MkdirAll(unitDir, 0755))

		unitPaths[i] = filepath.Join(unitDir, config.DefaultTerragruntConfigPath)

		unit := `
include "root" {
  path = "../` + hclFileCacheParentFile + `"
}

inputs = {
  unit = "unit-` + strconv.Itoa(i) + `"
}
`
		require.NoError(t, os.WriteFile(unitPaths[i], []byte(unit), 0644))
	}

	return parentPath, unitPaths
}

// newHCLFileCacheContext returns a context carrying the caches a run installs, the HCL file
// cache on it, and the venv every unit parsed against it must use.
func newHCLFileCacheContext(
	t *testing.T,
	configPath string,
) (context.Context, *config.HCLFileCache, *venv.Venv) {
	t.Helper()

	v := venvtest.NewWithOSFS()
	baseCtx, _ := newTestParsingContext(t, v, configPath)
	ctx := config.WithConfigValues(baseCtx)

	return ctx, config.ContextHCLFileCache(ctx), v
}

// newUnitParsingContext returns the parsing context a run gives one unit: its own, while the
// caches it reads live on the context shared by the whole run.
func newUnitParsingContext(t *testing.T, v *venv.Venv, configPath string) *config.ParsingContext {
	t.Helper()

	_, pctx := newTestParsingContext(t, v, configPath)

	return pctx
}

// suppressingDiagnostics returns pctx carrying the swallow-everything handler discovery installs
// by default, under which a file that does not parse yields whatever the parser recovered.
func suppressingDiagnostics(pctx *config.ParsingContext) *config.ParsingContext {
	return pctx.WithParseOption(slices.Concat(
		pctx.ParserOptions,
		[]hclparse.Option{
			hclparse.WithDiagnosticsHandler(
				func(_ *hcl.File, _ hcl.Diagnostics) (hcl.Diagnostics, error) { return nil, nil },
			),
		},
	))
}
