package config_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/sync/errgroup"

	config "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// includingRepo lays out root.hcl, common.yaml, and n units that include the root and read the YAML.
func includingRepo(tb testing.TB, root string, n int) (vfs.FS, []string) {
	tb.Helper()

	files := map[string]string{
		"root.hcl": `
locals {
  common = yamldecode(file("${get_parent_terragrunt_dir()}/common.yaml"))
  unit   = basename(get_terragrunt_dir())
}

inputs = {
  region = local.common.region
  unit   = local.unit
}
`,
		"common.yaml": "region: us-east-1\n",
	}

	paths := make([]string, 0, n)

	for i := range n {
		rel := filepath.Join(fmt.Sprintf("unit-%d", i), pkgconfig.DefaultTerragruntConfigPath)
		files[rel] = `
include "root" {
  path = find_in_parent_folders("root.hcl")
}

locals {
  name   = "unit"
  region = yamldecode(file(find_in_parent_folders("common.yaml"))).region
}

inputs = {
  name = local.name
}
`
		paths = append(paths, filepath.Join(root, rel))
	}

	fsys := vfs.NewMemMapFS()

	for rel, content := range files {
		require.NoError(tb, vfs.WriteFile(fsys, filepath.Join(root, rel), []byte(content), 0o644))
	}

	return fsys, paths
}

// parseForBenchmark parses path through store with a fresh context and fails b on any error.
func parseForBenchmark(b *testing.B, l log.Logger, v *venv.Venv, store *config.Store, path string) {
	b.Helper()

	ctx, run := newTestParsingContext(b, path)

	_, err := store.ResolveUnitConfig(ctx, l, v, config.NewParseContext(run), path)
	require.NoError(b, err)
}

func TestResolveUnitConfigSharesTheParse(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestParsingContext(t, paths[0])
	run = run.WithFileReadTracking()

	firstCfg, err := store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), paths[0])
	require.NoError(t, err)
	assert.Equal(
		t,
		config.Stats{Hits: 0, Misses: 6},
		store.Stats(),
		"two files, each at three stages",
	)

	secondCtx, secondRun := newTestParsingContext(t, paths[0])
	secondRun = secondRun.WithFileReadTracking()

	secondCfg, err := store.ResolveUnitQueueConfig(
		secondCtx,
		l,
		v,
		config.NewParseContext(secondRun),
		paths[0],
	)
	require.NoError(t, err)
	assert.Equal(
		t,
		config.Stats{Hits: 1, Misses: 6},
		store.Stats(),
		"the unit's parse is served whole",
	)

	assert.Equal(t, firstCfg, secondCfg)
	assert.NotSame(t, firstCfg, secondCfg, "each caller gets a config of its own")
	assert.Equal(t, "us-east-1", secondCfg.Locals["region"])
	assert.Equal(t, run.FilesRead.Paths(), secondRun.FilesRead.Paths())
	assert.Contains(t, secondRun.FilesRead.Paths(), filepath.Join(root, "common.yaml"))

	firstPC := config.NewParseContext(run)

	first, err := store.ResolveUnitConfig(ctx, l, v, firstPC, paths[0])
	require.NoError(t, err)

	secondPC := config.NewParseContext(secondRun)

	second, err := store.ResolveUnitConfig(secondCtx, l, v, secondPC, paths[0])
	require.NoError(t, err)
	assert.Same(t, first, second, "callers share one parse")

	firstV1, err := first.ToV1(ctx, l, v, firstPC)
	require.NoError(t, err)

	secondV1, err := second.ToV1(secondCtx, l, v, secondPC)
	require.NoError(t, err)

	assert.Equal(t, firstV1, secondV1)
	assert.Equal(
		t,
		map[string]any{"name": "unit", "region": "us-east-1", "unit": "unit-0"},
		secondV1.Inputs,
	)
}

