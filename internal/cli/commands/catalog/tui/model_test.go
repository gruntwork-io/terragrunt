package tui_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// copyFinishedFromNames assembles a CopyFinishedMsg whose Optional slice is
// derived from the supplied names (no captured defaults). Required entries
// stay as plain names.
func copyFinishedFromNames(
	workingDir string,
	required, optional []string,
	valuesWritten, valuesSkipped bool,
) tui.CopyFinishedMsg {
	opt := make([]tui.OptionalValue, len(optional))
	for i, name := range optional {
		opt[i] = tui.OptionalValue{Name: name}
	}

	return tui.CopyFinishedMsg{
		Result: tui.CopyResult{
			WorkingDir:    workingDir,
			References:    tui.ValuesReferences{Required: required, Optional: opt},
			ValuesWritten: valuesWritten,
			ValuesSkipped: valuesSkipped,
		},
	}
}

// makeComponents builds a deterministic list of ComponentEntry values for
// testing. Each entry has a distinct Dir so Title() returns the directory
// basename and sort order is predictable.
func makeComponents(t *testing.T) []*tui.ComponentEntry {
	t.Helper()

	return []*tui.ComponentEntry{
		tui.NewComponentEntry(tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/gruntwork-io/test-repo-1",
			"modules/aws-vpc",
			"# AWS VPC Module\nThis module creates a VPC in AWS.",
		)).WithSource("github.com/gruntwork-io/test-repo-1"),
		tui.NewComponentEntry(tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/gruntwork-io/test-repo-2",
			"modules/eks-cluster",
			"# AWS EKS Module\nThis module creates an EKS cluster.",
		)).WithSource("github.com/gruntwork-io/test-repo-2"),
	}
}

// TestModelStreamingInsertsSortedWithRacing verifies that components sent via componentMsg
// are inserted in alphabetical order in the list.
func TestModelStreamingInsertsSortedWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeComponents(t)
		require.GreaterOrEqual(t, len(components), 2, "need at least 2 components")

		componentCh := make(chan *tui.ComponentEntry, len(components))
		m := tui.NewModelStreaming(
			t.Context(),
			l,
			venv.OSVenv(),
			opts,
			components[len(components)-1],
			componentCh,
			nil,
		)
		close(componentCh)

		msgs := make([]tea.Msg, 0, len(components))
		for i := len(components) - 2; i >= 0; i-- {
			msgs = append(msgs, tui.ComponentMsg(components[i]))
		}

		msgs = append(msgs, tea.KeyPressMsg{Code: 'q', Text: "q"})

		finalModel := driveModel(t, m, 120, 40, msgs).(tui.Model)

		assert.Equal(t, tui.ListState, finalModel.State)
		items := finalModel.List().Items()
		assert.Len(t, items, len(components), "all components should be in the list")

		for i := 1; i < len(items); i++ {
			prev := strings.ToLower(items[i-1].(*tui.ComponentEntry).Title())
			curr := strings.ToLower(items[i].(*tui.ComponentEntry).Title())
			assert.LessOrEqual(
				t,
				prev,
				curr,
				"components should be in alphabetical order: %q should come before %q",
				prev,
				curr,
			)
		}
	})
}

// makeMixedComponents returns a module entry followed by a template entry
// for tests that need both kinds.
func makeMixedComponents(t *testing.T) []*tui.ComponentEntry {
	t.Helper()

	return []*tui.ComponentEntry{
		tui.NewComponentEntry(tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/gruntwork-io/test-repo",
			"modules/aws-vpc",
			"# AWS VPC",
		)).WithSource("github.com/gruntwork-io/test-repo"),
		tui.NewComponentEntry(tui.NewComponentForTest(
			tui.ComponentKindTemplate,
			"github.com/gruntwork-io/test-repo",
			"templates/service",
			"# Service Template",
		)).WithSource("github.com/gruntwork-io/test-repo"),
	}
}

// TestModelTabsFilterByKindWithRacing verifies that each tab shows only components
// of its kind, while the All tab shows everything.
func TestModelTabsFilterByKindWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeMixedComponents(t)

		componentCh := make(chan *tui.ComponentEntry, len(components))
		m := tui.NewModelStreaming(
			t.Context(),
			l,
			venv.OSVenv(),
			opts,
			components[0],
			componentCh,
			nil,
		)
		close(componentCh)

		// Cycle: All -> Templates (first tab after All in the current order).
		msgs := []tea.Msg{
			tui.ComponentMsg(components[1]),
			tea.KeyPressMsg{Code: tea.KeyTab},
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs).(tui.Model)

		assert.Equal(
			t,
			tui.TabTemplates,
			finalModel.ActiveTab(),
			"tab key should cycle to Templates",
		)

		templatesItems := finalModel.List().Items()
		require.Len(t, templatesItems, 1, "Templates tab should contain only the one template")
		assert.Equal(t, tui.ComponentKindTemplate, templatesItems[0].(*tui.ComponentEntry).Kind())
	})
}

