package redesign_test

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Welcome Loading View ---

func TestWelcomeLoadingView_RendersSpinnerAndStatus(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	m := redesign.NewWelcomeModel(t.Context(), l, opts, blockingLoad)
	m = updateModel(m, windowSize).(redesign.WelcomeModel)

	view := m.View()
	content := stripANSI(view.Content)

	assert.True(t, view.AltScreen, "welcome loading view should use alt screen")
	assert.Contains(t, content, "Terragrunt Catalog", "should render title")
	assert.Contains(t, content, "Discovering modules from your infrastructure...", "should render default status text")
}

func TestWelcomeLoadingView_StatusTextUpdates(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	m := redesign.NewWelcomeModel(t.Context(), l, opts, blockingLoad)
	m = updateModel(m, windowSize).(redesign.WelcomeModel)

	// Verify initial status
	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "Discovering modules from your infrastructure...")

	// Send a status update
	m = updateModel(m, redesign.StatusUpdateMsg("Loading terraform-aws-vpc...")).(redesign.WelcomeModel)

	content = stripANSI(m.View().Content)
	assert.Contains(t, content, "Loading terraform-aws-vpc...", "status text should update after StatusUpdateMsg")
	assert.NotContains(t, content, "Discovering modules from your infrastructure...", "old status text should be replaced")
}

// --- Welcome No-Sources View ---

func TestWelcomeNoSourcesView_RendersHelpText(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ModuleEntry) (catalog.CatalogService, error) {
		return nil, nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad)
	m = updateModel(m, windowSize).(redesign.WelcomeModel)

	// Simulate discovery completing with no modules
	m = updateModel(m, redesign.DiscoveryCompleteMsg{Svc: nil, Err: nil}).(redesign.WelcomeModel)

	view := m.View()
	content := stripANSI(view.Content)

	assert.True(t, view.AltScreen, "no-sources view should use alt screen")
	assert.Contains(t, content, "Terragrunt Catalog", "should render title")
	assert.Contains(t, content, "No module sources were discovered", "should explain no sources found")
	assert.Contains(t, content, "catalog {", "should show catalog block example")
	assert.Contains(t, content, "urls =", "should show urls attribute in example")
	assert.Contains(t, content, "terraform.source", "should mention terraform.source as alternative")
	assert.Contains(t, content, "h: open docs in browser", "should show docs key hint")
	assert.Contains(t, content, "q/esc: exit", "should show quit key hint")
}

// --- Module List View ---

func TestModuleListView_LoadingTitle(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()
	modules := svc.Modules()
	require.NotEmpty(t, modules)

	moduleCh := make(chan *redesign.ModuleEntry, 10)
	m := redesign.NewModelStreaming(l, opts, redesign.NewModuleEntry(modules[0]), moduleCh)

	updated, _ := m.Update(windowSize)
	m = updated.(redesign.Model)

	// Should show loading indicator
	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "List of Modules (loading...)", "streaming model should show loading indicator")

	// Send discoveryComplete to clear loading state
	updated, _ = m.Update(redesign.DiscoveryCompleteMsg{Svc: svc, Err: nil})
	m = updated.(redesign.Model)

	content = stripANSI(m.View().Content)
	assert.Contains(t, content, "List of Modules", "should still have title")
	assert.NotContains(t, content, "(loading...)", "loading indicator should be gone after discovery completes")
}

func TestModuleListView_MetadataRowRendered(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()
	modules := svc.Modules()
	require.NotEmpty(t, modules)

	entry := redesign.NewModuleEntry(modules[0]).
		WithVersion("v1.10.2").
		WithSource("github.com/gruntwork-io/terragrunt-scale-catalog")

	moduleCh := make(chan *redesign.ModuleEntry, 10)
	m := redesign.NewModelStreaming(l, opts, entry, moduleCh)

	updated, _ := m.Update(windowSize)
	m = updated.(redesign.Model)

	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "module", "metadata row should contain item type")
	assert.Contains(t, content, "github.com/gruntwork-io/terragrunt-scale-catalog", "metadata row should contain source")
	assert.Contains(t, content, "v1.10.2", "metadata row should contain version")
}

func TestModuleListView_NoVersionOmitsVersionPill(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	svc := createMockCatalogService(t, opts)
	l := logger.CreateLogger()
	modules := svc.Modules()
	require.NotEmpty(t, modules)

	entry := redesign.NewModuleEntry(modules[0]).
		WithSource("github.com/gruntwork-io/terragrunt-scale-catalog")

	moduleCh := make(chan *redesign.ModuleEntry, 10)
	m := redesign.NewModelStreaming(l, opts, entry, moduleCh)

	updated, _ := m.Update(windowSize)
	m = updated.(redesign.Model)

	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "module", "metadata row should contain item type")
	assert.Contains(t, content, "github.com/gruntwork-io/terragrunt-scale-catalog", "metadata row should contain source")
	assert.NotContains(t, content, "v1.10.2", "version pill should not appear when version is empty")
}

// --- synctest: Streaming Flow ---