func TestResolveUnitQueueConfigReturnsPartialConfigWithParseError(t *testing.T) {
	t.Parallel()

	fsys, path := missingIncludeRepo(t)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()

	ctx, run := newTestParsingContext(t, path)

	cfg, err := new(
		config.Store,
	).ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), path)
	require.ErrorAs(t, err, new(pkgconfig.IncludeConfigNotFoundError))
	require.NotNil(t, cfg)
	assert.Equal(t, "us-east-1", cfg.Locals["region"])
}

func TestResolveUnitQueueConfigGivesCallersOfAFailedParseTheirOwnConfigWithRacing(t *testing.T) {
	t.Parallel()

	const callers = 8

	fsys, path := missingIncludeRepo(t)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()

	for range 50 {
		store := &config.Store{}
		cfgs := make([]*pkgconfig.TerragruntConfig, callers)

		var g errgroup.Group

		for i := range callers {
			g.Go(func() error {
				ctx, run := newTestParsingContext(t, path)

				cfg, err := store.ResolveUnitQueueConfig(
					ctx,
					l,
					v,
					config.NewParseContext(run),
					path,
				)
				assert.ErrorAs(t, err, new(pkgconfig.IncludeConfigNotFoundError))

				cfgs[i] = cfg

				return nil
			})
		}

		require.NoError(t, g.Wait())

		seen := make(map[*pkgconfig.TerragruntConfig]struct{}, callers)

		for _, cfg := range cfgs {
			require.NotNil(t, cfg)
			seen[cfg] = struct{}{}
		}

		assert.Len(t, seen, callers)
	}
}

// dependencyReadsRepo returns a filesystem holding a unit whose dependency includes a file, the unit's config path, and
// the path of the included file, which only the dependency reads.
func dependencyReadsRepo(t *testing.T, root string) (vfs.FS, string, string) {
	t.Helper()

	path := filepath.Join(root, "app", pkgconfig.DefaultTerragruntConfigPath)
	readByDependency := filepath.Join(root, "dep.hcl")

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, readByDependency, []byte("locals {}\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
dependency "dep" {
  config_path  = "../dep"
  skip_outputs = true
}
`), 0o644))
	require.NoError(t, vfs.WriteFile(
		fsys,
		filepath.Join(root, "dep", pkgconfig.DefaultTerragruntConfigPath),
		[]byte("include \"dep\" {\n  path = find_in_parent_folders(\"dep.hcl\")\n}\n"),
		0o644,
	))

	return fsys, path, readByDependency
}

// missingIncludeRepo returns a filesystem holding a unit that includes a file that does not exist, and the unit's
// config path.
func missingIncludeRepo(t *testing.T) (vfs.FS, string) {
	t.Helper()

	root := t.TempDir()
	path := filepath.Join(root, pkgconfig.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
include {
  path = "`+filepath.Join(root, "missing.hcl")+`"
}

locals {
  region = "us-east-1"
}
`), 0o644))

	return fsys, path
}

