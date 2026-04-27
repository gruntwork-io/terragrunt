package redesign_test

import (
	"bytes"
	"context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWelcomeLoadingScreen_NoSources verifies that when discovery finds no
// component sources, the welcome model stays on the no-sources help screen.
func TestWelcomeLoadingScreen_NoSources(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
		return nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		time.Sleep(200 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	_, isWelcome := finalModel.(redesign.WelcomeModel)
	assert.True(t, isWelcome, "should remain on welcome screen when no sources found")
}

// TestWelcomeLoadingScreen_TransitionsToComponentList verifies that when
// discovery finds components, the welcome model transitions to the full
// component list TUI.
func TestWelcomeLoadingScreen_TransitionsToComponentList(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)

	withComponentsLoad := func(_ context.Context, _ redesign.StatusFunc, componentCh chan<- *redesign.ComponentEntry) error {
		for _, c := range components {
			componentCh <- c
		}

		return nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, withComponentsLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		time.Sleep(200 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	listModel, isList := finalModel.(redesign.Model)
	require.True(t, isList, "should transition to component list when components found")
	assert.Equal(t, redesign.ListState, listModel.State)
	assert.Len(t, listModel.List().Items(), len(components), "should have all streamed components in list")
}

// TestWelcomeLoadingScreen_ComponentListNavigation verifies the full flow:
// loading → component list → select component details → back to list → quit.
func TestWelcomeLoadingScreen_ComponentListNavigation(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)

	withComponentsLoad := func(_ context.Context, _ redesign.StatusFunc, componentCh chan<- *redesign.ComponentEntry) error {
		for _, c := range components {
			componentCh <- c
		}

		return nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, withComponentsLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		time.Sleep(200 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		time.Sleep(50 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
		time.Sleep(50 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	listModel, isList := finalModel.(redesign.Model)
	require.True(t, isList, "should be on component list after navigating back")
	assert.Equal(t, redesign.ListState, listModel.State)
}

// TestWelcomeLoadingScreen_QuitDuringLoading verifies that pressing q
// during the loading phase exits cleanly.
func TestWelcomeLoadingScreen_QuitDuringLoading(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	slowLoad := func(ctx context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
		}

		return nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, slowLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	_, isWelcome := finalModel.(redesign.WelcomeModel)
	assert.True(t, isWelcome, "should exit as WelcomeModel when quit during loading")
}

// TestWelcomeNoSourcesScreen_HelpKeyOpensDocs verifies that pressing h on
// the no-sources screen triggers the open-URL function with the docs URL.
func TestWelcomeNoSourcesScreen_HelpKeyOpensDocs(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
		return nil
	}

	var openedURL string

	m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad).
		WithOpenURL(func(url string) error {
			openedURL = url
			return nil
		})

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		time.Sleep(200 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'h', Text: "h"})
		time.Sleep(50 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	_, isWelcome := finalModel.(redesign.WelcomeModel)
	assert.True(t, isWelcome, "should remain on welcome screen after pressing h")
	assert.Equal(t, "https://docs.terragrunt.com/features/catalog/", openedURL, "should have opened docs URL")
}

// TestWelcomeNoSourcesScreen_UnhandledKey verifies that pressing an
// unrecognized key on the no-sources screen does not crash or change state.
func TestWelcomeNoSourcesScreen_UnhandledKey(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
		return nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		time.Sleep(200 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'x', Text: "x"})
		time.Sleep(50 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	_, isWelcome := finalModel.(redesign.WelcomeModel)
	assert.True(t, isWelcome, "should remain on welcome screen after pressing unhandled key")
}

// TestWelcomeStreamingComponents verifies that components stream into the list
// one at a time, ending up in sorted order.
func TestWelcomeStreamingComponents(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)
	require.GreaterOrEqual(t, len(components), 2, "need at least 2 components for streaming test")

	streamingLoad := func(_ context.Context, _ redesign.StatusFunc, componentCh chan<- *redesign.ComponentEntry) error {
		for _, c := range components {
			componentCh <- c

			time.Sleep(20 * time.Millisecond)
		}

		return nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, streamingLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		time.Sleep(500 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	listModel, isList := finalModel.(redesign.Model)
	require.True(t, isList, "should transition to component list")
	assert.Equal(t, redesign.ListState, listModel.State)
	assert.Len(t, listModel.List().Items(), len(components), "all streamed components should appear in list")

	items := listModel.List().Items()
	for i := 1; i < len(items); i++ {
		prev := strings.ToLower(items[i-1].(*redesign.ComponentEntry).Title())
		curr := strings.ToLower(items[i].(*redesign.ComponentEntry).Title())
		assert.LessOrEqual(t, prev, curr, "components should be in alphabetical order")
	}
}

// runTeaModel starts a tea.Program with any tea.Model, sends messages via
// the interact callback, and returns the final tea.Model once the program
// exits. Unlike runModel, this accepts and returns the tea.Model interface
// so it works with both WelcomeModel and Model.
func runTeaModel(t *testing.T, m tea.Model, width, height int, interact func(p *tea.Program)) tea.Model {
	t.Helper()

	// TODO(windows): bubbletea ignores the input pipe on Windows and hangs on
	// ReadConsole in headless CI. Re-enable when that is fixed upstream.
	if runtime.GOOS == "windows" {
		t.Skip("bubbletea hangs reading the console on Windows CI")
	}

	var out bytes.Buffer

	pr, pw, err := os.Pipe()
	require.NoError(t, err)

	defer pr.Close()
	defer pw.Close()

	p := tea.NewProgram(m,
		tea.WithInput(pr),
		tea.WithOutput(&out),
		tea.WithWindowSize(width, height),
		tea.WithColorProfile(colorprofile.TrueColor),
	)

	done := make(chan tea.Model, 1)

	go func() {
		finalModel, err := p.Run()
		assert.NoError(t, err)

		done <- finalModel
	}()

	// Give the program a moment to start and process the initial WindowSizeMsg.
	time.Sleep(50 * time.Millisecond)

	interact(p)

	select {
	case fm := <-done:
		return fm
	case <-time.After(10 * time.Second):
		p.Kill()
		t.Fatal("program did not exit within timeout")

		return nil
	}
}
