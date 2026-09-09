package config_test

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureRoot is where the in-memory corpus is written.
const fixtureRoot = "/fixture"

// renderFixtureParentPath is the shared parent behind the render differential in
// test/integration_base_blocks_cache_test.go. That run reaches the cache through the CLI,
// where its counters are out of reach, so the fixture's own content is held to being
// cacheable from here instead.
const renderFixtureParentPath = "../../test/fixtures/base-blocks-cache/root.hcl"

// corpusUnitCommands gives every corpus unit a command of its own, keyed by the directory it
// sits in. A run rewrites the command and the arguments between units, so two units under one
// shared parent have to disagree here for the differential to reach a function that reads
// either of them.
var corpusUnitCommands = map[string]string{
	"a": "plan",
	"b": "apply",
	"c": "output",
	"d": "destroy",
}

func TestBaseBlocksCacheParsesEveryUnitIdenticallyWhenDisabled(t *testing.T) {
	t.Parallel()

	files, units := baseBlocksCorpus()

	withCache, enabled := parseCorpusUnits(t, files, units, map[string]string{})
	withoutCache, disabled := parseCorpusUnits(t, files, units, map[string]string{
		config.EnvDisableBaseBlocksCache: "true",
	})

	require.Positive(t, enabled.Hits(), "corpus never exercised the cache")
	require.Zero(t, disabled.Hits()+disabled.Misses(), "kill switch did not turn the cache off")

	for _, unit := range units {
		assert.Equal(t, withoutCache[unit], withCache[unit], unit)
	}
}

func TestBaseBlocksCacheDecodesEachCorpusFixturesCacheableFilesOnce(t *testing.T) {
	t.Parallel()

	files, units := baseBlocksCorpus()
	byFixture := corpusUnitsByFixture(t, units)

	require.Equal(
		t,
		slices.Sorted(maps.Keys(baseBlocksCorpusCacheableFiles)),
		slices.Sorted(maps.Keys(byFixture)),
		"baseBlocksCorpusCacheableFiles no longer names the corpus fixtures one for one",
	)

	for fixture, fixtureUnits := range byFixture {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()

			_, cache := parseCorpusUnits(t, files, fixtureUnits, map[string]string{})
			cacheable := baseBlocksCorpusCacheableFiles[fixture]

			require.Equal(
				t,
				cacheable,
				cache.Misses(),
				"the cache decoded a different set of files for %s than the fixture claims is cacheable",
				fixture,
			)

			if cacheable == 0 {
				require.Zero(
					t,
					cache.Hits(),
					"the cache answered for %s, none of whose files it may decode",
					fixture,
				)

				return
			}

			require.Positive(
				t,
				cache.Hits(),
				"%s decoded its cacheable files but reused none of them",
				fixture,
			)
		})
	}
}

func TestBaseBlocksCacheSeparatesUnitsReadingDifferentEnvironments(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"root.hcl": `
locals {
  from_env = get_env("TG_TEST_UNIT_VALUE", "fallback")
}

inputs = {
  from_env = local.from_env
}
`,
		"a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
		"b/terragrunt.hcl": childIncluding("../root.hcl", "b"),
	}

	fsys := venvtest.NewFS(t, fixtureRoot, files)
	ctx := config.WithConfigValues(t.Context())
	l := logger.CreateLogger()

	// A run gives each unit an environment of its own and writes that unit's auth provider
	// output into it before parsing, so two units of one parent can read one variable and
	// find different values under it.
	for _, name := range []string{"a", "b"} {
		v := venvtest.New().WithFS(fsys).WithEnv(map[string]string{
			"TG_TEST_UNIT_VALUE": "value-for-" + name,
		})

		unit := filepath.Join(fixtureRoot, name, config.DefaultTerragruntConfigPath)

		assert.Equal(
			t,
			"value-for-"+name,
			parseUnit(t, ctx, v, l, unit).Inputs["from_env"],
			"unit %s was handed the other unit's environment",
			name,
		)
	}

	cache := config.ContextBaseBlocksCache(ctx)
	require.Equal(
		t,
		int64(2),
		cache.Misses(),
		"the two environments were served from one entry",
	)
}

