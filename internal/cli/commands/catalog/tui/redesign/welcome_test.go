package redesign_test

import (
	"bytes"
	"context"
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWelcomeLoadingScreen_NoSources verifies that when discovery finds no
// module sources, the welcome model stays on the no-sources help screen.
func TestWelcomeLoadingScreen_NoSources(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *module.Module) (catalog.CatalogService, error) {
		return nil, nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		// Wait for discovery to complete and settle into no-sources view
		time.Sleep(200 * time.Millisecond)

		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	// Should still be a WelcomeModel (no transition to module list)
	_, isWelcome := finalModel.(redesign.WelcomeModel)
	assert.True(t, isWelcome, "should remain on welcome screen when no sources found")
}

// TestWelcomeLoadingScreen_TransitionsToModuleList verifies that when
// discovery finds modules, the welcome model transitions to the full
// module list TUI.
func TestWelcomeLoadingScreen_TransitionsToModuleList(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	svc := createMockCatalogService(t, opts)

	withModulesLoad := func(_ context.Context, _ redesign.StatusFunc, moduleCh chan<- *module.Module) (catalog.CatalogService, error) {
		for _, mod := range svc.Modules() {
			moduleCh <- mod
		}

		return svc, nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, withModulesLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		// Wait for discovery and transition to module list
		time.Sleep(200 * time.Millisecond)

		// Quit from the module list
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	listModel, isList := finalModel.(redesign.Model)
	require.True(t, isList, "should transition to module list when modules found")
	assert.Equal(t, redesign.ListState, listModel.State)
	assert.Len(t, listModel.List.Items(), 2, "should have 2 modules in the list")
	require.NotNil(t, listModel.SVC, "SVC should be set after discovery completes")
	assert.Len(t, listModel.SVC.Modules(), 2, "should have 2 test modules")
}

// TestWelcomeLoadingScreen_ModuleListNavigation verifies the full flow:
// loading → module list → select module details → back to list → quit.
func TestWelcomeLoadingScreen_ModuleListNavigation(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	svc := createMockCatalogService(t, opts)

	withModulesLoad := func(_ context.Context, _ redesign.StatusFunc, moduleCh chan<- *module.Module) (catalog.CatalogService, error) {
		for _, mod := range svc.Modules() {
			moduleCh <- mod
		}

		return svc, nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, withModulesLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		// Wait for discovery and transition to module list
		time.Sleep(200 * time.Millisecond)

		// Press Enter to select the first module (navigate to details)
		p.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		time.Sleep(50 * time.Millisecond)

		// Press q to go back to list
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
		time.Sleep(50 * time.Millisecond)

		// Press q again to quit
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	listModel, isList := finalModel.(redesign.Model)
	require.True(t, isList, "should be on module list after navigating back")
	assert.Equal(t, redesign.ListState, listModel.State)
}

// TestWelcomeLoadingScreen_QuitDuringLoading verifies that pressing q
// during the loading phase exits cleanly.
func TestWelcomeLoadingScreen_QuitDuringLoading(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	slowLoad := func(ctx context.Context, _ redesign.StatusFunc, _ chan<- *module.Module) (catalog.CatalogService, error) {
		// Simulate slow discovery
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
		}

		return nil, nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, slowLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		// Quit immediately while still loading
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	// Should still be a WelcomeModel (loading was interrupted)
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

	noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *module.Module) (catalog.CatalogService, error) {
		return nil, nil
	}

	var openedURL string

	m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad).
		WithOpenURL(func(url string) error {
			openedURL = url
			return nil
		})

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		// Wait for discovery to complete
		time.Sleep(200 * time.Millisecond)

		// Press h to open docs
		p.Send(tea.KeyPressMsg{Code: 'h', Text: "h"})
		time.Sleep(50 * time.Millisecond)

		// Quit
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

	noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *module.Module) (catalog.CatalogService, error) {
		return nil, nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		// Wait for discovery to complete
		time.Sleep(200 * time.Millisecond)

		// Press an unhandled key — should be ignored
		p.Send(tea.KeyPressMsg{Code: 'x', Text: "x"})
		time.Sleep(50 * time.Millisecond)

		// Quit
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	_, isWelcome := finalModel.(redesign.WelcomeModel)
	assert.True(t, isWelcome, "should remain on welcome screen after pressing unhandled key")
}

// TestWelcomeStreamingModules verifies that modules stream into the list
// one at a time, ending up in sorted order.
func TestWelcomeStreamingModules(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	svc := createMockCatalogService(t, opts)

	modules := svc.Modules()
	require.GreaterOrEqual(t, len(modules), 2, "need at least 2 modules for streaming test")

	streamingLoad := func(_ context.Context, _ redesign.StatusFunc, moduleCh chan<- *module.Module) (catalog.CatalogService, error) {
		// Stream modules one at a time with a small delay to simulate real discovery
		for _, mod := range modules {
			moduleCh <- mod

			time.Sleep(20 * time.Millisecond)
		}

		return svc, nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, streamingLoad)

	finalModel := runTeaModel(t, m, 120, 40, func(p *tea.Program) {
		// Wait for all modules to stream in
		time.Sleep(500 * time.Millisecond)

		// Quit
		p.Send(tea.KeyPressMsg{Code: 'q', Text: "q"})
	})

	listModel, isList := finalModel.(redesign.Model)
	require.True(t, isList, "should transition to module list")
	assert.Equal(t, redesign.ListState, listModel.State)
	assert.Len(t, listModel.List.Items(), len(modules), "all streamed modules should appear in list")

	// Verify alphabetical order
	items := listModel.List.Items()
	for i := 1; i < len(items); i++ {
		prev := items[i-1].(*module.Module).Title()
		curr := items[i].(*module.Module).Title()
		assert.LessOrEqual(t, prev, curr, "modules should be in alphabetical order")
	}
}

// runTeaModel starts a tea.Program with any tea.Model, sends messages via
// the interact callback, and returns the final tea.Model once the program
// exits. Unlike runModel, this accepts and returns the tea.Model interface
// so it works with both WelcomeModel and Model.
func runTeaModel(t *testing.T, m tea.Model, width, height int, interact func(p *tea.Program)) tea.Model {
	t.Helper()

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
