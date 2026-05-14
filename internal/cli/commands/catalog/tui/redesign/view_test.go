package redesign_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
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
	assert.Contains(t, content, "Discovering components from your infrastructure...", "should render default status text")
}

func TestWelcomeLoadingView_StatusTextUpdates(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	m := redesign.NewWelcomeModel(t.Context(), l, opts, blockingLoad)
	m = updateModel(m, windowSize).(redesign.WelcomeModel)

	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "Discovering components from your infrastructure...")

	m = updateModel(m, redesign.StatusUpdateMsg("Loading terraform-aws-vpc...")).(redesign.WelcomeModel)

	content = stripANSI(m.View().Content)
	assert.Contains(t, content, "Loading terraform-aws-vpc...", "status text should update after StatusUpdateMsg")
	assert.NotContains(t, content,
		"Discovering components from your infrastructure...",
		"old status text should be replaced")
}

// --- Welcome No-Sources View ---

func TestWelcomeNoSourcesView_RendersHelpText(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
		return nil
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad)
	m = updateModel(m, windowSize).(redesign.WelcomeModel)

	m = updateModel(m, redesign.DiscoveryCompleteMsg{Err: nil}).(redesign.WelcomeModel)

	view := m.View()
	content := stripANSI(view.Content)

	assert.True(t, view.AltScreen, "no-sources view should use alt screen")
	assert.Contains(t, content, "Terragrunt Catalog", "should render title")
	assert.Contains(t, content, "No catalog sources were discovered", "should explain no sources found")
	assert.Contains(t, content, "catalog {", "should show catalog block example")
	assert.Contains(t, content, "urls =", "should show urls attribute in example")
	assert.Contains(t, content, "terraform.source", "should mention terraform.source as alternative")
	assert.Contains(t, content, "h: open docs in browser", "should show docs key hint")
	assert.Contains(t, content, "q/esc: exit", "should show quit key hint")
}

// --- Welcome Discovery-Error View ---

// TestWelcomeDiscoveryErrorView_RendersErrorAndHint verifies that when
// discovery finishes with an error, the welcome model switches to the
// discovery-error view and surfaces the underlying error message.
func TestWelcomeDiscoveryErrorView_RendersErrorAndHint(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	erroringLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
		return errors.New("network unreachable")
	}

	m := redesign.NewWelcomeModel(t.Context(), l, opts, erroringLoad)
	m = updateModel(m, windowSize).(redesign.WelcomeModel)

	m = updateModel(m, redesign.DiscoveryCompleteMsg{Err: errors.New("network unreachable")}).(redesign.WelcomeModel)

	view := m.View()
	content := stripANSI(view.Content)

	assert.True(t, view.AltScreen, "discovery-error view should use alt screen")
	assert.Contains(t, content, "Terragrunt Catalog", "should render title")
	assert.Contains(t, content, "An error occurred while discovering catalog sources")
	assert.Contains(t, content, "network unreachable", "should render the underlying error message")
	assert.Contains(t, content, "q/esc: exit", "should render the quit hint")
}

// --- Component List View ---

func TestComponentListView_LoadingTitle(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)
	require.NotEmpty(t, components)

	componentCh := make(chan *redesign.ComponentEntry, 10)
	m := redesign.NewModelStreaming(l, opts, components[0], componentCh, nil)

	updated, _ := m.Update(windowSize)
	m = updated.(redesign.Model)

	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "All", "tab bar should show the All tab")
	assert.Contains(t, content, "Modules", "tab bar should show the Modules tab")
	assert.Contains(t, content, "Templates", "tab bar should show the Templates tab")
	assert.Contains(t, content, "(loading...)", "streaming model should show loading indicator")

	updated, _ = m.Update(redesign.DiscoveryCompleteMsg{Err: nil})
	m = updated.(redesign.Model)

	content = stripANSI(m.View().Content)
	assert.Contains(t, content, "All", "tab bar should still show the All tab")
	assert.NotContains(t, content, "(loading...)", "loading indicator should be gone after discovery completes")
}

