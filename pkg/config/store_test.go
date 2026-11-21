package config_test

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
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
		rel := filepath.Join(fmt.Sprintf("unit-%d", i), config.DefaultTerragruntConfigPath)
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

	ctx, run := newTestUnitParsingContext(b, path)

	_, err := store.ResolveUnitConfig(ctx, l, v, run, path)
	require.NoError(b, err)
}

func TestResolveUnitConfigServesALaterCallTheTreesAndScan(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestUnitParsingContext(t, paths[0])
	run = run.WithFileReadTracking()

	firstCfg, err := store.ResolveUnitQueueConfig(ctx, l, v, run, paths[0])
	require.NoError(t, err)
	assert.Equal(
		t,
		config.Stats{Hits: 6, Misses: 7},
		store.Stats(),
		"two files, each at three stages, and the root's dependency scan; the unit's dependency decode and merge are "+
			"served the root's scan, tree, and base",
	)

	secondCtx, secondRun := newTestUnitParsingContext(t, paths[0])
	secondRun = secondRun.WithFileReadTracking()

	secondCfg, err := store.ResolveUnitQueueConfig(
		secondCtx,
		l,
		v,
		secondRun,
		paths[0],
	)
	require.NoError(t, err)
	assert.Equal(
		t,
		config.Stats{Hits: 15, Misses: 11},
		store.Stats(),
		"both files' base and queue stages run again, served the trees and the root's scan",
	)

	assert.Equal(t, firstCfg, secondCfg)
	assert.NotSame(t, firstCfg, secondCfg, "each caller gets a config of its own")
	assert.Equal(t, "us-east-1", secondCfg.Locals["region"])
	assert.Equal(t, run.FilesRead.Paths(), secondRun.FilesRead.Paths())
	assert.Contains(t, secondRun.FilesRead.Paths(), filepath.Join(root, "common.yaml"))

	firstPC := run

	first, err := store.ResolveUnitConfig(ctx, l, v, firstPC, paths[0])
	require.NoError(t, err)

	secondPC := secondRun

	second, err := store.ResolveUnitConfig(secondCtx, l, v, secondPC, paths[0])
	require.NoError(t, err)

	firstV1, err := first.ToV1(ctx, l, v, new(config.Store), firstPC)
	require.NoError(t, err)

	secondV1, err := second.ToV1(secondCtx, l, v, new(config.Store), secondPC)
	require.NoError(t, err)

	assert.Equal(t, firstV1, secondV1)
	assert.Equal(
		t,
		map[string]any{"name": "unit", "region": "us-east-1", "unit": "unit-0"},
		secondV1.Inputs,
	)
}

func TestResolveRunConfigFromBytesResolvesTheContentInPlaceOfTheFile(t *testing.T) {
	t.Parallel()

	const content = `
include "root" {
  path   = find_in_parent_folders("root.hcl")
  expose = true
}

inputs = {
  name   = "from-content"
  region = include.root.locals.region
}
`

	testCases := []struct {
		files  map[string]string
		name   string
		onDisk bool
	}{
		{
			name: "file on disk with other content",
			files: map[string]string{
				"root.hcl":            `locals { region = "us-east-1" }`,
				"unit/terragrunt.hcl": `inputs = { name = "from-disk" }`,
			},
			onDisk: true,
		},
		{
			name:  "directory not on disk",
			files: map[string]string{"root.hcl": `locals { region = "us-east-1" }`},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := vfs.MemRoot
			path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
			v := venv.MemVenv().WithFS(memFS(root, tc.files)(t))
			ctx, run := newTestUnitParsingContext(t, path)

			cfg, err := new(config.Store).ResolveRunConfigFromBytes(
				ctx,
				logger.CreateLogger(),
				v,
				run,
				path,
				[]byte(content),
			)
			require.NoError(t, err)
			assert.Equal(t, map[string]any{"name": "from-content", "region": "us-east-1"}, cfg.Inputs)

			exists, err := vfs.FileExists(v.FS, path)
			require.NoError(t, err)
			assert.Equal(t, tc.onDisk, exists, "the content never reaches the filesystem of v")
		})
	}
}