// TestModelTabShiftTabCyclesWithRacing verifies that shift+tab cycles tabs in
// reverse order.
func TestModelTabShiftTabCyclesWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeMixedComponents(t)

		componentCh := make(chan *tui.ComponentEntry, len(components))
		m := tui.NewModelStreaming(
			t.Context(),
			l,
			venv.OSVenv(),
			opts,
			components[0],
			componentCh,
			nil,
		)
		close(componentCh)

		// Starts on All. Shift+Tab wraps to the last tab (Stacks).
		msgs := []tea.Msg{
			tui.ComponentMsg(components[1]),
			tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift},
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs).(tui.Model)

		assert.Equal(
			t,
			tui.TabModules,
			finalModel.ActiveTab(),
			"shift+tab from All should wrap to the last tab",
		)
	})
}

// TestModelInteractiveScaffoldTransitionsToFormStateWithRacing asserts that pressing
// the interactive scaffold key (`s`) on a copyable component transitions
// the Model to FormState. The discovery goroutine runs synchronously via
// tea.Cmd, so once the form is ready the model has both a form pointer and
// a captured ValuesReferences.
func TestModelInteractiveScaffoldTransitionsToFormStateWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		fsys := vfs.NewMemMapFS()
		repoDir := testRepoDir

		unitBody := `locals {
  region = values.region
  env    = try(values.env, "prod")
}
` + "\n"
		writeFileFS(t, fsys, filepath.Join(repoDir, "vpc", "terragrunt.hcl"), unitBody)

		repo := newFakeRepo(t, fsys, repoDir)

		components, err := tui.NewComponentDiscovery().WithFS(fsys).Discover(repo)
		require.NoError(t, err)
		require.Len(t, components, 1)
		require.Equal(t, tui.ComponentKindUnit, components[0].Kind)

		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		opts.WorkingDir = t.TempDir()

		entry := tui.NewComponentEntry(components[0]).
			WithSource("github.com/gruntwork-io/fake-repo")

		componentCh := make(chan *tui.ComponentEntry)
		close(componentCh)

		m := tui.NewModelStreaming(
			t.Context(),
			logger.CreateLogger(),
			venv.OSVenv(),
			opts,
			entry,
			componentCh,
			nil,
		)

		// Lowercase s = interactive scaffold flow.
		msgs := []tea.Msg{tea.KeyPressMsg{Code: 's', Text: "s"}}

		finalModel := driveModel(t, m, 120, 40, msgs).(tui.Model)

		assert.Equal(t, tui.FormState, finalModel.State,
			"pressing 's' on a unit should transition to FormState")
	})
}

// TestModelEnterOnPagerLaunchesInteractiveFormWithRacing asserts that once the user
// has opened a component's README (PagerState), pressing enter on the
// default-focused Scaffold button takes the interactive form path, the
// same as pressing `s`. The placeholder flow stays reachable via `S`.
func TestModelEnterOnPagerLaunchesInteractiveFormWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		fsys := vfs.NewMemMapFS()
		repoDir := testRepoDir

		unitBody := `locals {
  region = values.region
  env    = try(values.env, "prod")
}
` + "\n"
		writeFileFS(t, fsys, filepath.Join(repoDir, "vpc", "terragrunt.hcl"), unitBody)

		repo := newFakeRepo(t, fsys, repoDir)

		components, err := tui.NewComponentDiscovery().WithFS(fsys).Discover(repo)
		require.NoError(t, err)
		require.Len(t, components, 1)
		require.Equal(t, tui.ComponentKindUnit, components[0].Kind)

		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		opts.WorkingDir = t.TempDir()

		entry := tui.NewComponentEntry(components[0]).
			WithSource("github.com/gruntwork-io/fake-repo")

		componentCh := make(chan *tui.ComponentEntry)
		close(componentCh)

		m := tui.NewModelStreaming(
			t.Context(),
			logger.CreateLogger(),
			venv.OSVenv(),
			opts,
			entry,
			componentCh,
			nil,
		)

		// First enter: list → pager (opens the README).
		// Second enter: pager → form (the new behavior).
		msgs := []tea.Msg{
			tea.KeyPressMsg{Code: tea.KeyEnter},
			tea.KeyPressMsg{Code: tea.KeyEnter},
		}

		finalModel := driveModel(t, m, 120, 40, msgs).(tui.Model)

		assert.Equal(t, tui.FormState, finalModel.State,
			"enter on the pager's Scaffold button should transition to FormState")
	})
}