func TestComponentListView_MetadataRowRendered(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)
	require.NotEmpty(t, components)

	entry := components[0].WithVersion("v1.10.2").WithSource("github.com/gruntwork-io/terragrunt-scale-catalog")

	componentCh := make(chan *redesign.ComponentEntry, 10)
	m := redesign.NewModelStreaming(l, opts, entry, componentCh, nil)

	updated, _ := m.Update(windowSize)
	m = updated.(redesign.Model)

	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "module", "metadata row should contain component kind label")
	assert.Contains(t, content, "github.com/gruntwork-io/terragrunt-scale-catalog", "metadata row should contain source")
	assert.Contains(t, content, "v1.10.2", "metadata row should contain version")
}

func TestComponentListView_TemplateKindRendered(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	template := redesign.NewComponentEntry(redesign.NewComponentForTest(
		redesign.ComponentKindTemplate,
		"github.com/gruntwork-io/templates-repo",
		"templates/unit",
		"# Unit Template\nA boilerplate template.",
	)).WithSource("github.com/gruntwork-io/templates-repo")

	componentCh := make(chan *redesign.ComponentEntry, 10)
	m := redesign.NewModelStreaming(l, opts, template, componentCh, nil)

	updated, _ := m.Update(windowSize)
	m = updated.(redesign.Model)

	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "template", "template components should render with a 'template' kind pill")
}

func TestComponentListView_NoVersionOmitsVersionPill(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()
	components := makeComponents(t)
	require.NotEmpty(t, components)

	entry := components[0].WithSource("github.com/gruntwork-io/terragrunt-scale-catalog")

	componentCh := make(chan *redesign.ComponentEntry, 10)
	m := redesign.NewModelStreaming(l, opts, entry, componentCh, nil)

	updated, _ := m.Update(windowSize)
	m = updated.(redesign.Model)

	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "module", "metadata row should contain component kind label")
	assert.Contains(t, content, "github.com/gruntwork-io/terragrunt-scale-catalog", "metadata row should contain source")
	assert.NotContains(t, content, "v1.10.2", "version pill should not appear when version is empty")
}

// TestComponentListView_LongSourceAbbreviatesWithEllipsis feeds the metadata
// row a long source string at a narrow terminal width. The rendered metadata
// must contain the middle-ellipsis character and preserve both the prefix
// and suffix of the source, which drives takeWidthPrefix and takeWidthSuffix
// through abbreviateMiddle.
func TestComponentListView_LongSourceAbbreviatesWithEllipsis(t *testing.T) {
	t.Parallel()

	opts, err := options.NewTerragruntOptionsForTest("")
	require.NoError(t, err)

	l := logger.CreateLogger()

	const longSource = "github.com/gruntwork-io/terragrunt-scale-catalog-extra-long-path-that-must-be-abbreviated/subdir"

	entry := redesign.NewComponentEntry(redesign.NewComponentForTest(
		redesign.ComponentKindModule,
		longSource,
		"modules/vpc",
		"# VPC",
	)).WithSource(longSource)

	componentCh := make(chan *redesign.ComponentEntry, 1)
	m := redesign.NewModelStreaming(l, opts, entry, componentCh, nil)

	// Narrow terminal forces the source column to shrink below the raw width,
	// which forces abbreviateMiddle to truncate.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = updated.(redesign.Model)

	content := stripANSI(m.View().Content)
	assert.Contains(t, content, "…",
		"abbreviateMiddle should emit the ellipsis when the source is too wide")
	assert.Contains(t, content, "github.com",
		"prefix of the source should survive abbreviation (takeWidthPrefix)")
}

// --- synctest: Streaming Flow ---