func TestResolveUnitQueueConfigReturnsPartialConfigWithParseError(t *testing.T) {
	t.Parallel()

	fsys, path := missingIncludeRepo(t)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()

	ctx, run := newTestUnitParsingContext(t, path)

	s := new(config.Store)

	cfg, err := s.ResolveUnitQueueConfig(ctx, l, v, run, path)
	require.ErrorAs(t, err, new(config.IncludeConfigNotFoundError))
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
		cfgs := make([]*config.TerragruntConfig, callers)

		var g errgroup.Group

		for i := range callers {
			g.Go(func() error {
				ctx, run := newTestUnitParsingContext(t, path)

				cfg, err := store.ResolveUnitQueueConfig(
					ctx,
					l,
					v,
					run,
					path,
				)
				assert.ErrorAs(t, err, new(config.IncludeConfigNotFoundError))

				cfgs[i] = cfg

				return nil
			})
		}

		require.NoError(t, g.Wait())

		seen := make(map[*config.TerragruntConfig]struct{}, callers)

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

	path := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)
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
		filepath.Join(root, "dep", config.DefaultTerragruntConfigPath),
		[]byte("include \"dep\" {\n  path = find_in_parent_folders(\"dep.hcl\")\n}\n"),
		0o644,
	))

	return fsys, path, readByDependency
}