func TestBaseBlocksCacheReusesTheRenderFixtureParent(t *testing.T) {
	t.Parallel()

	parent, err := os.ReadFile(renderFixtureParentPath)
	require.NoError(t, err)

	files := map[string]string{
		"root.hcl":         string(parent),
		"a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
		"b/terragrunt.hcl": childIncluding("../root.hcl", "b"),
	}

	v := venvtest.New().WithFS(venvtest.NewFS(t, fixtureRoot, files))
	ctx := config.WithConfigValues(t.Context())
	l := logger.CreateLogger()

	for _, name := range []string{"a", "b"} {
		unit := filepath.Join(fixtureRoot, name, config.DefaultTerragruntConfigPath)

		_, pctx := newTestParsingContext(t, v, unit)
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.DeepMerge))

		_, err := config.ParseConfigFile(ctx, pctx, l, unit, nil)
		require.NoError(t, err, unit)
	}

	cache := config.ContextBaseBlocksCache(ctx)
	require.Equal(t, int64(1), cache.Misses(), "the render fixture's parent is no longer cacheable")
	require.Positive(t, cache.Hits(), "the render fixture's parent was never reused")
}

func TestBaseBlocksCacheDecodesSharedParentOnce(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"root.hcl": `
locals {
  env = "prod"
}

inputs = {
  env = local.env
}
`,
	}

	units := make([]string, 0, 4)

	for _, name := range []string{"a", "b", "c", "d"} {
		files[filepath.Join(name, "terragrunt.hcl")] = childIncluding("../root.hcl", name)
		units = append(units, filepath.Join(fixtureRoot, name, "terragrunt.hcl"))
	}

	configs, cache := parseCorpusUnits(t, files, units, map[string]string{})

	for _, unit := range units {
		assert.Equal(t, "prod", configs[unit].Inputs["env"], unit)
	}

	require.Equal(t, int64(1), cache.Misses(), "shared parent was decoded more than once")
	require.GreaterOrEqual(
		t,
		cache.Hits()+cache.Misses(),
		int64(len(units)),
		"parent was not decoded once per unit without the cache",
	)
}

func TestBaseBlocksCacheKeepsTheAutoIncludeBesideAReusedParent(t *testing.T) {
	t.Parallel()

	// A cached entry holds no TrackInclude, so the sibling autoinclude is registered again on
	// every hit. The corpus differential would notice the second unit losing it; this holds
	// the first unit to keeping it too, since a parent is already served from the cache
	// several times over within one unit's own parse.
	files, units := baseBlocksCorpus()
	fixtureUnits := corpusUnitsByFixture(t, units)["parentautoinc"]
	require.Len(t, fixtureUnits, 2, "the parentautoinc fixture no longer has two units")

	configs, cache := parseCorpusUnits(t, files, fixtureUnits, map[string]string{})

	for _, unit := range fixtureUnits {
		assert.Equal(t, "from-parent-autoinclude", configs[unit].Inputs["extra"], unit)
	}

	require.Positive(t, cache.Hits(), "the parent beside the autoinclude was never reused")
}

func TestBaseBlocksCacheSkipsFileCallingUnclassifiedFunction(t *testing.T) {
	t.Parallel()

	// uuid is deliberately left off the allowlist, so it stands in for any function name the
	// classification does not vouch for.
	files := map[string]string{
		"root.hcl": `
locals {
  id = uuid()
}

inputs = {
  id = local.id
}
`,
		"a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
	}

	unit := filepath.Join(fixtureRoot, "a", "terragrunt.hcl")

	configs, cache := parseCorpusUnits(t, files, []string{unit}, map[string]string{})

	require.NotEmpty(t, configs[unit].Inputs["id"], "uuid did not evaluate")
	require.Zero(t, cache.Hits()+cache.Misses(), "cache was consulted for an unclassified call")
}