func TestWelcomeStreamingFlow_Synctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		svc := createMockCatalogService(t, opts)
		modules := svc.Modules()
		require.GreaterOrEqual(t, len(modules), 2, "need at least 2 modules")

		streamingLoad := func(_ context.Context, status redesign.StatusFunc, moduleCh chan<- *redesign.ModuleEntry) (catalog.CatalogService, error) {
			status("Discovering catalog sources...")

			for _, mod := range modules {
				time.Sleep(100 * time.Millisecond)

				moduleCh <- redesign.NewModuleEntry(mod)
			}

			return svc, nil
		}

		var m tea.Model = redesign.NewWelcomeModel(t.Context(), l, opts, streamingLoad)

		// Set window size
		m = updateModel(m, windowSize)

		// Initially should be welcome model showing loading
		_, isWelcome := m.(redesign.WelcomeModel)
		assert.True(t, isWelcome, "should start as WelcomeModel")

		content := stripANSI(m.View().Content)
		assert.Contains(t, content, "Terragrunt Catalog", "should show title while loading")

		// Execute Init() to start discovery and spinners
		cmd := m.Init()

		// Collect the first message from Init commands
		msgCh := make(chan tea.Msg, 10)

		go func() {
			if cmd != nil {
				msg := cmd()
				if msg != nil {
					msgCh <- msg
				}
			}
		}()

		// Advance fake time past the first module's Sleep(100ms)
		time.Sleep(150 * time.Millisecond)

		// Drain available messages and feed them to the model
		draining := true
		for draining {
			select {
			case msg := <-msgCh:
				var nextCmd tea.Cmd

				m, nextCmd = m.Update(msg)

				if nextCmd != nil {
					go func() {
						result := nextCmd()
						if result != nil {
							msgCh <- result
						}
					}()
				}
			default:
				draining = false
			}
		}

		// Advance more time for remaining modules and discovery completion
		time.Sleep(500 * time.Millisecond)

		// Drain remaining messages
		draining = true
		for draining {
			select {
			case msg := <-msgCh:
				var nextCmd tea.Cmd

				m, nextCmd = m.Update(msg)

				if nextCmd != nil {
					go func() {
						result := nextCmd()
						if result != nil {
							msgCh <- result
						}
					}()
				}
			default:
				draining = false
			}
		}

		// After streaming completes, should have transitioned to Model
		listModel, isList := m.(redesign.Model)
		if isList {
			assert.Equal(t, redesign.ListState, listModel.State, "should be in list state")

			items := listModel.List.Items()
			assert.GreaterOrEqual(t, len(items), 1, "should have at least one module in the list")

			// Verify alphabetical order if multiple items present
			for i := 1; i < len(items); i++ {
				prev := items[i-1].(*redesign.ModuleEntry).Title()
				curr := items[i].(*redesign.ModuleEntry).Title()
				assert.LessOrEqual(t, strings.ToLower(prev), strings.ToLower(curr),
					"modules should be in alphabetical order: %q should come before %q", prev, curr)
			}
		}
	})
}

// --- synctest: Spinner Animation ---

func TestWelcomeLoadingSpinner_Synctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()

		slowLoad := func(ctx context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ModuleEntry) (catalog.CatalogService, error) {
			// Block for a long time (fake time) so we stay in loading state
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
			}

			return nil, nil
		}

		var m tea.Model = redesign.NewWelcomeModel(t.Context(), l, opts, slowLoad)

		m = updateModel(m, windowSize)

		// Execute Init() to get the initial command batch (includes spinner.Tick)
		cmd := m.Init()

		msgCh := make(chan tea.Msg, 10)

		go func() {
			if cmd != nil {
				msg := cmd()
				if msg != nil {
					msgCh <- msg
				}
			}
		}()

		// The spinner.Dot FPS is 100ms. Advance fake time past that.
		time.Sleep(150 * time.Millisecond)

		// Drain and process messages — the spinner tick should have fired
		draining := true
		for draining {
			select {
			case msg := <-msgCh:
				var nextCmd tea.Cmd

				m, nextCmd = m.Update(msg)

				if nextCmd != nil {
					go func() {
						result := nextCmd()
						if result != nil {
							msgCh <- result
						}
					}()
				}
			default:
				draining = false
			}
		}

		// After processing a spinner tick, the view should still render
		// the loading screen with spinner frame and status text
		view := m.View()
		content := stripANSI(view.Content)

		assert.Contains(t, content, "Terragrunt Catalog", "should still show title")
		assert.Contains(t, content, "Discovering modules from your infrastructure...", "should still show status")

		// Verify the view contains a spinner frame character (Dot spinner frames)
		dotFrames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
		hasSpinnerFrame := false

		for _, frame := range dotFrames {
			if strings.Contains(view.Content, frame) {
				hasSpinnerFrame = true

				break
			}
		}

		assert.True(t, hasSpinnerFrame, "loading view should contain a spinner frame character")
	})
}

// windowSize is a convenience WindowSizeMsg used across view tests.
var windowSize = tea.WindowSizeMsg{Width: 120, Height: 40}

// blockingLoad is a LoadFunc that blocks until the context is cancelled,
// keeping the WelcomeModel in the loading state.
func blockingLoad(ctx context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ModuleEntry) (catalog.CatalogService, error) {
	<-ctx.Done()

	return nil, nil
}

// updateModel sends a message to a tea.Model and returns the updated model,
// discarding commands.
func updateModel(m tea.Model, msg tea.Msg) tea.Model {
	updated, _ := m.Update(msg)

	return updated
}

// stripANSI removes ANSI escape sequences from a string so assertions
// can match on plain text.
func stripANSI(s string) string {
	var out strings.Builder

	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			// Skip until we hit a letter (the terminator of the escape sequence).
			for i < len(s) && !isLetter(s[i]) {
				i++
			}

			continue
		}

		out.WriteByte(s[i])
	}

	return out.String()
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