// missingIncludeRepo returns a filesystem holding a unit that includes a file that does not exist, and the unit's
// config path.
func missingIncludeRepo(t *testing.T) (vfs.FS, string) {
	t.Helper()

	root := vfs.MemRoot
	path := filepath.Join(root, config.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
include {
  path = "`+filepath.ToSlash(filepath.Join(root, "missing.hcl"))+`"
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

	root := vfs.MemRoot
	path := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
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

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()

	ctx, parseRun := newTestUnitParsingContext(t, path)
	parseRun = parseRun.WithFileReadTracking()

	s := new(config.Store)

	parsed, err := s.ResolveUnitConfig(ctx, l, v, parseRun, path)
	require.NoError(t, err)

	var g errgroup.Group

	for range callers {
		g.Go(func() error {
			runCtx, run := newTestUnitParsingContext(t, path)
			run = run.WithFileReadTracking()

			if _, err := parsed.ToV1(runCtx, l, v, new(config.Store), run); err != nil {
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

	path := filepath.Join(vfs.MemRoot, config.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte("terraform_binary = local.missing\n"), 0o644))

	v := venv.MemVenv().WithFS(fsys)
	ctx, run := newTestUnitParsingContext(t, path)

	var received hcl.Diagnostics

	run.Parser.DiagnosticsHandler = func(_ *hcl.File, diags hcl.Diagnostics) (hcl.Diagnostics, error) {
		received = append(received, diags...)

		return diags, nil
	}

	s := new(config.Store)

	_, err := s.ResolveUnitConfig(ctx, logger.CreateLogger(), v, run, path)
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

			path := filepath.Join(vfs.MemRoot, "app", config.DefaultTerragruntConfigPath)

			fsys := vfs.NewMemMapFS()
			require.NoError(t, vfs.WriteFile(fsys, path, []byte(unit), 0o644))

			v := venv.MemVenv().WithFS(fsys)
			ctx, run := newTestUnitParsingContext(t, path)
			run.DecodedDependencies = tc.supplied

			s := new(config.Store)

			cfg, err := s.ResolveUnitQueueConfig(
				ctx,
				logger.CreateLogger(),
				v,
				run,
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

	root := vfs.MemRoot
	path := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)

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

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestUnitParsingContext(t, path)

	_, err := store.ResolveUnitQueueConfig(ctx, l, v, run, path)
	require.NoError(t, err)

	hit, err := store.ResolveUnitQueueConfig(ctx, l, v, run, path)
	require.NoError(t, err)
	clear(hit.Errors.Ignore[0].Signals)

	var g errgroup.Group

	for range callers {
		g.Go(func() error {
			callerCtx, callerRun := newTestUnitParsingContext(t, path)

			cfg, err := store.ResolveUnitQueueConfig(callerCtx, l, v, callerRun, path)
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

	root := vfs.MemRoot
	path := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)

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

	v := venv.MemVenv().WithFS(fsys)
	ctx, run := newTestUnitParsingContext(t, path)

	s := new(config.Store)

	cfg, err := s.ResolveRunConfig(ctx, logger.CreateLogger(), v, run, path)

	var unparsed config.UnparsedIncludeError

	require.ErrorAs(t, err, &unparsed)
	assert.Equal(t, filepath.Join(root, "a.hcl"), unparsed.IncludePath)
	assert.NotNil(t, cfg)
}

func TestResolveRunConfigAddsTheRunParts(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestUnitParsingContext(t, paths[0])

	queueCfg, err := store.ResolveUnitQueueConfig(ctx, l, v, run, paths[0])
	require.NoError(t, err)
	assert.Nil(t, queueCfg.Inputs)

	runCfg, err := store.ResolveRunConfig(ctx, l, v, run, paths[0])
	require.NoError(t, err)
	assert.Equal(
		t,
		map[string]any{"name": "unit", "region": "us-east-1", "unit": "unit-0"},
		runCfg.Inputs,
	)
	assert.Equal(t, config.Stats{Hits: 15, Misses: 11}, store.Stats())
}

func TestResolveRunConfigEvaluatesGetWorkingDirInLocals(t *testing.T) {
	t.Parallel()

	path := filepath.Join(vfs.MemRoot, "unit", config.DefaultTerragruntConfigPath)
	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
terraform {
  source = "../modules/app"
}

locals {
  dir = get_working_dir()
}

inputs = {
  dir = local.dir
}
`), 0o644))

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestUnitParsingContext(t, path)

	first, err := store.ResolveRunConfig(ctx, l, v, run, path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(first.Inputs["dir"].(string), run.DownloadDir), "the working dir is downloaded")

	second, err := store.ResolveRunConfig(ctx, l, v, run, path)
	require.NoError(t, err)
	assert.Equal(t, first.Inputs, second.Inputs)
}

func TestResolveRunConfigReturnsNoConfigForAMissingFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(vfs.MemRoot, config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(vfs.NewMemMapFS())
	ctx, run := newTestUnitParsingContext(t, path)

	s := new(config.Store)

	cfg, err := s.ResolveRunConfig(ctx, logger.CreateLogger(), v, run, path)
	require.ErrorAs(t, err, new(config.TerragruntConfigNotFoundError))
	assert.Nil(t, cfg)
}

func TestReadTerragruntConfigReadsThroughTheStore(t *testing.T) {
	t.Parallel()

	fsys, paths := includingRepo(t, vfs.MemRoot, 1)
	v := venv.MemVenv().WithFS(fsys)
	store := &config.Store{}

	ctx, run := newTestUnitParsingContext(t, paths[0])

	cfg, err := store.ReadTerragruntConfig(ctx, logger.CreateLogger(), v, run)
	require.NoError(t, err)
	assert.Equal(
		t,
		map[string]any{"name": "unit", "region": "us-east-1", "unit": "unit-0"},
		cfg.Inputs,
	)
	assert.NotZero(t, store.Stats().Misses)
}

func TestDependencyReadsReachOnlyARun(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys, path, readByDependency := dependencyReadsRepo(t, root)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, queueRun := newTestUnitParsingContext(t, path)
	queueRun = queueRun.WithFileReadTracking()

	_, err := store.ResolveUnitQueueConfig(ctx, l, v, queueRun, path)
	require.NoError(t, err)
	assert.NotContains(t, queueRun.FilesRead.Paths(), readByDependency)

	runCtx, run := newTestUnitParsingContext(t, path)
	run = run.WithFileReadTracking()

	_, err = store.ResolveRunConfig(runCtx, l, v, run, path)
	require.NoError(t, err)
	assert.Contains(t, run.FilesRead.Paths(), readByDependency)
}

func TestResolveUnitBaseVariablesStopsBeforeTheDependencyBlocks(t *testing.T) {
	t.Parallel()

	path := filepath.Join(vfs.MemRoot, config.DefaultTerragruntConfigPath)

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

	ctx, run := newTestUnitParsingContext(t, path)

	vars, err := store.ResolveUnitBaseVariables(ctx, l, v, run, path)
	require.NoError(t, err)
	assert.Equal(t, "us-east-1", vars[config.MetadataLocal].GetAttr("region").AsString())

	_, err = store.ResolveUnitQueueConfig(ctx, l, v, run, path)
	require.Error(t, err)
}

func TestResolveUnitQueueConfigBuildsOnTheBaseVariables(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestUnitParsingContext(t, paths[0])

	_, err := store.ResolveUnitBaseVariables(ctx, l, v, run, paths[0])
	require.NoError(t, err)
	assert.Equal(
		t,
		config.Stats{Hits: 0, Misses: 5},
		store.Stats(),
		"the syntax tree and the base of both files, and the root's dependency scan",
	)

	_, err = store.ResolveUnitQueueConfig(ctx, l, v, run, paths[0])
	require.NoError(t, err)

	stats := store.Stats()
	assert.Equal(
		t,
		uint64(8),
		stats.Hits,
		"the unit's tree and base are served, and the root's scan, tree, and base to the dependency decode and to "+
			"the merge",
	)
}

func TestResolveUnitBaseVariablesEvaluatesOnceWithRacing(t *testing.T) {
	t.Parallel()

	const callers = 8

	root := vfs.MemRoot
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	var g errgroup.Group

	for range callers {
		g.Go(func() error {
			ctx, run := newTestUnitParsingContext(t, paths[0])
			run = run.WithFileReadTracking()

			vars, err := store.ResolveUnitBaseVariables(ctx, l, v, run, paths[0])
			if err != nil {
				return err
			}

			assert.Equal(t, "unit", vars[config.MetadataLocal].GetAttr("name").AsString())
			assert.Contains(t, run.FilesRead.Paths(), filepath.Join(root, "common.yaml"))

			return nil
		})
	}

	require.NoError(t, g.Wait())
	assert.Equal(
		t,
		uint64(5),
		store.Stats().Misses,
		"the syntax tree and the base of both files, and the root's dependency scan",
	)
}

func TestResolveUnitBaseVariablesGivesEachCallerItsOwnMap(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestUnitParsingContext(t, paths[0])

	first, err := store.ResolveUnitBaseVariables(ctx, l, v, run, paths[0])
	require.NoError(t, err)

	delete(first, config.MetadataLocal)

	second, err := store.ResolveUnitBaseVariables(ctx, l, v, run, paths[0])
	require.NoError(t, err)
	assert.Contains(t, second, config.MetadataLocal)
}

func TestResolveUnitBaseVariablesDoesNotRetainAFailedBase(t *testing.T) {
	t.Parallel()

	path := filepath.Join(vfs.MemRoot, config.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
locals {
  region = local.missing
}
`), 0o644))

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	resolve := func() error {
		ctx, run := newTestUnitParsingContext(t, path)

		_, err := store.ResolveUnitBaseVariables(ctx, l, v, run, path)

		return err
	}

	require.Error(t, resolve())

	misses := store.Stats().Misses

	require.Error(t, resolve())
	assert.Equal(t, misses+1, store.Stats().Misses, "the base ran again and the syntax tree was served")
}

func TestResolveUnitBaseVariablesReportsAnEvalContextFailure(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)

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

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	resolve := func() (map[string]cty.Value, error) {
		ctx, run := newTestUnitParsingContext(t, path)

		return store.ResolveUnitBaseVariables(ctx, l, v, run, path)
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

	path := filepath.Join(vfs.MemRoot, config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithFS(vfs.NewMemMapFS())
	ctx, run := newTestUnitParsingContext(t, path)

	s := new(config.Store)

	vars, err := s.ResolveUnitBaseVariables(
		ctx,
		logger.CreateLogger(),
		v,
		run,
		path,
	)
	require.ErrorAs(t, err, new(config.TerragruntConfigNotFoundError))
	assert.Nil(t, vars)
}

func TestResolveUnitConfigSharesIncludedFilesAcrossUnits(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys, paths := includingRepo(t, root, 3)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	for _, path := range paths {
		ctx, run := newTestUnitParsingContext(t, path)

		pc := run

		parsed, err := store.ResolveUnitConfig(ctx, l, v, pc, path)
		require.NoError(t, err)

		cfg, err := parsed.ToV1(ctx, l, v, new(config.Store), pc)
		require.NoError(t, err)
		assert.Equal(t, filepath.Base(filepath.Dir(path)), cfg.Inputs["unit"])
	}

	assert.Equal(
		t,
		config.Stats{Hits: 22, Misses: 17},
		store.Stats(),
		"the root's syntax tree is parsed once; each unit evaluates it in its own context",
	)
}

func TestResolveUnitConfigLabelsBareIncludeUnderTwoContextsWithRacing(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys := vfs.NewMemMapFS()
	require.NoError(
		t,
		vfs.WriteFile(fsys, filepath.Join(root, "root.hcl"), []byte("inputs = { a = 1 }\n"), 0o644),
	)

	unit := filepath.Join(root, "unit", config.DefaultTerragruntConfigPath)
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
				ctx, run := newTestUnitParsingContext(t, unit)
				run.WorkingDir = workingDir

				cfg, err := store.ResolveUnitQueueConfig(
					ctx,
					l,
					v,
					run,
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

func TestResolveUnitQueueConfigReadsAChangedFileOnTheNextCall(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	region := func() any {
		ctx, run := newTestUnitParsingContext(t, paths[0])
		run = run.WithFileReadTracking()

		cfg, err := store.ResolveUnitQueueConfig(ctx, l, v, run, paths[0])
		require.NoError(t, err)

		return cfg.Locals["region"]
	}

	assert.Equal(t, "us-east-1", region())

	common := filepath.Join(root, "common.yaml")
	require.NoError(t, vfs.WriteFile(fsys, common, []byte("region: eu-west-1\n"), 0o644))

	assert.Equal(t, "eu-west-1", region())
}

func TestResolveUnitQueueConfigScansAnIncludedFileAgainWhenItChanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		wantMisses uint64
		rewrite    bool
	}{
		{name: "unchanged", rewrite: false, wantMisses: 5},
		{name: "changed", rewrite: true, wantMisses: 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			root := vfs.MemRoot
			fsys, paths := includingRepo(t, root, 2)
			v := venv.MemVenv().WithFS(fsys)
			l := logger.CreateLogger()
			store := &config.Store{}

			resolve := func(path string) {
				ctx, run := newTestUnitParsingContext(t, path)

				_, err := store.ResolveUnitQueueConfig(ctx, l, v, run, path)
				require.NoError(t, err)
			}

			resolve(paths[0])

			misses := store.Stats().Misses

			if tt.rewrite {
				rootPath := filepath.Join(root, "root.hcl")

				content, err := vfs.ReadFile(fsys, rootPath)
				require.NoError(t, err)
				require.NoError(t, vfs.WriteFile(fsys, rootPath, append(content, '\n'), 0o644))
			}

			resolve(paths[1])

			assert.Equal(
				t,
				tt.wantMisses,
				store.Stats().Misses-misses,
				"the second unit's three stages and the root's base and queue stage, "+
					"plus the root's tree and dependency scan when it changed",
			)
		})
	}
}

func TestResolveUnitQueueConfigMergesAnExposedIncludeFromTheResolutionOfItsVariable(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	path := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(
		fsys,
		filepath.Join(root, "exposed.hcl"),
		[]byte("inputs = {\n  region = \"us-east-1\"\n}\n"),
		0o644,
	))
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
include "exposed" {
  path   = "../exposed.hcl"
  expose = true
}

locals {
  region = include.exposed.inputs.region
}
`), 0o644))

	v := venv.MemVenv().WithFS(fsys)
	store := &config.Store{}
	ctx, run := newTestUnitParsingContext(t, path)

	cfg, err := store.ResolveUnitQueueConfig(ctx, logger.CreateLogger(), v, run, path)
	require.NoError(t, err)

	assert.Equal(t, "us-east-1", cfg.Locals["region"])
	assert.Equal(
		t,
		config.Stats{Hits: 8, Misses: 7},
		store.Stats(),
		"each file's tree, base, and queue stage and the exposed file's dependency scan ran once, "+
			"and the merge was served the exposed file's queue stage",
	)
}

func TestResolveUnitConfigRunsUnmemoizedForSuppliedValues(t *testing.T) {
	t.Parallel()

	root := vfs.MemRoot
	fsys, paths := includingRepo(t, root, 1)
	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	store := &config.Store{}

	ctx, run := newTestUnitParsingContext(t, paths[0])
	locals := cty.ObjectVal(map[string]cty.Value{"from_run": cty.StringVal("run")})
	run.Locals = &locals

	for range 2 {
		_, err := store.ResolveUnitConfig(ctx, l, v, run, paths[0])
		require.NoError(t, err)
	}

	assert.Equal(
		t,
		config.Stats{Hits: 11, Misses: 3},
		store.Stats(),
		"only the syntax trees and the root's dependency scan are shared",
	)
}

func TestResolveUnitQueueConfigParsesAgainForAnotherEnvironment(t *testing.T) {
	t.Parallel()

	path := filepath.Join(vfs.MemRoot, "unit", config.DefaultTerragruntConfigPath)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`
locals {
  token = get_env("AWS_SESSION_TOKEN")
}
`), 0o644))

	l := logger.CreateLogger()
	store := &config.Store{}

	for _, token := range []string{"fake-token", "fake-refreshed-token"} {
		v := venv.MemVenv().WithFS(fsys).WithEnv(map[string]string{"AWS_SESSION_TOKEN": token})
		ctx, run := newTestUnitParsingContext(t, path)

		cfg, err := store.ResolveUnitQueueConfig(ctx, l, v, run, path)
		require.NoError(t, err)
		assert.Equal(t, token, cfg.Locals["token"])
	}
}

func TestStoreContextCoversEachInput(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(vfs.MemRoot, config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithWriter(new(bytes.Buffer))

	_, base := newTestUnitParsingContext(t, cfgPath)
	baseKey, memoized := config.StoreContext(v, base)
	require.True(t, memoized)
	require.NotEmpty(t, baseKey)

	keyed := map[string]func(*config.ParsingContext){
		"config path":          func(p *config.ParsingContext) { p.TerragruntConfigPath += ".other" },
		"original config path": func(p *config.ParsingContext) { p.OriginalTerragruntConfigPath += ".other" },
		"working dir":          func(p *config.ParsingContext) { p.WorkingDir += "-other" },
		"root working dir":     func(p *config.ParsingContext) { p.RootWorkingDir += "-other" },
		"download dir":         func(p *config.ParsingContext) { p.DownloadDir += "-other" },
		"source":               func(p *config.ParsingContext) { p.Source = "git::other" },
		"source map":           func(p *config.ParsingContext) { p.SourceMap = map[string]string{"a": "b"} },
		"command":              func(p *config.ParsingContext) { p.TerraformCommand = "apply" },
		"original command":     func(p *config.ParsingContext) { p.OriginalTofuCommand = "apply" },
		"cli args":             func(p *config.ParsingContext) { p.TerraformCliArgs = iacargs.New("-lock=false") },
		"iam role": func(p *config.ParsingContext) {
			p.IAMRoleOptions = iam.RoleOptions{RoleARN: "arn:aws:iam::123456789012:role/a"}
		},
		"original iam role": func(p *config.ParsingContext) {
			p.OriginalIAMRoleOptions = iam.RoleOptions{AssumeRoleDuration: 1}
		},
		"feature flags":     func(p *config.ParsingContext) { p.FeatureFlags = map[string]string{"f": "1"} },
		"auth provider cmd": func(p *config.ParsingContext) { p.AuthProviderCmd = "creds" },
		"diagnostics":       func(p *config.ParsingContext) { p.Parser.Diagnostics = config.DiagnosticsSuppressed },
		"ignore diagnostics": func(p *config.ParsingContext) {
			p.Parser.IgnoreDiagnostics = true
		},
		"bare include rewrite": func(p *config.ParsingContext) {
			p.Parser.RewriteBareInclude = !p.Parser.RewriteBareInclude
		},
		"halt only in blocks": func(p *config.ParsingContext) {
			p.Parser.HaltOnErrorOnlyInBlocks = []string{"catalog"}
		},
		"skip output":             func(p *config.ParsingContext) { p.SkipOutput = true },
		"skip outputs resolution": func(p *config.ParsingContext) { p.SkipOutputsResolution = true },
		"parse depth":             func(p *config.ParsingContext) { p.ParseDepth = 3 },
		"skip autoinclude merge":  func(p *config.ParsingContext) { p.SkipAutoIncludeMerge = true },
		"read tracking":           func(p *config.ParsingContext) { p.FilesRead = config.NewFilesRead() },
		"partial parse": func(p *config.ParsingContext) {
			p.PartialParseDecodeList = []config.PartialDecodeSectionType{config.ExcludeBlock}
		},
	}

	for name, mutate := range keyed {
		t.Run("keyed/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestUnitParsingContext(t, cfgPath)
			mutate(run)

			key, memoized := config.StoreContext(v, run)
			require.True(t, memoized)
			assert.NotEqual(t, baseKey, key)
		})
	}

	fixed := map[string]func(*config.ParsingContext){
		"tofu binary":       func(p *config.ParsingContext) { p.TFPath = "terraform" },
		"log abs paths":     func(p *config.ParsingContext) { p.LogShowAbsPaths = true },
		"debug":             func(p *config.ParsingContext) { p.Debug = true },
		"max parent probes": func(p *config.ParsingContext) { p.MaxFoldersToCheck = 5 },
	}

	for name, mutate := range fixed {
		t.Run("fixed/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestUnitParsingContext(t, cfgPath)
			mutate(run)

			key, memoized := config.StoreContext(v, run)
			require.True(t, memoized)
			assert.Equal(t, baseKey, key)
		})
	}

	keyedByVenv := map[string]*venv.Venv{
		"environment":      v.WithEnv(map[string]string{"AWS_SESSION_TOKEN": "fake-token"}),
		"discarded output": v.WithWriter(io.Discard),
	}

	for name, other := range keyedByVenv {
		t.Run("keyed/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestUnitParsingContext(t, cfgPath)

			key, memoized := config.StoreContext(other, run)
			require.True(t, memoized)
			assert.NotEqual(t, baseKey, key)
		})
	}

	fixedByVenv := map[string]*venv.Venv{
		"another writer":   v.WithWriter(new(bytes.Buffer)),
		"error writer":     v.WithErrWriter(io.Discard),
		"same environment": v.WithEnvCloned(),
	}

	for name, other := range fixedByVenv {
		t.Run("fixed/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestUnitParsingContext(t, cfgPath)

			key, memoized := config.StoreContext(other, run)
			require.True(t, memoized)
			assert.Equal(t, baseKey, key)
		})
	}

	value := cty.StringVal("v")
	unmemoized := map[string]func(*config.ParsingContext){
		"locals":       func(p *config.ParsingContext) { p.Locals = &value },
		"values":       func(p *config.ParsingContext) { p.Values = &value },
		"features":     func(p *config.ParsingContext) { p.Features = &value },
		"dependencies": func(p *config.ParsingContext) { p.DecodedDependencies = &value },
		"includes":     func(p *config.ParsingContext) { *p = *p.WithTrackInclude(&config.TrackInclude{}) },
		"diagnostics handler": func(p *config.ParsingContext) {
			p.Parser.DiagnosticsHandler = func(*hcl.File, hcl.Diagnostics) (hcl.Diagnostics, error) { return nil, nil }
		},
	}

	for name, mutate := range unmemoized {
		t.Run("unmemoized/"+name, func(t *testing.T) {
			t.Parallel()

			_, run := newTestUnitParsingContext(t, cfgPath)
			mutate(run)

			_, memoized := config.StoreContext(v, run)
			assert.False(t, memoized)
		})
	}
}

func TestStoreContextKeysWhetherTheParseIsPartial(t *testing.T) {
	t.Parallel()

	cfgPath := filepath.Join(t.TempDir(), config.DefaultTerragruntConfigPath)
	v := venv.MemVenv().WithWriter(new(bytes.Buffer))

	key := func(t *testing.T, sections ...config.PartialDecodeSectionType) string {
		t.Helper()

		_, run := newTestUnitParsingContext(t, cfgPath)
		run.PartialParseDecodeList = sections

		key, memoized := config.StoreContext(v, run)
		require.True(t, memoized)

		return key
	}

	excludeOnly := key(t, config.ExcludeBlock)

	assert.NotEqual(t, key(t), excludeOnly)
	assert.Equal(t, excludeOnly, key(t, config.RemoteStateBlock))
	assert.Equal(t, excludeOnly, key(t, config.ExcludeBlock, config.RemoteStateBlock))
}

// sharedStoreFixtureRoots are the fixture directories whose terragrunt.hcl files
// TestResolveRunConfigThroughASharedStoreMatchesAFreshStoreWithRacing resolves.
var sharedStoreFixtureRoots = []string{
	"include",
	"include-deep",
	"include-expose",
	"include-multiple",
	"multiinclude-dependency",
	"locals",
	"inputs",
	"feature-flags",
	"errors",
	"exclude",
	"hooks",
	"extra-args",
	"read-config",
	"parent-folders",
	"find-parent",
	"get-path",
	"null-values",
	"partial-parse",
	"render-json-mock-outputs",
	"dependency-expansion/mocks",
	"dependency-expansion/keyed",
}

// sharedStoreExcludedDirs are fixtures that call run_cmd, which the tofu stub cannot answer, or AWS, including
// through the output fetch.
var sharedStoreExcludedDirs = map[string]struct{}{
	"locals/run-once":                        {},
	"locals/run-multiple":                    {},
	"read-config/iam_role_in_file":           {},
	"read-config/iam_roles_multiple_modules": {},
	"render-json-mock-outputs/app":           {},
}

func TestResolveRunConfigThroughASharedStoreMatchesAFreshStoreWithRacing(t *testing.T) {
	t.Parallel()

	fixtures := loadFixtures(t, sharedStoreFixtureRoots, sharedStoreExcludedDirs)
	shared := new(config.Store)

	t.Cleanup(func() {
		assert.Positive(t, shared.Stats().Hits)
	})

	for _, pass := range []string{"first", "second"} {
		t.Run(pass, func(t *testing.T) {
			t.Parallel()

			for _, fixture := range fixtures.configs {
				t.Run(fixture.name, func(t *testing.T) {
					t.Parallel()

					want, wantErr := resolveFixture(t, fixtures, fixture.path, new(config.Store))
					got, gotErr := resolveFixture(t, fixtures, fixture.path, shared)

					assert.Equal(t, want, got)

					wantTree, gotTree := walkErrors(t, wantErr), walkErrors(t, gotErr)
					assert.Equal(t, wantTree.types(), gotTree.types())
					assert.Equal(t, wantTree.diagnostics(), gotTree.diagnostics())
				})
			}
		})
	}
}

// resolveFixture resolves the fixture config at cfgPath through s on a venv of its own, fetching outputs from a tofu
// stub.
func resolveFixture(
	t *testing.T,
	fixtures *fixtureSet,
	cfgPath string,
	s *config.Store,
) (*config.TerragruntConfig, error) {
	t.Helper()

	v := venv.MemVenv().WithFS(fixtures.newFS(t)).WithHandler(new(tofuStub).handler())
	ctx, pctx := newTestUnitParsingContext(t, cfgPath)

	return s.ResolveRunConfig(ctx, logger.CreateLogger(), v, pctx, cfgPath)
}

func BenchmarkResolveUnitConfig(b *testing.B) {
	for _, n := range []int{16, 128} {
		b.Run(fmt.Sprintf("units_%d", n), func(b *testing.B) {
			root := vfs.MemRoot
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

func BenchmarkResolveUnitConfigMergedIncludeLocals(b *testing.B) {
	root := b.TempDir()
	files := map[string]string{
		"root.hcl": `
locals {
  units = {
    for i in range(1000) : "unit-${i}" => { name = "unit-${i}", zones = ["a", "b", "c"], index = i }
  }
}
`,
		"app/terragrunt.hcl": `
include "root" {
  path = find_in_parent_folders("root.hcl")
}
`,
	}

	fsys := vfs.NewMemMapFS()

	for rel, content := range files {
		require.NoError(b, vfs.WriteFile(fsys, filepath.Join(root, rel), []byte(content), 0o644))
	}

	v := venv.MemVenv().WithFS(fsys)
	l := logger.CreateLogger()
	path := filepath.Join(root, "app", config.DefaultTerragruntConfigPath)

	for b.Loop() {
		parseForBenchmark(b, l, v, &config.Store{}, path)
	}
}