func TestBaseBlocksCacheSkipsParentDecodedUnderSuppressedDiagnostics(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		parent string
	}{
		{
			name: "local that never evaluates",
			parent: `
locals {
  ok  = "value"
  bad = local.missing
}

inputs = {
  ok = local.ok
}
`,
		},
		{
			name: "local whose function call fails",
			parent: `
locals {
  broken = jsondecode("{")
}

inputs = {
  broken = local.broken
}
`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			files := map[string]string{
				"root.hcl":         tc.parent,
				"a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
				"b/terragrunt.hcl": childIncluding("../root.hcl", "b"),
			}

			v := venvtest.New().WithFS(venvtest.NewFS(t, fixtureRoot, files))
			ctx := config.WithConfigValues(t.Context())
			l := logger.CreateLogger()

			suppressed := filepath.Join(fixtureRoot, "a", "terragrunt.hcl")

			_, suppressedPctx := newTestParsingContext(t, v, suppressed)
			suppressedPctx.ParserOptions = append(
				suppressedPctx.ParserOptions,
				hclparse.WithDiagnosticsHandler(
					func(*hcl.File, hcl.Diagnostics) (hcl.Diagnostics, error) {
						return nil, nil
					},
				),
			)

			_, err := config.ParseConfigFile(ctx, suppressedPctx, l, suppressed, nil)
			require.NoError(t, err, "the handler no longer suppresses the parent's error")

			strict := filepath.Join(fixtureRoot, "b", "terragrunt.hcl")

			_, strictPctx := newTestParsingContext(t, v, strict)

			_, err = config.ParseConfigFile(ctx, strictPctx, l, strict, nil)
			require.Error(t, err, "the strict parse was served the suppressed parse's locals")

			cache := config.ContextBaseBlocksCache(ctx)
			assert.Zero(t, cache.Hits(), "a decode under a diagnostics handler was cached")
		})
	}
}

func TestBaseBlocksCacheSkipsParentReadingAnotherFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	versionPath := filepath.Join(tmpDir, "version.txt")
	require.NoError(t, os.WriteFile(versionPath, []byte("v1"), 0644))

	parentPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(t, os.WriteFile(parentPath, []byte(`
locals {
  version = file("./version.txt")
}

inputs = {
  version = local.version
}
`), 0644))

	units := make([]string, 0, 2)

	for _, name := range []string{"a", "b"} {
		dir := filepath.Join(tmpDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))

		unit := filepath.Join(dir, config.DefaultTerragruntConfigPath)
		require.NoError(t, os.WriteFile(unit, []byte(childIncluding(parentPath, name)), 0644))

		units = append(units, unit)
	}

	// The tofu builtins read the operating system rather than the parsing context's
	// filesystem, so this corpus cannot live in memory the way the others do.
	v := venvtest.NewWithOSFS()
	ctx := config.WithConfigValues(t.Context())
	l := logger.CreateLogger()

	parse := func(unit string) *config.TerragruntConfig {
		t.Helper()

		_, pctx := newTestParsingContext(t, v, unit)

		cfg, err := config.ParseConfigFile(ctx, pctx, l, unit, nil)
		require.NoError(t, err)

		return cfg
	}

	assert.Equal(t, "v1", parse(units[0]).Inputs["version"])

	require.NoError(t, os.WriteFile(versionPath, []byte("v2"), 0644))

	assert.Equal(
		t,
		"v2",
		parse(units[1]).Inputs["version"],
		"the second unit was handed the content the first one read",
	)

	cache := config.ContextBaseBlocksCache(ctx)
	assert.Zero(
		t,
		cache.Hits()+cache.Misses(),
		"a parent reading another file was treated as cacheable",
	)
}

func TestBaseBlocksCacheInvalidatesOnParentEdit(t *testing.T) {
	t.Parallel()

	const parentPath = fixtureRoot + "/root.hcl"

	v := venvtest.New().WithFS(venvtest.NewFS(t, fixtureRoot, nil))
	ctx := config.WithConfigValues(t.Context())
	l := logger.CreateLogger()
	cache := config.ContextBaseBlocksCache(ctx)

	parseParent := func(content string) *config.TerragruntConfig {
		t.Helper()

		_, pctx := newTestParsingContext(t, v, parentPath)

		cfg, err := config.ParseConfigString(ctx, pctx, l, parentPath, content, nil)
		require.NoError(t, err)

		return cfg
	}

	const first = `
locals {
  env = "one"
}

inputs = {
  env = local.env
}
`

	const second = `
locals {
  env = "two"
}

inputs = {
  env = local.env
}
`

	assert.Equal(t, "one", parseParent(first).Inputs["env"])
	assert.Equal(t, "two", parseParent(second).Inputs["env"])
	require.Equal(t, int64(2), cache.Misses(), "edited content reused the stored entry")

	assert.Equal(t, "one", parseParent(first).Inputs["env"])
	require.Equal(t, int64(2), cache.Misses(), "restored content was decoded again")
	require.Positive(t, cache.Hits(), "restored content did not reuse the stored entry")
}

