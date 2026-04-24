package redesign_test

import (
	"bytes"
	"context"
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

// unrelatedModel is a tea.Model that is not a redesign.Model, used to verify
// EmitExitMessage is a no-op for unrelated model types.
type unrelatedModel struct{}

func (unrelatedModel) Init() tea.Cmd                           { return nil }
func (m unrelatedModel) Update(_ tea.Msg) (tea.Model, tea.Cmd) { return m, nil }
func (unrelatedModel) View() tea.View                          { return tea.NewView("") }

func TestEmitExitMessage_WritesMessageToErrWriter(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	m := redesign.NewModelWithExitMessageForTest("values stub written to foo")

	redesign.EmitExitMessage(m, &buf, logger.CreateLogger())

	assert.Equal(t, "values stub written to foo\n", buf.String())
}

func TestEmitExitMessage_NoMessageWritesNothing(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	redesign.EmitExitMessage(redesign.NewModelWithExitMessageForTest(""), &buf, logger.CreateLogger())

	assert.Empty(t, buf.String())
}

func TestEmitExitMessage_UnrelatedModelIsNoop(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	redesign.EmitExitMessage(unrelatedModel{}, &buf, logger.CreateLogger())

	assert.Empty(t, buf.String())
}

// TestEmitExitMessage_WriteFailureIsLogged verifies that a writer error is
// logged rather than propagated or causing a panic.
func TestEmitExitMessage_WriteFailureIsLogged(t *testing.T) {
	t.Parallel()

	m := redesign.NewModelWithExitMessageForTest("anything")

	assert.NotPanics(t, func() {
		redesign.EmitExitMessage(m, failingWriter{}, logger.CreateLogger())
	})
}

// TestWelcomeLoadingScreen_NoSources verifies that when discovery finds no
// component sources, the welcome model stays on the no-sources help screen.
func TestWelcomeLoadingScreen_NoSources(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()

		noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
			return nil
		}

		m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad)

		msgs := []tea.Msg{tea.KeyPressMsg{Code: 'q', Text: "q"}}

		finalModel := driveModel(t, m, 120, 40, msgs)

		_, isWelcome := finalModel.(redesign.WelcomeModel)
		assert.True(t, isWelcome, "should remain on welcome screen when no sources found")
	})
}

// TestWelcomeLoadingScreen_TransitionsToComponentList verifies that when
// discovery finds components, the welcome model transitions to the full
// component list TUI.
func TestWelcomeLoadingScreen_TransitionsToComponentList(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
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

		msgs := []tea.Msg{tea.KeyPressMsg{Code: 'q', Text: "q"}}

		finalModel := driveModel(t, m, 120, 40, msgs)

		listModel, isList := finalModel.(redesign.Model)
		require.True(t, isList, "should transition to component list when components found")
		assert.Equal(t, redesign.ListState, listModel.State)
		assert.Len(t, listModel.List().Items(), len(components), "should have all streamed components in list")
	})
}

// TestWelcomeLoadingScreen_ComponentListNavigation verifies the full flow:
// loading → component list → select component details → back to list → quit.
func TestWelcomeLoadingScreen_ComponentListNavigation(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
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

		msgs := []tea.Msg{
			tea.KeyPressMsg{Code: tea.KeyEnter},
			tea.KeyPressMsg{Code: 'q', Text: "q"},
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs)

		listModel, isList := finalModel.(redesign.Model)
		require.True(t, isList, "should be on component list after navigating back")
		assert.Equal(t, redesign.ListState, listModel.State)
	})
}

// TestWelcomeLoadingScreen_QuitDuringLoading verifies that pressing q
// during the loading phase exits cleanly.
func TestWelcomeLoadingScreen_QuitDuringLoading(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
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

		msgs := []tea.Msg{tea.KeyPressMsg{Code: 'q', Text: "q"}}

		finalModel := driveModel(t, m, 120, 40, msgs)

		_, isWelcome := finalModel.(redesign.WelcomeModel)
		assert.True(t, isWelcome, "should exit as WelcomeModel when quit during loading")
	})
}

// TestWelcomeNoSourcesScreen_HelpKeyOpensDocs verifies that pressing h on
// the no-sources screen triggers the open-URL function with the docs URL.
func TestWelcomeNoSourcesScreen_HelpKeyOpensDocs(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
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

		msgs := []tea.Msg{
			tea.KeyPressMsg{Code: 'h', Text: "h"},
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs)

		_, isWelcome := finalModel.(redesign.WelcomeModel)
		assert.True(t, isWelcome, "should remain on welcome screen after pressing h")
		assert.Equal(t, "https://docs.terragrunt.com/features/catalog/", openedURL, "should have opened docs URL")
	})
}

// TestWelcomeNoSourcesScreen_UnhandledKey verifies that pressing an
// unrecognized key on the no-sources screen does not crash or change state.
func TestWelcomeNoSourcesScreen_UnhandledKey(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()

		noSourcesLoad := func(_ context.Context, _ redesign.StatusFunc, _ chan<- *redesign.ComponentEntry) error {
			return nil
		}

		m := redesign.NewWelcomeModel(t.Context(), l, opts, noSourcesLoad)

		msgs := []tea.Msg{
			tea.KeyPressMsg{Code: 'x', Text: "x"},
			tea.KeyPressMsg{Code: 'q', Text: "q"},
		}

		finalModel := driveModel(t, m, 120, 40, msgs)

		_, isWelcome := finalModel.(redesign.WelcomeModel)
		assert.True(t, isWelcome, "should remain on welcome screen after pressing unhandled key")
	})
}

// TestWelcomeStreamingComponents verifies that components stream into the list
// one at a time, ending up in sorted order.
func TestWelcomeStreamingComponents(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		opts, err := options.NewTerragruntOptionsForTest("")
		require.NoError(t, err)

		l := logger.CreateLogger()
		components := makeComponents(t)
		require.GreaterOrEqual(t, len(components), 2, "need at least 2 components for streaming test")

		streamingLoad := func(_ context.Context, _ redesign.StatusFunc, componentCh chan<- *redesign.ComponentEntry) error {
			for _, c := range components {
				componentCh <- c
			}

			return nil
		}

		m := redesign.NewWelcomeModel(t.Context(), l, opts, streamingLoad)

		msgs := []tea.Msg{tea.KeyPressMsg{Code: 'q', Text: "q"}}

		finalModel := driveModel(t, m, 120, 40, msgs)

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
	})
}