func TestToV1WritesToTheSinksOfItsCallerWithRacing(t *testing.T) {
	t.Parallel()

	const callers = 8

	root := t.TempDir()
	path := filepath.Join(root, "unit", pkgconfig.DefaultTerragruntConfigPath)
	readByInputs := filepath.Join(root, "unit", "read-by-inputs.txt")

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "root.hcl"), []byte("locals {}\n"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, readByInputs, []byte("content"), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
include "root" {
  path = find_in_parent_folders("root.hcl")
}

inputs = {
  content = file("${get_terragrunt_dir()}/read-by-inputs.txt")
}
`), 0o644))

	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()

	ctx, parseRun := newTestParsingContext(t, path)
	parseRun = parseRun.WithFileReadTracking()

	parsed, err := new(config.Store).ResolveUnitConfig(ctx, l, v, config.NewParseContext(parseRun), path)
	require.NoError(t, err)

	var g errgroup.Group

	for range callers {
		g.Go(func() error {
			runCtx, run := newTestParsingContext(t, path)
			run = run.WithFileReadTracking()

			if _, err := parsed.ToV1(runCtx, l, v, config.NewParseContext(run)); err != nil {
				return err
			}

			assert.Contains(t, run.FilesRead.Paths(), readByInputs)
			assert.Contains(t, run.FilesRead.Paths(), filepath.Join(root, "root.hcl"))

			return nil
		})
	}

	require.NoError(t, g.Wait())
	assert.NotContains(t, parseRun.FilesRead.Paths(), readByInputs)
}

func TestResolveUnitConfigSendsDiagnosticsToItsCallersHandler(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), pkgconfig.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte("terraform_binary = local.missing\n"), 0o644))

	v := venvtest.New().WithFS(fsys)
	ctx, run := newTestParsingContext(t, path)

	var received hcl.Diagnostics

	run.Parser.DiagnosticsHandler = func(_ *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
		received = append(received, diags...)

		return diags, nil
	}

	_, err := new(config.Store).ResolveUnitConfig(ctx, logger.CreateLogger(), v, config.NewParseContext(run), path)
	require.Error(t, err)
	assert.NotEmpty(t, received)
}

func TestResolveUnitQueueConfigDecodesTheEngine(t *testing.T) {
	t.Parallel()

	const unit = `
dependency "vpc" {
  config_path = "../vpc"
  skip_outputs = true
}

engine {
  source = "github.com/example/engine"
  meta = {
    region = "us-east-1"
    vpc_id = dependency.vpc.outputs.id
  }
}
`

	suppliedVPC := cty.ObjectVal(map[string]cty.Value{
		"vpc": cty.ObjectVal(map[string]cty.Value{
			"outputs": cty.ObjectVal(map[string]cty.Value{"id": cty.StringVal("vpc-from-run")}),
		}),
	})

	testCases := []struct {
		supplied *cty.Value
		want     cty.Value
		name     string
	}{
		{name: "an output is unknown", want: cty.DynamicVal},
		{name: "a value the run supplied is kept", supplied: &suppliedVPC, want: cty.StringVal("vpc-from-run")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "app", pkgconfig.DefaultTerragruntConfigPath)

			fsys := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fsys, path, []byte(unit), 0o644))

			v := venvtest.New().WithFS(fsys)
			ctx, run := newTestParsingContext(t, path)
			run.DecodedDependencies = tc.supplied

			cfg, err := new(config.Store).ResolveUnitQueueConfig(
				ctx,
				logger.CreateLogger(),
				v,
				config.NewParseContext(run),
				path,
			)
			require.NoError(t, err)
			require.NotNil(t, cfg.Engine)
			assert.Equal(t, "github.com/example/engine", cfg.Engine.Source)

			meta := cfg.Engine.Meta.AsValueMap()
			assert.Equal(t, "us-east-1", meta["region"].AsString())
			assert.True(t, tc.want.RawEquals(meta["vpc_id"]), "vpc_id is %#v", meta["vpc_id"])
		})
	}
}

func TestResolveUnitQueueConfigKeepsMergedSignalsApartWithRacing(t *testing.T) {
	t.Parallel()

	const callers = 16

	root := t.TempDir()
	path := filepath.Join(root, "app", pkgconfig.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, "root.hcl"), []byte(`
errors {
  ignore "x" {
    ignorable_errors = [".*"]
    signals          = { from_root = "root" }
  }
}
`), 0o644))
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
include "root" {
  path           = find_in_parent_folders("root.hcl")
  merge_strategy = "deep"
}

errors {
  ignore "x" {
    ignorable_errors = [".*"]
    signals          = { from_unit = "unit" }
  }
}
`), 0o644))

	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestParsingContext(t, path)

	_, err := store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), path)
	require.NoError(t, err)

	hit, err := store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), path)
	require.NoError(t, err)
	clear(hit.Errors.Ignore[0].Signals)

	var g errgroup.Group

	for range callers {
		g.Go(func() error {
			callerCtx, callerRun := newTestParsingContext(t, path)

			cfg, err := store.ResolveUnitQueueConfig(callerCtx, l, v, config.NewParseContext(callerRun), path)
			if err != nil {
				return err
			}

			assert.Len(t, cfg.Errors.Ignore[0].Signals, 2)

			return nil
		})
	}

	require.NoError(t, g.Wait())
}