func TestBaseBlocksCacheServesConcurrentChildrenWithRacing(t *testing.T) {
	t.Parallel()

	const childCount = 8

	files := map[string]string{
		"root.hcl": `
locals {
  first  = "a"
  second = "${local.first}b"
  third  = "${local.second}c"
}

inputs = {
  chain = local.third
}
`,
	}

	units := make([]string, 0, childCount)

	for i := range childCount {
		name := fmt.Sprintf("unit%d", i)
		files[filepath.Join(name, "terragrunt.hcl")] = childIncluding("../root.hcl", name)
		units = append(units, filepath.Join(fixtureRoot, name, "terragrunt.hcl"))
	}

	v := venvtest.New().WithFS(venvtest.NewFS(t, fixtureRoot, files))
	ctx := config.WithConfigValues(t.Context())
	l := logger.CreateLogger()

	configs := make([]*config.TerragruntConfig, len(units))

	var wg sync.WaitGroup

	for i, unit := range units {
		wg.Go(func() {
			_, pctx := newTestParsingContext(t, v, unit)

			cfg, err := config.ParseConfigFile(ctx, pctx, l, unit, nil)
			assert.NoError(t, err, unit)

			configs[i] = cfg
		})
	}

	wg.Wait()

	for i, cfg := range configs {
		require.NotNil(t, cfg, units[i])
		assert.Equal(t, "abc", cfg.Inputs["chain"], units[i])
		assert.Equal(t, fmt.Sprintf("unit%d", i), cfg.Inputs["unit"], units[i])
	}

	cache := config.ContextBaseBlocksCache(ctx)
	require.Positive(t, cache.Hits(), "concurrent children never shared an entry")
}

func TestFuncNameIsChildIndependentReportsUnclassifiedNames(t *testing.T) {
	t.Parallel()

	independent, classified := config.FuncNameIsChildIndependent("a_function_added_tomorrow")
	assert.False(t, classified)
	assert.False(t, independent)

	independent, classified = config.FuncNameIsChildIndependent(config.FuncNameGetPlatform)
	assert.True(t, classified)
	assert.True(t, independent)

	independent, classified = config.FuncNameIsChildIndependent(config.FuncNameGetTerragruntDir)
	assert.True(t, classified)
	assert.False(t, independent)
}

func TestEveryFuncNameConstantIsClassified(t *testing.T) {
	t.Parallel()

	names := funcNameConstants(t)
	require.NotEmpty(t, names, "no FuncName constants found to check")

	for constName, funcName := range names {
		_, classified := config.FuncNameIsChildIndependent(funcName)
		assert.True(
			t,
			classified,
			"%s (%q) is not classified in funcNameChildIndependence: decide whether its result can vary between the units a shared parent is decoded for, and add it there",
			constName,
			funcName,
		)
	}
}

// funcNameConstants reads the package's own source for its FuncName constants, so a
// function added to Terragrunt cannot slip past the classification unnoticed.
func funcNameConstants(t *testing.T) map[string]string {
	t.Helper()

	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	names := map[string]string{}
	fset := token.NewFileSet()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, entry.Name(), nil, 0)
		require.NoError(t, err)

		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}

			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				for i, ident := range valueSpec.Names {
					if !strings.HasPrefix(ident.Name, "FuncName") || i >= len(valueSpec.Values) {
						continue
					}

					lit, ok := valueSpec.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}

					value, err := strconv.Unquote(lit.Value)
					require.NoError(t, err)

					names[ident.Name] = value
				}
			}
		}
	}

	return names
}

// baseBlocksCorpusCacheableFiles says, for each corpus fixture, how many distinct files a
// parse of every unit in it is expected to decode through the cache. The whole-corpus
// differential is satisfied by any one fixture still caching, so this is what stops a fixture
// that silently stopped hiding behind the ones that did not.
//
// Three entries count a file other than the fixture's own shared parent: readcfg's parent is
// child-dependent and the config it reads is not, and autoinc and parentautoinc each add the
// sibling file they autoinclude.
var baseBlocksCorpusCacheableFiles = map[string]int64{
	"plain":         1,
	"childfuncs":    0,
	"exposed":       1,
	"chain":         1,
	"readcfg":       1,
	"autoinc":       2,
	"parentautoinc": 2,
	"envflags":      1,
	"cliargs":       0,
}