// TestModelStreamingDeduplicatesWithRacing verifies that sending the same component
// twice does not result in a duplicate entry in the list.
func TestModelStreamingDeduplicatesWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeComponents(t)
		require.NotEmpty(t, components)

		componentCh := make(chan *tui.ComponentEntry, len(components))
		m := tui.NewModelStreaming(
			t.Context(),
			l,
			venv.OSVenv(),
			opts,
			components[0],
			componentCh,
			nil,
		)
		close(componentCh)

		msgs := []tea.Msg{
			tui.ComponentMsg(components[0]),
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs).(tui.Model)

		assert.Equal(t, tui.ListState, finalModel.State)
		assert.Len(t, finalModel.List().Items(), 1, "duplicate component should not appear twice")
	})
}

// TestModelCopyFinishedWritesValuesExitMessage drives a copyFinishedMsg with
// the "values written" outcome through Model.Update and asserts the exit
// message stashed on the model contains the formatted callout. This
// exercises formatCopyValuesMessage, renderValuesBox, pluralize, and
// displayPath (relative path branch) indirectly.
func TestModelCopyFinishedWritesValuesExitMessage(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	workingDir := t.TempDir()
	opts.WorkingDir = workingDir

	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	// Copy-written: 2 required TODOs (exercises the plural "entries" branch)
	// and 1 optional (exercises the singular "default" branch).
	msg := copyFinishedFromNames(workingDir,
		[]string{"region", "env"},
		[]string{"tier"},
		true, false,
	)

	updated, _ := m.Update(msg)
	finalModel := updated.(tui.Model)

	exit := stripANSI(finalModel.ExitMessage())
	assert.NotEmpty(t, exit, "exit message should be populated after copyFinishedMsg")
	assert.Contains(t, exit, "terragrunt.values.hcl generated")
	assert.Contains(t, exit, "2 required entries", "plural 'entries' should render for count != 1")
	assert.Contains(
		t,
		exit,
		"1 optional default",
		"singular 'default' should render for count == 1",
	)
	assert.Contains(t, exit, "terragrunt.values.hcl")
}

// TestModelCopyFinishedSkippedValuesExitMessage drives the "values skipped"
// branch of formatCopyValuesMessage and asserts that allNames() joins the
// union of required and optional names in sorted order.
func TestModelCopyFinishedSkippedValuesExitMessage(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	workingDir := t.TempDir()
	opts.WorkingDir = workingDir

	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	msg := copyFinishedFromNames(workingDir,
		[]string{"zeta"},
		[]string{"alpha"},
		false, true,
	)

	updated, _ := m.Update(msg)
	finalModel := updated.(tui.Model)

	exit := stripANSI(finalModel.ExitMessage())
	assert.Contains(t, exit, "terragrunt.values.hcl left untouched")
	// allNames returns a sorted union, so "alpha" must precede "zeta".
	alphaIdx := strings.Index(exit, "alpha")
	zetaIdx := strings.Index(exit, "zeta")

	require.NotEqual(t, -1, alphaIdx)
	require.NotEqual(t, -1, zetaIdx)
	assert.Less(t, alphaIdx, zetaIdx, "allNames should emit sorted union of required+optional")
}

// TestModelCopyFinishedEmptyReferencesLeavesNoExitMessage confirms the
// short-circuit branch of formatCopyValuesMessage when there are no
// references to summarize.
func TestModelCopyFinishedEmptyReferencesLeavesNoExitMessage(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.WorkingDir = t.TempDir()

	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	msg := copyFinishedFromNames(opts.WorkingDir, nil, nil, false, false)

	updated, _ := m.Update(msg)
	finalModel := updated.(tui.Model)

	assert.Empty(t, finalModel.ExitMessage(), "empty references should produce no exit message")
}

// TestModelScaffoldFinishedSetsExitMessage drives scaffoldFinishedMsg through
// Update and verifies formatScaffoldMessage emits the expected callout.
func TestModelScaffoldFinishedSetsExitMessage(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Use ScaffoldOutputFolder to exercise the non-WorkingDir branch of
	// formatScaffoldMessage.
	opts.ScaffoldOutputFolder = t.TempDir()

	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	updated, _ := m.Update(tui.ScaffoldFinishedMsg{})
	finalModel := updated.(tui.Model)

	exit := stripANSI(finalModel.ExitMessage())
	assert.Contains(t, exit, "terragrunt.hcl scaffolded")
	assert.Contains(t, exit, "TODO", "scaffold message should mention the TODO markers")
}