func TestWelcomeStreamingFlow_Synctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeComponents(t)
		require.GreaterOrEqual(t, len(components), 2, "need at least 2 components")

		streamingLoad := func(
			_ context.Context,
			status redesign.StatusFunc,
			componentCh chan<- *redesign.ComponentEntry,
		) error {
			status("Discovering catalog sources...")

			for _, c := range components {
				time.Sleep(100 * time.Millisecond)

				componentCh <- c
			}

			return nil
		}

		var m tea.Model = redesign.NewWelcomeModel(t.Context(), l, opts, streamingLoad)

		m = updateModel(m, windowSize)

		_, isWelcome := m.(redesign.WelcomeModel)
		assert.True(t, isWelcome, "should start as WelcomeModel")

		content := stripANSI(m.View().Content)
		assert.Contains(t, content, "Terragrunt Catalog", "should show title while loading")

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

		time.Sleep(150 * time.Millisecond)

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

		time.Sleep(500 * time.Millisecond)

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

		listModel, isList := m.(redesign.Model)
		if isList {
			assert.Equal(t, redesign.ListState, listModel.State, "should be in list state")

			items := listModel.List().Items()
			assert.GreaterOrEqual(t, len(items), 1, "should have at least one component in the list")

			for i := 1; i < len(items); i++ {
				prev := items[i-1].(*redesign.ComponentEntry).Title()
				curr := items[i].(*redesign.ComponentEntry).Title()
				assert.LessOrEqual(t, strings.ToLower(prev), strings.ToLower(curr),
					"components should be in alphabetical order: %q should come before %q", prev, curr)
			}
		}
	})
}

// TestWelcomeStreamingFlow_LoadingIndicatorClearsAfterDiscovery_Synctest
// drives the full welcome → streaming-list transition end-to-end through the
// bubbletea command cycle and asserts that the rendered list view stops
// showing the `(loading...)` tab-bar suffix once discovery completes. It
// guards against a regression where the swap from WelcomeModel to the
// streaming Model dropped the in-flight DiscoveryCompleteMsg, leaving the
// loading indicator stuck on screen.
func TestWelcomeStreamingFlow_LoadingIndicatorClearsAfterDiscovery_Synctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeComponents(t)
		require.NotEmpty(t, components)

		streamingLoad := func(
			_ context.Context,
			_ redesign.StatusFunc,
			componentCh chan<- *redesign.ComponentEntry,
		) error {
			for _, c := range components {
				time.Sleep(50 * time.Millisecond)

				componentCh <- c
			}

			return nil
		}

		var m tea.Model = redesign.NewWelcomeModel(t.Context(), l, opts, streamingLoad)

		m = updateModel(m, windowSize)

		cmd := m.Init()

		msgCh := make(chan tea.Msg, 32)

		spawn := func(c tea.Cmd) {
			if c == nil {
				return
			}

			go func() {
				if msg := c(); msg != nil {
					msgCh <- msg
				}
			}()
		}

		spawn(cmd)

		drainOnce := func() {
			for {
				select {
				case msg := <-msgCh:
					// tea.Batch emits BatchMsg; the bubbletea runtime
					// would normally fan it out into separate cmd
					// goroutines. Replicate that here so listeners and
					// the discovery goroutine all run.
					if batch, ok := msg.(tea.BatchMsg); ok {
						for _, c := range batch {
							spawn(c)
						}

						continue
					}

					var next tea.Cmd

					m, next = m.Update(msg)

					spawn(next)
				default:
					return
				}
			}
		}

		// Repeatedly nudge time forward and drain. Each cycle gives any
		// goroutines we've spawned a chance to finish and push onto msgCh.
		for range 20 {
			time.Sleep(100 * time.Millisecond)
			drainOnce()
		}

		listModel, ok := m.(redesign.Model)
		require.True(t, ok, "should have transitioned to streaming Model after discovery")

		assert.False(t, redesign.LoadingForTest(listModel),
			"streaming Model.loading should be cleared by DiscoveryCompleteMsg")

		content := stripANSI(listModel.View().Content)
		assert.NotContains(t, content, "(loading...)",
			"loading indicator should disappear after DiscoveryCompleteMsg flows through "+
				"the welcome → streaming-list swap; got tab bar:\n%s",
			content)
	})
}

// --- synctest: Spinner Animation ---

func TestWelcomeLoadingSpinner_Synctest(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()

		slowLoad := func(ctx context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
			select {
			case <-time.After(10 * time.Second):
			case <-ctx.Done():
			}

			return nil
		}

		var m tea.Model = redesign.NewWelcomeModel(t.Context(), l, opts, slowLoad)

		m = updateModel(m, windowSize)

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

		time.Sleep(150 * time.Millisecond)

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

		view := m.View()
		content := stripANSI(view.Content)

		assert.Contains(t, content, "Terragrunt Catalog", "should still show title")
		assert.Contains(t, content, "Discovering components from your infrastructure...", "should still show status")

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
func blockingLoad(ctx context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
	<-ctx.Done()

	return nil
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