// corpusUnitsByFixture groups corpus units by the fixture directory they sit under.
func corpusUnitsByFixture(t *testing.T, units []string) map[string][]string {
	t.Helper()

	byFixture := map[string][]string{}

	for _, unit := range units {
		rel, err := filepath.Rel(fixtureRoot, unit)
		require.NoError(t, err)

		fixture, _, nested := strings.Cut(rel, string(filepath.Separator))
		require.True(t, nested, "unit %s does not sit under a fixture directory", unit)

		byFixture[fixture] = append(byFixture[fixture], unit)
	}

	return byFixture
}

// parseCorpusUnits fully parses every unit against one in-memory corpus and one set of
// caches, and returns the parsed configurations alongside the cache they shared.
func parseCorpusUnits(
	t *testing.T,
	files map[string]string,
	units []string,
	env map[string]string,
) (map[string]*config.TerragruntConfig, *config.BaseBlocksCache) {
	t.Helper()

	v := venvtest.New().WithFS(venvtest.NewFS(t, fixtureRoot, files)).WithEnv(env)
	ctx := config.WithConfigValues(t.Context())
	l := logger.CreateLogger()

	configs := make(map[string]*config.TerragruntConfig, len(units))

	for _, unit := range units {
		configs[unit] = parseUnit(t, ctx, v, l, unit)
	}

	return configs, config.ContextBaseBlocksCache(ctx)
}

// parseUnit parses one unit with a parsing context of its own, the way a run gives each
// unit its own context while sharing the caches on ctx. The command and the arguments come
// from the unit too: the runner hands each unit its own, down to its own plan file, and
// dependency output resolution replaces both before it parses a dependency's target.
func parseUnit(
	t *testing.T,
	ctx context.Context,
	v *venv.Venv,
	l log.Logger,
	unit string,
) *config.TerragruntConfig {
	t.Helper()

	dir := filepath.Dir(unit)

	command, ok := corpusUnitCommands[filepath.Base(dir)]
	require.True(t, ok, "unit %s sits in a directory corpusUnitCommands does not name", unit)

	_, pctx := newTestParsingContext(t, v, unit)
	pctx.TerraformCommand = command
	pctx.TerraformCliArgs = iacargs.New(command, "-out="+filepath.Join(dir, "tfplan"))

	cfg, err := config.ParseConfigFile(ctx, pctx, l, unit, nil)
	require.NoError(t, err, unit)

	return cfg
}

// childIncluding returns a unit that includes the parent at path and labels its own inputs,
// so a value that leaked from one unit into another shows up as the wrong label.
func childIncluding(path, name string) string {
	return fmt.Sprintf(`
include "root" {
  path = %q
}

inputs = {
  unit = %q
}
`, path, name)
}