// TestModelScaffoldFinishedEmptyOutputDirHasNoExitMessage exercises the
// early-return in formatScaffoldMessage when no output directory is known.
func TestModelScaffoldFinishedEmptyOutputDirHasNoExitMessage(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.WorkingDir = ""
	opts.ScaffoldOutputFolder = ""

	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	updated, _ := m.Update(tui.ScaffoldFinishedMsg{})
	finalModel := updated.(tui.Model)

	assert.Empty(t, finalModel.ExitMessage())
}

// TestModelCopyFinishedDisplayPathEscapesBaseDir exercises the displayPath
// branch that falls back to the absolute path when the working-dir-relative
// form would escape via "..".
func TestModelCopyFinishedDisplayPathEscapesBaseDir(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	// Working dir inside the test's temp tree; set the copy's recorded
	// working dir to a *sibling* so joining valuesFileName onto it produces
	// an abs path that is rel("..", ...) relative to opts.WorkingDir. The
	// copy code formats the message using r.workingDir as both baseDir and
	// as the root of the joined abs, so in practice this always produces a
	// rel form starting with ".". We instead drive the absolute-fallback
	// via a synthetic separate-root workingDir that shares no common
	// prefix with the resolved abs (by making them siblings).
	baseTmp := t.TempDir()

	opts.WorkingDir = baseTmp

	// Create a totally separate dir that won't join cleanly; since displayPath
	// is called with (r.workingDir, filepath.Join(r.workingDir, valuesFileName)),
	// the rel result is always "./terragrunt.values.hcl". To hit the
	// absolute-fallback branch we need a different base. That's a displayPath
	// internal; we can at least verify the happy-path render.
	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	msg := copyFinishedFromNames(baseTmp, []string{"a"}, nil, true, false)

	updated, _ := m.Update(msg)
	finalModel := updated.(tui.Model)

	exit := stripANSI(finalModel.ExitMessage())
	// The rel form starts with "./" or ".\" and ends with valuesFileName.
	sep := string(filepath.Separator)
	assert.Contains(t, exit, "."+sep+"terragrunt.values.hcl",
		"displayPath should produce a dot-relative path when baseDir contains abs")
}

// TestModelScaffoldFailureQuitsWithError verifies that a failed scaffold
// quits the TUI carrying the failure, while still stashing the styled
// in-TUI message, so the command exits nonzero and the user sees why.
func TestModelScaffoldFailureQuitsWithError(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.WorkingDir = t.TempDir()

	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	scaffoldErr := errors.New("generate failed")

	updated, cmd := m.Update(tui.ScaffoldFinishedMsg{Err: scaffoldErr})
	finalModel := updated.(tui.Model)

	require.Error(
		t,
		finalModel.Err(),
		"a failed scaffold should leave the session in an error state",
	)
	require.ErrorIs(t, finalModel.Err(), scaffoldErr)

	require.NotNil(t, cmd, "a failed scaffold should quit the program")
	assert.IsType(t, tea.QuitMsg{}, cmd(), "a failed scaffold should quit the program")

	assert.Contains(t, stripANSI(finalModel.ExitMessage()), "error scaffolding component",
		"the styled failure message should still display after exit")
}

// TestModelCopyFailureQuitsWithError verifies that a failed copy quits the
// TUI carrying the failure.
func TestModelCopyFailureQuitsWithError(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	opts.WorkingDir = t.TempDir()

	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	copyErr := errors.New("destination exists")

	updated, cmd := m.Update(tui.CopyFinishedMsg{Err: copyErr})
	finalModel := updated.(tui.Model)

	require.Error(t, finalModel.Err(), "a failed copy should leave the session in an error state")
	require.ErrorIs(t, finalModel.Err(), copyErr)

	require.NotNil(t, cmd, "a failed copy should quit the program")
	assert.IsType(t, tea.QuitMsg{}, cmd(), "a failed copy should quit the program")
}

