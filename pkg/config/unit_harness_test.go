package config_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const (
	fixturesDir = "../../test/fixtures"
	// maxErrorTreeNodes caps the walk of an error tree.
	maxErrorTreeNodes = 10000
	// maxFixtureFiles caps the files a fixture tree may hold.
	maxFixtureFiles = 5000
	// cacheDirMarker is the path element under which tofu runs for a dependency target.
	cacheDirMarker = string(filepath.Separator) + ".terragrunt-cache" + string(filepath.Separator)
)

// defaultOutputs is the `tofu output -json` of any target a case does not set, with every key the fixtures read.
var defaultOutputs = outputsJSON(map[string]string{
	"bar_original_terragrunt_dir": "bar-original-dir",
	"bar_terragrunt_dir":          "bar-dir",
	"data":                        "data-from-state",
	"env":                         "env-from-state",
	"foo":                         "foo-from-state",
	"id":                          "id-from-state",
	"new_attribute":               "new-from-state",
	"original_terragrunt_dir":     "original-dir",
	"some_output":                 "some-from-state",
	"terragrunt_dir":              "dir-from-state",
})

// unitCase is one config that resolveUnit resolves.
type unitCase struct {
	// newFS returns a fresh filesystem with the case's files.
	newFS func(t *testing.T) vfs.FS
	// configure adjusts the parsing context.
	configure func(*config.ParsingContext)
	// outputs maps a target directory to its `tofu output -json`.
	outputs map[string]string
	// cfgPath is the config the case resolves.
	cfgPath string
}

// tofuStub answers tofu invocations and counts them, keeping the ones ToV1 makes apart.
type tofuStub struct {
	outputs    map[string]string
	parseExecs atomic.Int64
	toV1Execs  atomic.Int64
	inToV1     atomic.Bool
}

// handler counts invocations and answers `output` with the outputs of the target whose cache directory it runs in.
// Other commands succeed with no output.
func (s *tofuStub) handler() vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if s.inToV1.Load() {
			s.toV1Execs.Add(1)
		} else {
			s.parseExecs.Add(1)
		}

		if len(inv.Args) == 0 || inv.Args[0] != "output" {
			return vexec.Result{}
		}

		target, _, _ := strings.Cut(inv.Dir, cacheDirMarker)
		if out, ok := s.outputs[target]; ok {
			return vexec.Result{Stdout: []byte(out)}
		}

		return vexec.Result{Stdout: []byte(defaultOutputs)}
	}
}

// unitResult is what resolveUnit observed.
type unitResult struct {
	// parsed is the parsed config, nil when the parse produced none.
	parsed *config.UnitConfig
	// cfg is the config ToV1 returned, nil when the parse produced none.
	cfg *config.TerragruntConfig
	// parseErr is the error of the parse.
	parseErr error
	// toV1Err is the error of ToV1, or parseErr when the parse produced no config.
	toV1Err error
	// parseExecs is the tofu invocations the parse made.
	parseExecs int64
	// toV1Execs is the tofu invocations ToV1 made.
	toV1Execs int64
}

// resolveUnit resolves tc with [config.Store.ResolveUnitConfig] and converts the parse with [config.UnitConfig.ToV1],
// asserting that ToV1 returns every error type and diagnostic the parse returns. When the parse returns no config,
// its error stands in for ToV1's.
func resolveUnit(t *testing.T, tc unitCase) unitResult {
	t.Helper()

	l := logger.CreateLogger()

	stub := &tofuStub{outputs: tc.outputs}
	v := venv.MemVenv().WithFS(tc.newFS(t)).WithHandler(stub.handler())
	ctx, pctx := newTestUnitParsingContext(t, tc.cfgPath)
	tc.configure(pctx)

	pc := config.NewParseContext(v, pctx)

	parsed, parseErr := new(config.Store).ResolveUnitConfig(ctx, l, v, pc, tc.cfgPath)

	stub.inToV1.Store(true)

	var cfg *config.TerragruntConfig

	toV1Err := parseErr

	if parsed != nil {
		cfg, toV1Err = parsed.ToV1(ctx, l, v, new(config.Store), pc)
		assertErrorsWithin(t, parseErr, toV1Err)
	}

	return unitResult{
		parsed:     parsed,
		cfg:        cfg,
		parseErr:   parseErr,
		toV1Err:    toV1Err,
		parseExecs: stub.parseExecs.Load(),
		toV1Execs:  stub.toV1Execs.Load(),
	}
}

