package config_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/iacargs"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

const (
	fixturesDir = "../../../test/fixtures"
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

// parityCase is one config that assertParity parses with pkg/config and with the rewrite.
type parityCase struct {
	// newFS returns a fresh filesystem with the case's files.
	newFS func(t *testing.T) vfs.FS
	// configure adjusts each side's parsing context.
	configure func(*pkgconfig.ParsingContext)
	// outputs maps a target directory to its `tofu output -json`.
	outputs map[string]string
	// cfgPath is the config both sides parse.
	cfgPath string
	// drift is the drifts the case shows.
	drift []drift
}

// drift names a known diagnostic difference between the rewrite and pkg/config. assertParity normalizes only the
// drifts a case lists, fails when a listed drift changes nothing, and never relaxes config or error type equality.
type drift string

const (
	// driftDiagnosticOrder is diagnostic order, which already varies between pkg/config runs. It applies to every
	// case.
	driftDiagnosticOrder drift = "diagnostic order"
	// driftSplitBodySuggestion is the "Did you mean" suggestion in the top level, remote_state, and engine bodies,
	// which the rewrite decodes in two stages. The second stage suggests only from its own schema.
	driftSplitBodySuggestion drift = "split body suggestion"
	// driftJSONShapeRepeated is a JSON shape diagnostic that each decode stage reports again.
	driftJSONShapeRepeated drift = "repeated JSON shape diagnostic"
)

// splitBodySuggestion matches the suggestion after an unsupported argument, block type, or JSON property.
var splitBodySuggestion = regexp.MustCompile(
	`((?:An argument named "[^"]*" is not expected here|Blocks of type "[^"]*" are not expected here|` +
		`No argument or block type is named "[^"]*")\.) Did you mean (?:to define a block of type "[^"]*"\?|` +
		`to define argument "[^"]*"\? If so, use the equals sign to assign it a value\.|"[^"]*"\?)`,
)

// tofuStub answers tofu invocations and counts the ones ToV1 makes.
type tofuStub struct {
	outputs   map[string]string
	toV1Execs atomic.Int64
	inToV1    atomic.Bool
}

// handler counts invocations during ToV1 and answers `output` with the outputs of the target whose cache directory
// it runs in. Other commands succeed with no output.
func (s *tofuStub) handler() vexec.Handler {
	return func(_ context.Context, inv vexec.Invocation) vexec.Result {
		if s.inToV1.Load() {
			s.toV1Execs.Add(1)
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

// assertParity parses tc with pkg/config and the rewrite on separate venvs, contexts, and caches, and asserts that
// ToV1 matches pkg/config's config, errors, and files read, that the parse returns ToV1's error, and that ToV1
// runs no tofu. When the rewrite's parse returns no config, its error stands in for ToV1's.
func assertParity(t *testing.T, tc parityCase) {
	t.Helper()

	l := logger.CreateLogger()

	v1Stub := &tofuStub{outputs: tc.outputs}
	v1Venv := venvtest.New().WithFS(tc.newFS(t)).WithHandler(v1Stub.handler())
	v1Ctx, v1Pctx := newTestParsingContext(t, tc.cfgPath)
	tc.configure(v1Pctx)
	v1Pctx = v1Pctx.WithFileReadTracking()

	stub := &tofuStub{outputs: tc.outputs}
	v := venvtest.New().WithFS(tc.newFS(t)).WithHandler(stub.handler())
	ctx, pctx := newTestParsingContext(t, tc.cfgPath)
	tc.configure(pctx)
	pctx = pctx.WithFileReadTracking()

	wantCfg, wantErr := pkgconfig.ParseConfigFile(v1Ctx, l, v1Venv, v1Pctx, tc.cfgPath, nil)
	if wantErr != nil {
		t.Logf("pkg/config returned an error: %v", wantErr)
	}

	parsed, parseErr := config.ParseConfigFile(ctx, l, v, &hclparse.Store{}, config.NewParseContext(pctx), tc.cfgPath)

	stub.inToV1.Store(true)

	var gotCfg *pkgconfig.TerragruntConfig

	gotErr := parseErr

	if parsed != nil {
		gotCfg, gotErr = parsed.ToV1(ctx, l, v)
		assertSameErrors(t, gotErr, parseErr, nil)
	}

	assert.Equal(t, wantCfg, gotCfg)
	assertSameErrors(t, wantErr, gotErr, tc.drift)
	assert.Equal(t, v1Pctx.FilesRead.Paths(), pctx.FilesRead.Paths())
	assert.Zero(t, stub.toV1Execs.Load(), "ToV1 ran tofu")
}

// assertSameErrors asserts matching error types, diagnostics, and text after normalizing drifts. It fails when a
// listed drift changes nothing.
func assertSameErrors(t *testing.T, want, got error, drifts []drift) {
	t.Helper()

	wantTree := walkErrors(t, want)
	gotTree := walkErrors(t, got)

	listed := map[drift]struct{}{driftDiagnosticOrder: {}}
	for _, d := range drifts {
		listed[d] = struct{}{}
	}

	wantView := wantTree.view(want, listed)
	gotView := gotTree.view(got, listed)

	assert.Equal(t, wantView.types, gotView.types)
	assert.Equal(t, wantView.diagnostics, gotView.diagnostics)
	assert.Equal(t, wantView.text, gotView.text)

	for _, d := range drifts {
		without := maps.Clone(listed)
		delete(without, d)

		assert.NotEqual(
			t,
			wantTree.view(want, without),
			gotTree.view(got, without),
			"drift %q no longer changes the comparison",
			d,
		)
	}
}

// errorView is what assertSameErrors compares of an error.
type errorView struct {
	text        string
	types       []string
	diagnostics []string
}

// view returns what assertSameErrors compares of err, with drifts normalized.
func (tree errorTree) view(err error, drifts map[drift]struct{}) errorView {
	return errorView{
		types:       tree.types(),
		diagnostics: tree.diagnostics(drifts),
		text:        tree.text(err, drifts),
	}
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
		require.Less(t, len(tree.nodes), maxErrorTreeNodes, "error tree exceeds %d nodes", maxErrorTreeNodes)

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

// diagnostics returns the normalized text of every diagnostic in tree.
func (tree errorTree) diagnostics(drifts map[drift]struct{}) []string {
	diags := make([]string, 0, len(tree.diags))

	for _, list := range tree.diags {
		diags = append(diags, diagnosticTexts(list)...)
	}

	return normalizeDiagnostics(diags, drifts)
}

// text returns err's message with each [hcl.Diagnostics] normalized, or the empty string for nil.
func (tree errorTree) text(err error, drifts map[drift]struct{}) string {
	if err == nil {
		return ""
	}

	text := err.Error()

	for _, list := range tree.diags {
		if len(list) > 0 {
			text = strings.ReplaceAll(
				text,
				list.Error(),
				strings.Join(normalizeDiagnostics(diagnosticTexts(list), drifts), "; "),
			)
		}
	}

	if _, ok := drifts[driftSplitBodySuggestion]; ok {
		text = splitBodySuggestion.ReplaceAllString(text, "$1")
	}

	return text
}

// normalizeDiagnostics returns texts with drifts normalized.
func normalizeDiagnostics(texts []string, drifts map[drift]struct{}) []string {
	normalized := slices.Clone(texts)

	if _, ok := drifts[driftSplitBodySuggestion]; ok {
		for i, text := range normalized {
			normalized[i] = splitBodySuggestion.ReplaceAllString(text, "$1")
		}
	}

	if _, ok := drifts[driftDiagnosticOrder]; ok {
		slices.Sort(normalized)
	}

	if _, ok := drifts[driftJSONShapeRepeated]; ok {
		normalized = slices.Compact(normalized)
	}

	return normalized
}

// diagnosticTexts renders each diagnostic of diags with its subject and detail.
func diagnosticTexts(diags hcl.Diagnostics) []string {
	texts := make([]string, 0, len(diags))

	for _, diag := range diags {
		texts = append(
			texts,
			fmt.Sprintf("%d|%s|%s|%s", diag.Severity, diag.Error(), diag.Detail, rangeText(diag.Subject)),
		)
	}

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
		fields = append(fields, fmt.Sprintf(`%q:{"sensitive":false,"type":"string","value":%q}`, name, outputs[name]))
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
				vfs.WriteFile(fsys, filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o644),
			)
		}

		return fsys
	}
}

// fixtureSet is the parity fixtures, read from disk once.
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

	osFS := vfs.NewOSFS()

	fixtures, err := filepath.Abs(fixturesDir)
	require.NoError(t, err)

	set := &fixtureSet{files: map[string][]byte{}}

	entries, err := vfs.ReadDir(osFS, fixtures)
	require.NoError(t, err)

	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}

		path := filepath.Join(fixtures, entry.Name())
		set.files[path], err = vfs.ReadFile(osFS, path)
		require.NoError(t, err)
	}

	for _, rootName := range rootNames {
		err := vfs.WalkDir(
			osFS,
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

				if !d.Type().IsRegular() {
					return nil
				}

				if len(set.files) >= maxFixtureFiles {
					return fmt.Errorf("fixture trees hold more than %d files", maxFixtureFiles)
				}

				set.files[path], err = vfs.ReadFile(osFS, path)
				if err != nil {
					return err
				}

				if d.Name() == pkgconfig.DefaultTerragruntConfigPath {
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

// newTestParsingContext mirrors pkg/config's test helper of the same name, with its own caches.
func newTestParsingContext(t *testing.T, cfgPath string) (context.Context, *pkgconfig.ParsingContext) {
	t.Helper()

	ctx := pkgconfig.WithConfigValues(t.Context())
	pctx := pkgconfig.NewParsingContext(pkgconfig.WithStrictControls(controls.New()))

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
	pctx.TofuImplementation = tfimpl.Unknown
	pctx.Experiments = experiment.NewExperiments()
	pctx.Telemetry = new(telemetry.Options)
	pctx.EngineOptions = new(engine.EngineOptions)
	pctx.FeatureFlags = map[string]string{}

	return ctx, pctx
}