func TestResolveRunConfigReportsAnIncludeTheParseStoppedBefore(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "app", pkgconfig.DefaultTerragruntConfigPath)

	files := map[string]string{
		"a.hcl": "locals {}\n",
		"r.hcl": `
dependency "x" {
  config_path  = "../x"
  skip_outputs = true
  mock_outputs = { v = dependency.y.outputs.z }
}
`,
		"x/terragrunt.hcl": "",
		"app/terragrunt.hcl": `
include "a" {
  path = find_in_parent_folders("a.hcl")
}

include "r" {
  path = find_in_parent_folders("r.hcl")
}
`,
	}

	fsys := vfs.NewMemMapFS()

	for rel, content := range files {
		require.NoError(t, vfs.WriteFile(fsys, filepath.Join(root, rel), []byte(content), 0o644))
	}

	v := venvtest.New().WithFS(fsys)
	ctx, run := newTestParsingContext(t, path)

	cfg, err := new(config.Store).ResolveRunConfig(ctx, logger.CreateLogger(), v, config.NewParseContext(run), path)

	var unparsed config.UnparsedIncludeError

	require.ErrorAs(t, err, &unparsed)
	assert.Equal(t, filepath.Join(root, "a.hcl"), unparsed.IncludePath)
	assert.NotNil(t, cfg)
}

func TestResolveRunConfigAddsTheRunPartsToTheSharedParse(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestParsingContext(t, paths[0])

	queueCfg, err := store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), paths[0])
	require.NoError(t, err)
	assert.Nil(t, queueCfg.Inputs)

	runCfg, err := store.ResolveRunConfig(ctx, l, v, config.NewParseContext(run), paths[0])
	require.NoError(t, err)
	assert.Equal(
		t,
		map[string]any{"name": "unit", "region": "us-east-1", "unit": "unit-0"},
		runCfg.Inputs,
	)
	assert.Equal(t, config.Stats{Hits: 1, Misses: 6}, store.Stats())
}

func TestResolveRunConfigReturnsNoConfigForAMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), pkgconfig.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(vfs.NewMemMapFS())
	ctx, run := newTestParsingContext(t, path)

	cfg, err := new(
		config.Store,
	).ResolveRunConfig(ctx, logger.CreateLogger(), v, config.NewParseContext(run), path)
	require.ErrorAs(t, err, new(pkgconfig.TerragruntConfigNotFoundError))
	assert.Nil(t, cfg)
}

func TestDependencyReadsReachOnlyARun(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, path, readByDependency := dependencyReadsRepo(t, root)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, queueRun := newTestParsingContext(t, path)
	queueRun = queueRun.WithFileReadTracking()

	_, err := store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(queueRun), path)
	require.NoError(t, err)
	assert.NotContains(t, queueRun.FilesRead.Paths(), readByDependency)

	runCtx, run := newTestParsingContext(t, path)
	run = run.WithFileReadTracking()

	_, err = store.ResolveRunConfig(runCtx, l, v, config.NewParseContext(run), path)
	require.NoError(t, err)
	assert.Contains(t, run.FilesRead.Paths(), readByDependency)
}

func TestInvalidateFindsAParseByTheFilesItsDependenciesRead(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, path, readByDependency := dependencyReadsRepo(t, root)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	resolve := func() {
		ctx, run := newTestParsingContext(t, path)
		run = run.WithFileReadTracking()

		_, err := store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), path)
		require.NoError(t, err)
	}

	resolve()

	misses := store.Stats().Misses

	resolve()
	assert.Equal(t, misses, store.Stats().Misses, "the parse is served")

	store.Invalidate(readByDependency)
	resolve()
	assert.Greater(t, store.Stats().Misses, misses, "the parse ran again")
}