// assertErrorsWithin asserts that outer holds every error type and diagnostic of inner.
func assertErrorsWithin(t *testing.T, inner, outer error) {
	t.Helper()

	innerTree := walkErrors(t, inner)
	outerTree := walkErrors(t, outer)

	assert.Subset(t, outerTree.types(), innerTree.types())
	assert.Subset(t, outerTree.diagnostics(), innerTree.diagnostics())
}

// errorTree is the nodes of an error tree, following both single and joined unwraps.
type errorTree struct {
	// nodes is every error in the tree.
	nodes []error
	// diags is every [hcl.Diagnostics] among the leaves of the tree.
	diags []hcl.Diagnostics
}

// walkErrors returns the tree of err, which is empty when err is nil.
func walkErrors(t *testing.T, err error) errorTree {
	t.Helper()

	var (
		tree  errorTree
		queue []error
	)

	if err != nil {
		queue = append(queue, err)
	}

	for len(queue) > 0 {
		require.Less(
			t,
			len(tree.nodes),
			maxErrorTreeNodes,
			"error tree exceeds %d nodes",
			maxErrorTreeNodes,
		)

		current := queue[0]
		queue = queue[1:]

		tree.nodes = append(tree.nodes, current)

		next := errors.Unwrap(current)
		if next != nil {
			queue = append(queue, next)
		}

		joined, isJoined := current.(interface{ Unwrap() []error })
		if isJoined {
			for _, next := range joined.Unwrap() {
				if next != nil {
					queue = append(queue, next)
				}
			}
		}

		var list hcl.Diagnostics
		if next == nil && !isJoined && errors.As(current, &list) {
			tree.diags = append(tree.diags, list)
		}
	}

	return tree
}

// types returns the sorted set of dynamic types in tree.
func (tree errorTree) types() []string {
	types := map[string]struct{}{}

	for _, err := range tree.nodes {
		types[fmt.Sprintf("%T", err)] = struct{}{}
	}

	return slices.Sorted(maps.Keys(types))
}

// diagnostics returns the text of every diagnostic in tree with its subject and detail, sorted.
func (tree errorTree) diagnostics() []string {
	var texts []string

	for _, list := range tree.diags {
		for _, diag := range list {
			texts = append(
				texts,
				fmt.Sprintf("%d|%s|%s|%s", diag.Severity, diag.Error(), diag.Detail, rangeText(diag.Subject)),
			)
		}
	}

	slices.Sort(texts)

	return texts
}

// rangeText renders rng, or the empty string when it is nil.
func rangeText(rng *hcl.Range) string {
	if rng == nil {
		return ""
	}

	return rng.String()
}

// outputsJSON renders string outputs the way `tofu output -json` prints them.
func outputsJSON(outputs map[string]string) string {
	fields := make([]string, 0, len(outputs))

	for _, name := range slices.Sorted(maps.Keys(outputs)) {
		fields = append(
			fields,
			fmt.Sprintf(`%q:{"sensitive":false,"type":"string","value":%q}`, name, outputs[name]),
		)
	}

	return "{" + strings.Join(fields, ",") + "}"
}