// baseBlocksCorpus returns the differential corpus and the units to parse from it. It
// covers a plain shared parent, a parent whose locals call every function that answers per
// child, an exposed include, locals chains that need several fixed point passes, a parent
// reading another config, a unit's own sibling autoinclude, an autoinclude beside a shared
// parent, a parent reading the environment and its own feature defaults, and a parent reading
// the command and arguments of the unit it is being parsed for.
func baseBlocksCorpus() (map[string]string, []string) {
	files := map[string]string{
		"plain/root.hcl": `
locals {
  env    = "prod"
  region = "us-east-1"
}

inputs = {
  env    = local.env
  region = local.region
}
`,
		"plain/a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
		"plain/b/terragrunt.hcl": childIncluding("../root.hcl", "b"),

		"childfuncs/root.hcl": `
locals {
  relative_to_include   = path_relative_to_include()
  relative_from_include = path_relative_from_include()
  terragrunt_dir        = get_terragrunt_dir()
  original_dir          = get_original_terragrunt_dir()
  parent_dir            = get_parent_terragrunt_dir()
  source_cli_flag       = get_terragrunt_source_cli_flag()
}

inputs = {
  relative_to_include   = local.relative_to_include
  relative_from_include = local.relative_from_include
  terragrunt_dir        = local.terragrunt_dir
  original_dir          = local.original_dir
  parent_dir            = local.parent_dir
  source_cli_flag       = local.source_cli_flag
}
`,
		"childfuncs/a/terragrunt.hcl":        childIncluding("../root.hcl", "a"),
		"childfuncs/deep/b/terragrunt.hcl":   childIncluding("../../root.hcl", "b"),
		"childfuncs/deeper/c/terragrunt.hcl": childIncluding("../../root.hcl", "c"),

		"exposed/root.hcl": `
locals {
  env = "prod"
}

inputs = {
  env = local.env
}
`,
		"exposed/a/terragrunt.hcl": `
include "root" {
  path   = "../root.hcl"
  expose = true
}

locals {
  from_parent = include.root.locals.env
}

inputs = {
  unit        = "a"
  from_parent = local.from_parent
}
`,
		"exposed/b/terragrunt.hcl": `
include "root" {
  path   = "../root.hcl"
  expose = true
}

locals {
  from_parent = upper(include.root.locals.env)
}

inputs = {
  unit        = "b"
  from_parent = local.from_parent
}
`,

		"chain/root.hcl": `
locals {
  fifth  = "${local.fourth}-fifth"
  fourth = "${local.third}-fourth"
  third  = "${local.second}-third"
  second = "${local.first}-second"
  first  = "first"
}

inputs = {
  chain = local.fifth
}
`,
		"chain/a/terragrunt.hcl": `
include "root" {
  path   = "../root.hcl"
  expose = true
}

locals {
  fourth = "${local.third}-child-fourth"
  third  = "${local.second}-child-third"
  second = "${local.first}-child-second"
  first  = include.root.locals.fifth
}

inputs = {
  unit        = "a"
  child_chain = local.fourth
}
`,
		"chain/b/terragrunt.hcl": childIncluding("../root.hcl", "b"),

		"readcfg/shared.hcl": `
locals {
  shared_value = "shared"
}
`,
		"readcfg/root.hcl": `
locals {
  shared = read_terragrunt_config("` + fixtureRoot + `/readcfg/shared.hcl").locals.shared_value
}

inputs = {
  shared = local.shared
}
`,
		"readcfg/a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
		"readcfg/b/terragrunt.hcl": childIncluding("../root.hcl", "b"),

		"autoinc/root.hcl": `
locals {
  env = "prod"
}

inputs = {
  env = local.env
}
`,
		"autoinc/a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
		"autoinc/a/terragrunt.autoinclude.hcl": `
inputs = {
  extra = "auto"
}
`,
		"autoinc/b/terragrunt.hcl": childIncluding("../root.hcl", "b"),

		"parentautoinc/root.hcl": `
locals {
  env = "prod"
}

inputs = {
  env = local.env
}
`,
		"parentautoinc/terragrunt.autoinclude.hcl": `
inputs = {
  extra = "from-parent-autoinclude"
}
`,
		"parentautoinc/a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
		"parentautoinc/b/terragrunt.hcl": childIncluding("../root.hcl", "b"),

		"envflags/root.hcl": `
feature "enabled" {
  default = true
}

locals {
  from_env = get_env("TG_TEST_CORPUS_VALUE", "fallback")
  enabled  = feature.enabled.value
}

inputs = {
  from_env = local.from_env
  enabled  = local.enabled
}
`,
		"envflags/a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
		"envflags/b/terragrunt.hcl": childIncluding("../root.hcl", "b"),

		"cliargs/root.hcl": `
locals {
  command = get_terraform_command()
  args    = join(" ", get_terraform_cli_args())
}

inputs = {
  command = local.command
  args    = local.args
}
`,
		"cliargs/a/terragrunt.hcl": childIncluding("../root.hcl", "a"),
		"cliargs/b/terragrunt.hcl": childIncluding("../root.hcl", "b"),
	}

	units := make([]string, 0, len(files))

	for path := range files {
		if filepath.Base(path) != "terragrunt.hcl" {
			continue
		}

		units = append(units, filepath.Join(fixtureRoot, path))
	}

	slices.Sort(units)

	return files, units
}