func TestResolveUnitBaseVariablesStopsBeforeTheDependencyBlocks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), pkgconfig.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
locals {
  region = "us-east-1"
}

dependency "vpc" {
  config_path = local.missing
}
`), 0o644))

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestParsingContext(t, path)

	vars, err := store.ResolveUnitBaseVariables(ctx, l, v, config.NewParseContext(run), path)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", vars[pkgconfig.MetadataLocal].GetAttr("region").AsString())

	_, err = store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), path)
	require.Error(t, err)
}

func TestResolveUnitQueueConfigBuildsOnTheBaseVariables(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestParsingContext(t, paths[0])

	_, err := store.ResolveUnitBaseVariables(ctx, l, v, config.NewParseContext(run), paths[0])
	require.NoError(t, err)
	assert.Equal(t, config.Stats{Hits: 0, Misses: 2}, store.Stats(), "the syntax tree and the base")

	_, err = store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), paths[0])
	require.NoError(t, err)

	stats := store.Stats()
	assert.Equal(t, uint64(2), stats.Hits, "the syntax tree and the base are served")
}

func TestResolveUnitBaseVariablesEvaluatesOnceWithRacing(t *testing.T) {
	t.Parallel()

	const callers = 8

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	var g errgroup.Group

	for range callers {
		g.Go(func() error {
			ctx, run := newTestParsingContext(t, paths[0])
			run = run.WithFileReadTracking()

			vars, err := store.ResolveUnitBaseVariables(ctx, l, v, config.NewParseContext(run), paths[0])
			if err != nil {
				return err
			}

			assert.Equal(t, "unit", vars[pkgconfig.MetadataLocal].GetAttr("name").AsString())
			assert.Contains(t, run.FilesRead.Paths(), filepath.Join(root, "common.yaml"))

			return nil
		})
	}

	require.NoError(t, g.Wait())
	assert.Equal(t, uint64(2), store.Stats().Misses, "the syntax tree and the base")
}

func TestResolveUnitBaseVariablesGivesEachCallerItsOwnMap(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestParsingContext(t, paths[0])

	first, err := store.ResolveUnitBaseVariables(ctx, l, v, config.NewParseContext(run), paths[0])
	require.NoError(t, err)

	delete(first, pkgconfig.MetadataLocal)

	second, err := store.ResolveUnitBaseVariables(ctx, l, v, config.NewParseContext(run), paths[0])
	require.NoError(t, err)
	assert.Contains(t, second, pkgconfig.MetadataLocal)
}

func TestResolveUnitBaseVariablesDoesNotRetainAFailedBase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), pkgconfig.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
locals {
  region = local.missing
}
`), 0o644))

	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	resolve := func() error {
		ctx, run := newTestParsingContext(t, path)

		_, err := store.ResolveUnitBaseVariables(ctx, l, v, config.NewParseContext(run), path)

		return err
	}

	require.Error(t, resolve())

	misses := store.Stats().Misses

	require.Error(t, resolve())
	assert.Equal(t, misses+1, store.Stats().Misses, "the base ran again and the syntax tree was served")
}