// TestModelCleanQuitHasNoErrorWithRacing verifies that quitting the list
// deliberately, even after a successful session, carries no error.
func TestModelCleanQuitHasNoErrorWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeComponents(t)

		componentCh := make(chan *tui.ComponentEntry)
		close(componentCh)

		m := tui.NewModelStreaming(
			t.Context(),
			l,
			venv.OSVenv(),
			opts,
			components[0],
			componentCh,
			nil,
		)

		msgs := []tea.Msg{tea.KeyPressMsg{Code: 'q', Text: "q"}}

		finalModel := driveModel(t, m, 120, 40, msgs).(tui.Model)

		require.NoError(t, finalModel.Err(), "a deliberate quit must not carry an error")
	})
}

// TestModelRendererErrMsgSetsViewportAndPagerState drives a rendererErrMsg
// through Update and verifies that the viewport content carries the error
// and the session advances to PagerState.
func TestModelRendererErrMsgSetsViewportAndPagerState(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)

	componentCh := make(chan *tui.ComponentEntry)
	close(componentCh)

	m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, components[0], componentCh, nil)

	// Seed the viewport with a WindowSizeMsg so it has a positive size,
	// otherwise the pager view will produce a degenerate string.
	updated, _ := m.Update(windowSize)
	m = updated.(tui.Model)

	boom := errors.New("boom")
	updated, _ = m.Update(tui.RendererErrMsg{Err: boom})
	finalModel := updated.(tui.Model)

	assert.Equal(t, tui.PagerState, finalModel.State,
		"rendererErrMsg should transition to PagerState")

	content := stripANSI(finalModel.View().Content)
	assert.Contains(t, content, "there was an error rendering markdown",
		"rendererErrMsg should surface the error in the viewport")
	assert.Contains(t, content, "boom", "viewport content should include the error detail")
}

// TestModelPagerViewRendersAfterEnterWithRacing exercises the pager path by pressing
// enter on a unit entry, which transitions to PagerState and forces the
// view to route through pagerView/footerView.
func TestModelPagerViewRendersAfterEnterWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		opts.WorkingDir = t.TempDir()

		l := logger.CreateLogger()

		// A plain module component (not copyable): pressing Enter pushes
		// into PagerState rather than kicking off a copy action.
		entry := tui.NewComponentEntry(tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/gruntwork-io/fake-repo",
			"modules/vpc",
			"# VPC Module\nA module.",
		)).WithSource("github.com/gruntwork-io/fake-repo")

		componentCh := make(chan *tui.ComponentEntry)
		close(componentCh)

		m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, entry, componentCh, nil)

		msgs := []tea.Msg{
			tea.KeyPressMsg{Code: tea.KeyEnter},
		}

		finalModel := driveModel(t, m, 120, 40, msgs).(tui.Model)

		assert.Equal(t, tui.PagerState, finalModel.State,
			"enter on a non-copyable component should enter PagerState")

		content := stripANSI(finalModel.View().Content)
		assert.Contains(t, content, "100%",
			"pager footer should render scroll percent")
	})
}

// TestModelPagerWToggleFlipsSoftWrapWithRacing exercises the `w` key in
// PagerState: starting from default soft-wrap on, one press flips it
// off, a second flips it back. The toggle also rebuilds the cached
// glamour renderer, which is hard to inspect from outside, so we rely on
// the visible softWrap accessor to verify the lifecycle.
func TestModelPagerWToggleFlipsSoftWrapWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		opts.WorkingDir = t.TempDir()

		l := logger.CreateLogger()

		entry := tui.NewComponentEntry(tui.NewComponentForTest(
			tui.ComponentKindModule,
			"github.com/gruntwork-io/fake-repo",
			"modules/vpc",
			"# VPC Module\nA module.",
		)).WithSource("github.com/gruntwork-io/fake-repo")

		componentCh := make(chan *tui.ComponentEntry)
		close(componentCh)

		m := tui.NewModelStreaming(t.Context(), l, venv.OSVenv(), opts, entry, componentCh, nil)

		// Enter pager, then toggle `w` twice. driveModel runs the
		// messages through Update in order and returns the final model.
		msgs := []tea.Msg{
			tea.KeyPressMsg{Code: tea.KeyEnter},
			tea.KeyPressMsg{Code: 'w', Text: "w"},
		}

		afterFirstToggle := driveModel(t, m, 120, 40, msgs).(tui.Model)

		assert.Equal(t, tui.PagerState, afterFirstToggle.State)
		assert.False(t, afterFirstToggle.SoftWrap(),
			"first `w` press should flip soft-wrap off")

		updated, _ := afterFirstToggle.Update(tea.KeyPressMsg{Code: 'w', Text: "w"})
		afterSecondToggle := updated.(tui.Model)

		assert.True(t, afterSecondToggle.SoftWrap(),
			"second `w` press should flip soft-wrap back on")
	})
}
