package redesign

import (
	"fmt"
	"os"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/pkg/browser"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func updateList(msg tea.Msg, m Model) (tea.Model, tea.Cmd) { //nolint:gocritic
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.userNavigated = true

		// Don't match any of the keys below if we're actively filtering.
		if m.List.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, m.delegateKeys.Choose, m.delegateKeys.Scaffold):
			if selectedEntry, ok := m.List.SelectedItem().(*ComponentEntry); ok {
				selectedComponent := selectedEntry.Component

				switch {
				case key.Matches(msg, m.delegateKeys.Choose):
					// prepare the viewport
					var content string

					if selectedComponent.IsMarkDown() {
						style := "dark"
						if !lipgloss.HasDarkBackground(os.Stdin, os.Stdout) {
							style = "light"
						}

						renderer, err := glamour.NewTermRenderer(
							glamour.WithStandardStyle(style),
							glamour.WithWordWrap(m.width),
						)
						if err != nil {
							return m, rendererErrCmd(err)
						}

						md, err := renderer.Render(selectedComponent.Content(false))
						if err != nil {
							return m, rendererErrCmd(err)
						}

						content = md
					} else {
						content = selectedComponent.Content(true)
					}

					m.viewport.SetContent(content)

					// Dynamically create button bar based on component URL
					var pagerButtons []button

					buttonNames := []string{}

					// Always add scaffold button
					pagerButtons = append(pagerButtons, scaffoldBtn)
					buttonNames = append(buttonNames, scaffoldBtn.String())

					if selectedComponent.URL() != "" {
						pagerButtons = append(pagerButtons, viewSourceBtn)
						buttonNames = append(buttonNames, viewSourceBtn.String())
					}

					m.currentPagerButtons = pagerButtons
					m.buttonBar = buttonbar.New(buttonNames)
					// Ensure the button bar is initialized
					cmds = append(cmds, m.buttonBar.Init())

					// advance state
					m.selectedComponent = selectedComponent
					m.State = PagerState

					return m, tea.Batch(cmds...)
				case key.Matches(msg, m.delegateKeys.Scaffold):
					m.State = ScaffoldState

					return m, scaffoldComponentCmd(m.logger, m, selectedComponent)
				}
			} else {
				break
			}

		case key.Matches(msg, m.listKeys.Quit):
			// because we're on the first screen, we simply quit at this point
			return m, tea.Quit
		}
	}

	// Handle keyboard and mouse events for the list
	m.List, cmd = m.List.Update(msg)

	// Append any commands from button bar initialization
	if len(cmds) > 0 {
		return m, tea.Batch(append([]tea.Cmd{cmd}, cmds...)...)
	}

	return m, cmd
}

func updatePager(msg tea.Msg, m Model) (tea.Model, tea.Cmd) { //nolint:gocritic
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		bbModel, barCmd := m.buttonBar.Update(msg)
		if newButtonBar, ok := bbModel.(*buttonbar.ButtonBar); ok {
			m.buttonBar = newButtonBar
		}

		if barCmd != nil {
			cmds = append(cmds, barCmd)
		}

		switch {
		case key.Matches(msg, m.pagerKeys.Choose):
			// Choose changes the action depending on the active button
			// m.activeButton is set by ActiveBtnMsg, which is mapped from m.currentPagerButtons
			currentAction := m.activeButton

			switch currentAction {
			case scaffoldBtn:
				m.State = ScaffoldState

				return m, scaffoldComponentCmd(m.logger, m, m.selectedComponent)
			case viewSourceBtn:
				if m.selectedComponent.URL() != "" {
					if err := browser.OpenURL(m.selectedComponent.URL()); err != nil {
						m.viewport.SetContent(fmt.Sprintf("could not open url in browser: %s. got error: %s", m.selectedComponent.URL(), err))
					}
				}
			default:
				m.logger.Warnf("Unknown button pressed: %s", currentAction)
			}

		case key.Matches(msg, m.pagerKeys.Scaffold):
			m.State = ScaffoldState

			return m, scaffoldComponentCmd(m.logger, m, m.selectedComponent)

		case key.Matches(msg, m.pagerKeys.Quit):
			// because we're on the second screen, we need to go back
			m.State = ListState
			return m, nil
		}
	case buttonbar.ActiveBtnMsg:
		// Map the index from buttonbar.ActiveBtnMsg to the actual button type
		if int(msg) >= 0 && int(msg) < len(m.currentPagerButtons) {
			m.activeButton = m.currentPagerButtons[int(msg)]
		}
	}

	// Handle keyboard and mouse events in the viewport
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// Update handles all TUI interactions and implements bubbletea.Model.Update.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocritic
	switch msg := msg.(type) {
	case componentMsg:
		cmd := m.insertComponentSorted(msg.entry)

		return m, tea.Batch(cmd, m.listenForComponent())
	case DiscoveryCompleteMsg:
		m.loading = false

		if msg.Err != nil {
			m.logger.Warnf("Discovery error: %v", msg.Err)
		}

		return m, nil
	case tea.WindowSizeMsg:
		h, v := AppStyle.GetFrameSize()
		m.List.SetSize(msg.Width-h, msg.Height-v)
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			// Since this program is using the full size of the viewport we
			// need to wait until we've received the window dimensions before
			// we can initialize the viewport. The initial dimensions come in
			// quickly, though asynchronously, which is why we wait for them
			// here.
			m.viewport = viewport.New(viewport.WithWidth(msg.Width), viewport.WithHeight(msg.Height-v-lipgloss.Height(m.footerView())))
			m.ready = true
		} else {
			m.viewport.SetWidth(msg.Width)
			m.viewport.SetHeight(msg.Height - v - lipgloss.Height(m.footerView()))
		}

	case scaffoldFinishedMsg:
		if msg.err != nil {
			return m, tea.Batch(tea.Printf("error scaffolding component: %s", msg.err.Error()), tea.Quit)
		}

		return m, tea.Quit

	case rendererErrMsg:
		m.viewport.SetContent("there was an error rendering markdown: " + msg.err.Error())
		// ensure we show the viewport
		m.State = PagerState
	}

	// Hand off the message and model to the appropriate update function for the
	// appropriate view based on the current state.
	switch m.State {
	case ListState:
		return updateList(msg, m)
	case PagerState:
		return updatePager(msg, m)
	case ScaffoldState:
		// if we're on the scaffold state, we do nothing and wait for the
		// scaffoldFinishedMsg message. This prevents further input.
		return m, nil
	}

	return m, nil
}

type rendererErrMsg struct{ err error }

func rendererErrCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return rendererErrMsg{err}
	}
}

type scaffoldFinishedMsg struct{ err error }

// scaffoldComponentCmd returns a tea.Cmd that scaffolds the given component
// via the redesign-owned scaffold command. It does not require the legacy
// catalog.CatalogService.
func scaffoldComponentCmd(l log.Logger, m Model, c *Component) tea.Cmd { //nolint:gocritic
	return tea.Exec(newScaffoldCmd(l, m.terragruntOptions, c), func(err error) tea.Msg {
		return scaffoldFinishedMsg{err}
	})
}