func TestResolveUnitBaseVariablesReportsAnEvalContextFailure(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	path := filepath.Join(root, "app", pkgconfig.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(
		fsys,
		filepath.Join(root, "exposed.hcl"),
		[]byte("inputs = {\n  region = local.missing\n}\n"),
		0o644,
	))
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
include "exposed" {
  path   = "../exposed.hcl"
  expose = true
}
`), 0o644))

	v := venvtest.New().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	resolve := func() (map[string]cty.Value, error) {
		ctx, run := newTestParsingContext(t, path)

		return store.ResolveUnitBaseVariables(ctx, l, v, config.NewParseContext(run), path)
	}

	vars, err := resolve()
	require.Error(t, err)
	assert.Nil(t, vars)

	misses := store.Stats().Misses

	_, err = resolve()
	require.Error(t, err)
	assert.Greater(t, store.Stats().Misses, misses, "the failed base was not retained")
}

func TestResolveUnitBaseVariablesReturnsNoVariablesForAMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), pkgconfig.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(vfs.NewMemMapFS())
	ctx, run := newTestParsingContext(t, path)

	vars, err := new(config.Store).ResolveUnitBaseVariables(
		ctx,
		logger.CreateLogger(),
		v,
		config.NewParseContext(run),
		path,
	)
	require.ErrorAs(t, err, new(pkgconfig.TerragruntConfigNotFoundError))
	assert.Nil(t, vars)
}

func TestResolveUnitConfigSharesIncludedFilesAcrossUnits(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 3)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	for _, path := range paths {
		ctx, run := newTestParsingContext(t, path)

		pc := config.NewParseContext(run)

		parsed, err := store.ResolveUnitConfig(ctx, l, v, pc, path)
		require.NoError(t, err)

		cfg, err := parsed.ToV1(ctx, l, v, pc)
		require.NoError(t, err)
		assert.Equal(t, filepath.Base(filepath.Dir(path)), cfg.Inputs["unit"])
	}

	assert.Equal(
		t,
		config.Stats{Hits: 2, Misses: 16},
		store.Stats(),
		"the root's syntax tree is parsed once; each unit evaluates it in its own context",
	)
}

func TestResolveUnitConfigLabelsBareIncludeUnderTwoContextsWithRacing(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys := vfs.NewMemMapFS()
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(root, "root.hcl"), []byte("inputs = { a = 1 }\n"), 0o644),
	)

	unit := filepath.Join(root, "unit", pkgconfig.DefaultTerragruntConfigPath)
	require.NoError(t, vfs.WriteFile(fsys, unit, []byte(`
include {
  path = find_in_parent_folders("root.hcl")
}
`), 0o644))

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()

	for range 20 {
		store := &config.Store{}

		var g errgroup.Group

		for _, workingDir := range []string{filepath.Dir(unit), filepath.Join(root, "other")} {
			g.Go(func() error {
				ctx, run := newTestParsingContext(t, unit)
				run.WorkingDir = workingDir

				cfg, err := store.ResolveUnitQueueConfig(
					ctx,
					l,
					v,
					config.NewParseContext(run),
					unit,
				)
				if err != nil {
					return err
				}

				assert.Len(t, cfg.ProcessedIncludes, 1)

				return nil
			})
		}

		require.NoError(t, g.Wait())
	}
}

func TestResolveUnitConfigParsesAgainAfterInvalidate(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	region := func() any {
		ctx, run := newTestParsingContext(t, paths[0])
		run = run.WithFileReadTracking()

		cfg, err := store.ResolveUnitQueueConfig(ctx, l, v, config.NewParseContext(run), paths[0])
		require.NoError(t, err)

		return cfg.Locals["region"]
	}

	assert.Equal(t, "us-east-1", region())

	common := filepath.Join(root, "common.yaml")
	require.NoError(t, vfs.WriteFile(fsys, common, []byte("region: eu-west-1\n"), 0o644))

	assert.Equal(
		t,
		"us-east-1",
		region(),
		"the unit's parse is served until its reads are invalidated",
	)

	store.Invalidate(common)

	assert.Equal(t, "eu-west-1", region())
}

func TestResolveUnitConfigRunsUnmemoizedForSuppliedValues(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestParsingContext(t, paths[0])
	locals := cty.ObjectVal(map[string]cty.Value{"from_run": cty.StringVal("run")})
	run.Locals = &locals

	for range 2 {
		_, err := store.ResolveUnitConfig(ctx, l, v, config.NewParseContext(run), paths[0])
		require.NoError(t, err)
	}

	assert.Equal(
		t,
		config.Stats{Hits: 2, Misses: 2},
		store.Stats(),
		"only the syntax trees are shared",
	)
}

func TestParseContextStoreContextCoversEachInput(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), pkgconfig.DefaultTerragruntConfigPath)

	_, base := newTestParsingContext(t, cfgPath)
	baseKey, memoized := config.NewParseContext(base).StoreContext()
	require.True(t, memoized)
	require.NotEmpty(t, baseKey)

	keyed := map[string]func(*pkgconfig.ParsingContext){
		"config path":          func(p *pkgconfig.ParsingContext) { p.TerragruntConfigPath += ".other" },
		"original config path": func(p *pkgconfig.ParsingContext) { p.OriginalTerragruntConfigPath += ".other" },
		"working dir":          func(p *pkgconfig.ParsingContext) { p.WorkingDir += "-other" },
		"root working dir":     func(p *pkgconfig.ParsingContext) { p.RootWorkingDir += "-other" },
		"download dir":         func(p *pkgconfig.ParsingContext) { p.DownloadDir += "-other" },
		"source":               func(p *pkgconfig.ParsingContext) { p.Source = "git::other" },
		"source map":           func(p *pkgconfig.ParsingContext) { p.SourceMap = map[string]string{"a": "b"} },
		"command":              func(p *pkgconfig.ParsingContext) { p.TerraformCommand = "apply" },
		"original command":     func(p *pkgconfig.ParsingContext) { p.OriginalTofuCommand = "apply" },
		"cli args":             func(p *pkgconfig.ParsingContext) { p.TerraformCliArgs = iacargs.New("-lock=false") },
		"iam role": func(p *pkgconfig.ParsingContext) {
			p.IAMRoleOptions = iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/a"}
		},
		"original iam role": func(p *pkgconfig.ParsingContext) {
			p.OriginalIAMRoleOptions = iam.RoleOptions{AssumeRoleDuration: 1}
		},
		"feature flags":     func(p *pkgconfig.ParsingContext) { p.FeatureFlags = map[string]string{"f": "1"} },
		"auth provider cmd": func(p *pkgconfig.ParsingContext) { p.AuthProviderCmd = "creds" },
		"diagnostics":       func(p *pkgconfig.ParsingContext) { p.Parser.Diagnostics = pkgconfig.DiagnosticsSuppressed },
		"ignore diagnostics": func(p *pkgconfig.ParsingContext) {
			p.Parser.IgnoreDiagnostics = true
		},
		"bare include rewrite": func(p *pkgconfig.ParsingContext) {
			p.Parser.RewriteBareInclude = !p.Parser.RewriteBareInclude
		},
		"halt only in blocks": func(p *pkgconfig.ParsingContext) {
			p.Parser.HaltOnErrorOnlyInBlocks = []string{"catalog"}
		},
		"skip output":             func(p *pkgconfig.ParsingContext) { p.SkipOutput = true },
		"skip outputs resolution": func(p *pkgconfig.ParsingContext) { p.SkipOutputsResolution = true },
		"parse depth":             func(p *pkgconfig.ParsingContext) { p.ParseDepth = 3 },
		"skip autoinclude merge":  func(p *pkgconfig.ParsingContext) { p.SkipAutoIncludeMerge = true },
		"read tracking":           func(p *pkgconfig.ParsingContext) { p.FilesRead = pkgconfig.NewFilesRead() },
	}

	for name, mutate := range keyed {
		t.Run("keyed/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestParsingContext(t, cfgPath)
			mutate(run)

			key, memoized := config.NewParseContext(run).StoreContext()
			require.True(t, memoized)
			assert.NotEqual(t, baseKey, key)
		})
	}

	fixed := map[string]func(*pkgconfig.ParsingContext){
		"tofu binary":       func(p *pkgconfig.ParsingContext) { p.TFPath = "terraform" },
		"log abs paths":     func(p *pkgconfig.ParsingContext) { p.LogShowAbsPaths = true },
		"debug":             func(p *pkgconfig.ParsingContext) { p.Debug = true },
		"max parent probes": func(p *pkgconfig.ParsingContext) { p.MaxFoldersToCheck = 5 },
		"decode list": func(p *pkgconfig.ParsingContext) {
			p.PartialParseDecodeList = []pkgconfig.PartialDecodeSectionType{pkgconfig.ExcludeBlock}
		},
	}

	for name, mutate := range fixed {
		t.Run("fixed/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestParsingContext(t, cfgPath)
			mutate(run)

			key, memoized := config.NewParseContext(run).StoreContext()
			require.True(t, memoized)
			assert.Equal(t, baseKey, key)
		})
	}

	value := cty.StringVal("v")
	unmemoized := map[string]func(*pkgconfig.ParsingContext){
		"locals":       func(p *pkgconfig.ParsingContext) { p.Locals = &value },
		"values":       func(p *pkgconfig.ParsingContext) { p.Values = &value },
		"features":     func(p *pkgconfig.ParsingContext) { p.Features = &value },
		"dependencies": func(p *pkgconfig.ParsingContext) { p.DecodedDependencies = &value },
		"includes":     func(p *pkgconfig.ParsingContext) { p.TrackInclude = &pkgconfig.TrackInclude{} },
		"diagnostics handler": func(p *pkgconfig.ParsingContext) {
			p.Parser.DiagnosticsHandler = func(*hcl.File, hcl.Diagnostics) (hcl.Diagnostics, error) { return nil, nil }
		},
	}

	for name, mutate := range unmemoized {
		t.Run("unmemoized/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestParsingContext(t, cfgPath)
			mutate(run)

			_, memoized := config.NewParseContext(run).StoreContext()
			assert.False(t, memoized)
		})
	}
}

func TestParityFixturesSharedStoreWithRacing(t *testing.T) {
	t.Parallel()

	fixtures := loadFixtures(t, parityFixtureRoots, parityExcludedDirs)
	store := &config.Store{}

	t.Cleanup(func() {
		assert.Positive(t, store.Stats().Hits)
	})

	for _, pass := range []string{"first", "second"} {
		t.Run(pass, func(t *testing.T) {
			t.Parallel()

			for _, fixture := range fixtures.configs {
				if _, excluded := withOutputExcludedDirs[fixture.name]; excluded {
					continue
				}

				t.Run(fixture.name, func(t *testing.T) {
					t.Parallel()

					assertParity(t, parityCase{
						newFS:     fixtures.newFS,
						cfgPath:   fixture.path,
						configure: func(*pkgconfig.ParsingContext) {},
						store:     store,
					})
				})
			}
		})
	}
}

func BenchmarkResolveUnitConfig(b *testing.B) {
	for _, n := range []int{16, 128} {
		b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
			root := b.TempDir()
			fsys, paths := includingRepo(b, root, n)
			v := venv.MemVenv().WithFS(fsys)
			l := logger.CreateLogger()

			b.Run("store_per_unit", func(b *testing.B) {
				for b.Loop() {
					for _, path := range paths {
						parseForBenchmark(b, l, v, &config.Store{}, path)
					}
				}
			})

			b.Run("store_shared", func(b *testing.B) {
				for b.Loop() {
					store := &config.Store{}

					for _, path := range paths {
						parseForBenchmark(b, l, v, store, path)
					}
				}
			})

			b.Run("store_warm", func(b *testing.B) {
				store := &config.Store{}

				for _, path := range paths {
					parseForBenchmark(b, l, v, store, path)
				}

				b.ResetTimer()

				for b.Loop() {
					for _, path := range paths {
						parseForBenchmark(b, l, v, store, path)
					}
				}
			})
		})
	}
}