// memFS returns a builder for an in-memory filesystem with files under root, each "{{root}}" replaced with root.
func memFS(root string, files map[string]string) func(t *testing.T) vfs.FS {
	return func(t *testing.T) vfs.FS {
		t.Helper()

		fsys := vfs.NewMemMapFS()

		for name, content := range files {
			content = strings.ReplaceAll(content, "{{root}}", filepath.ToSlash(root))
			require.NoError(
				t,
				vfs.WriteFile(
					fsys,
					filepath.Join(root, filepath.FromSlash(name)),
					[]byte(content),
					0o644,
				),
			)
		}

		return fsys
	}
}

// fixtureSet is a set of fixtures, read from disk once.
type fixtureSet struct {
	// files maps each fixture file path to its content, including files directly under the fixtures directory.
	files map[string][]byte
	// configs is every terragrunt.hcl under the fixture roots.
	configs []fixtureConfig
}

// fixtureConfig is a terragrunt.hcl in a fixture root.
type fixtureConfig struct {
	path string
	name string
}

// loadFixtures reads every file under rootNames, skipping excluded directories.
func loadFixtures(t *testing.T, rootNames []string, excluded map[string]struct{}) *fixtureSet {
	t.Helper()

	fsys := venvtest.NewOSOverlayFS()

	fixtures, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	set := &fixtureSet{files: map[string][]byte{}}

	entries, err := vfs.ReadDir(fsys, fixtures)
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		path := filepath.Join(fixtures, entry.Name())
		set.files[path], err = vfs.ReadFile(fsys, path)
		require.NoError(t, err)
	}

	for _, rootName := range rootNames {
		err := vfs.WalkDir(
			fsys,
			filepath.Join(fixtures, filepath.FromSlash(rootName)),
			func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}

				rel, err := filepath.Rel(fixtures, path)
				if err != nil {
					return err
				}

				if _, skip := excluded[filepath.ToSlash(rel)]; skip && d.IsDir() {
					return fs.SkipDir
				}

				if d.IsDir() && d.Name() == ".terragrunt-cache" {
					return fs.SkipDir
				}

				if !d.Type().IsRegular() {
					return nil
				}

				if len(set.files) >= maxFixtureFiles {
					return fmt.Errorf("fixture trees hold more than %d files", maxFixtureFiles)
				}

				set.files[path], err = vfs.ReadFile(fsys, path)
				if err != nil {
					return err
				}

				if d.Name() == config.DefaultTerragruntConfigPath {
					set.configs = append(
						set.configs,
						fixtureConfig{path: path, name: filepath.ToSlash(filepath.Dir(rel))},
					)
				}

				return nil
			},
		)
		require.NoError(t, err)
	}

	require.NotEmpty(t, set.configs)

	return set
}

// newFS builds an in-memory copy of every fixture file.
func (s *fixtureSet) newFS(t *testing.T) vfs.FS {
	t.Helper()

	fsys := vfs.NewMemMapFS()

	for path, content := range s.files {
		require.NoError(t, vfs.WriteFile(fsys, path, content, 0o644))
	}

	return fsys
}

// newTestUnitParsingContext mirrors pkg/config's test helper of the same name, with its own caches.
func newTestUnitParsingContext(
	tb testing.TB,
	cfgPath string,
) (context.Context, *config.ParsingContext) {
	tb.Helper()

	ctx := config.WithCaches(tb.Context())
	pctx := config.NewParsingContext(config.WithStrictControls(controls.New()))

	workingDir, downloadDir := util.DefaultWorkingAndDownloadDirs(cfgPath)

	pctx.TerragruntConfigPath = cfgPath
	pctx.WorkingDir = workingDir
	pctx.RootWorkingDir = workingDir
	pctx.DownloadDir = downloadDir
	pctx.TFPath = "tofu"
	pctx.AutoInit = true
	pctx.SourceMap = map[string]string{}
	pctx.TerraformCliArgs = iacargs.New()
	pctx.MaxFoldersToCheck = 100
	pctx.Experiments = experiment.NewExperiments()
	pctx.Telemetry = new(telemetry.Options)
	pctx.EngineOptions = new(engine.EngineOptions)
	pctx.FeatureFlags = map[string]string{}

	return ctx, pctx
}
