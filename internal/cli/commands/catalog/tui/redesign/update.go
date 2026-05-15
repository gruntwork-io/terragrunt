package redesign

import (
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/pkg/browser"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Tab key bindings for cycling between the All/Modules/Templates tabs.
// These are only active outside the list's filter input mode.
var (
	tabNextKey = key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next tab"),
	)
	tabPrevKey = key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "prev tab"),
	)
)

func updateList(msg tea.Msg, m Model) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		m.userNavigated = true

		// Don't match any of the keys below if we're actively filtering.
		if m.lists[m.activeTab].FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, tabNextKey):
			m.activeTab = m.activeTab.next()

			return m, nil
		case key.Matches(msg, tabPrevKey):
			m.activeTab = m.activeTab.prev()

			return m, nil
		case key.Matches(msg, m.delegateKeys.Choose, m.delegateKeys.Scaffold):
			if selectedEntry, ok := m.lists[m.activeTab].SelectedItem().(*ComponentEntry); ok {
				selectedComponent := selectedEntry.Component

				switch {
				case key.Matches(msg, m.delegateKeys.Choose):
					// prepare the viewport
					var content string

					tagsStyle := resolveTagsDetailStyle(m.venv.Env)
					tags := selectedComponent.Tags()

					if selectedComponent.IsMarkDown() {
						var (
							renderer *glamour.TermRenderer
							err      error
						)

						m, renderer, err = m.markdownRenderer()
						if err != nil {
							return m, rendererErrCmd(err)
						}

						body := selectedComponent.Content(false)
						if tagsStyle == tagsDetailStyleSection {
							body += tagsMarkdownSection(tags)
						}

						md, err := renderer.Render(body)
						if err != nil {
							return m, rendererErrCmd(err)
						}

						if tagsStyle == tagsDetailStylePills {
							if pills := renderDetailTagPills(tags); pills != "" {
								md = lipgloss.NewStyle().PaddingLeft(glamourDocumentMargin).Render(pills) + "\n\n" + md
							}
						}

						content = md
					} else {
						content = selectedComponent.Content(true)

						if pills := renderDetailTagPills(tags); pills != "" {
							content = pills + "\n\n" + content
						}
					}

					m.viewport.SetContent(content)

					// Build the button bar. The primary button is always
					// labeled "Scaffold" regardless of kind; units and
					// stacks dispatch to the copy action under the hood.
					var pagerButtons []button

					buttonNames := []string{}

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

					return m, primaryActionCmd(m.logger, m, selectedComponent)
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
	m.lists[m.activeTab], cmd = m.lists[m.activeTab].Update(msg)

	// Append any commands from button bar initialization
	if len(cmds) > 0 {
		return m, tea.Batch(append([]tea.Cmd{cmd}, cmds...)...)
	}

	return m, cmd
}

func updatePager(msg tea.Msg, m Model) (tea.Model, tea.Cmd) {
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

				return m, primaryActionCmd(m.logger, m, m.selectedComponent)
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

			return m, primaryActionCmd(m.logger, m, m.selectedComponent)

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
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case tea.BackgroundColorMsg:
		dark := msg.IsDark()
		if dark != m.hasDarkBG {
			m.mdRenderer = nil
		}

		m.hasDarkBG = dark

	case tea.WindowSizeMsg:
		h, v := AppStyle.GetFrameSize()

		// Reserve one line for the tab bar plus a blank spacer line.
		const tabBarHeight = 2
		for i := range int(numTabs) {
			m.lists[i].SetSize(msg.Width-h, msg.Height-v-tabBarHeight)
		}

		if msg.Width != m.width {
			m.mdRenderer = nil
		}

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
			// tea.Printf during alt-screen gets discarded on teardown, so
			// stash the failure on the model and let RunRedesign emit it
			// to the user's scrollback after exit.
			m.exitMessage = formatActionFailure("scaffolding component", msg.err)

			return m, tea.Quit
		}

		// Same post-exit-message pattern as the copy flow: stash a styled
		// callout and let RunRedesign print it after the alt screen is
		// torn down, so it survives into the user's scrollback.
		m.exitMessage = formatScaffoldMessage(m.terragruntOptions)

		return m, tea.Quit

	case copyFinishedMsg:
		if msg.err != nil {
			m.exitMessage = formatActionFailure("copying component", msg.err)

			return m, tea.Quit
		}

		// Stash a styled post-exit message on the model so RunRedesign
		// can emit it to stderr after the alt screen is torn down.
		// tea.Printf lines emitted during alt-screen get discarded when
		// the alt buffer is restored on exit.
		m.exitMessage = formatCopyValuesMessage(msg.result)

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

type copyFinishedMsg struct {
	err    error
	result copyResult
}

// scaffoldComponentCmd returns a tea.Cmd that scaffolds the given component
// via the redesign-owned scaffold command. It does not require the legacy
// catalog.CatalogService.
func scaffoldComponentCmd(l log.Logger, m Model, c *Component) tea.Cmd {
	return tea.Exec(newScaffoldCmd(l, m.venv, m.terragruntOptions, c), func(err error) tea.Msg {
		return scaffoldFinishedMsg{err}
	})
}

// copyComponentCmd returns a tea.Cmd that copies the given component's
// directory tree into the user's working directory.
func copyComponentCmd(l log.Logger, m Model, c *Component) tea.Cmd {
	cmd := NewCopyCmd(l, m.terragruntOptions, c)

	return tea.Exec(cmd, func(err error) tea.Msg {
		return copyFinishedMsg{err: err, result: cmd.Result()}
	})
}

// Styling for the post-exit values-stub callout.
const (
	valuesBoxAccentGreen  = "#50FA7B"
	valuesBoxAccentYellow = "#F1FA8C"
	valuesBoxAccentRed    = "#FF5555"
	valuesBoxPathColor    = "#8BE9FD"
	valuesBoxMutedColor   = "#A8ACB1"
)

// formatActionFailure renders a bordered callout describing a failed
// scaffold or copy action. action is a verb phrase ("scaffolding component",
// "copying component"). The message is stashed on the model so RunRedesign
// can print it after the alt screen is restored — tea.Printf lines emitted
// during alt-screen are discarded on exit.
func formatActionFailure(action string, err error) string {
	heading := lipgloss.NewStyle().
		Foreground(lipgloss.Color(valuesBoxAccentRed)).
		Bold(true).
		Render("error " + action)

	body := err.Error()

	content := lipgloss.JoinVertical(lipgloss.Left, heading, "", body)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(valuesBoxAccentRed)).
		Padding(bodyPaddingVertical, bodyPaddingHorizontal).
		Render(content)
}

var (
	valuesBoxPathStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(valuesBoxPathColor))
	valuesBoxMuteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(valuesBoxMutedColor))
)

// formatCopyValuesMessage returns a lipgloss-bordered callout summarizing
// what happened with the values stub, or "" when there's nothing to say.
// The caller prints this to stderr after the TUI has torn down its alt
// screen, so the box lands in the user's scrollback.
func formatCopyValuesMessage(r copyResult) string {
	if r.references.IsEmpty() {
		return ""
	}

	path := displayPath(r.workingDir, filepath.Join(r.workingDir, valuesFileName))

	switch {
	case r.valuesWritten:
		heading := lipgloss.NewStyle().
			Foreground(lipgloss.Color(valuesBoxAccentGreen)).
			Bold(true).
			Render("terragrunt.values.hcl generated")

		summary := fmt.Sprintf("%d required TODO %s, %d optional %s",
			len(r.references.Required), pluralize("entry", "entries", len(r.references.Required)),
			len(r.references.Optional), pluralize("default", "defaults", len(r.references.Optional)))

		body := "Open the file and replace each \"TODO\" with a real value.\n" +
			"Optional defaults are pre-populated; edit or delete lines as needed\n" +
			"before running terragrunt."

		return renderValuesBox(valuesBoxAccentGreen, heading, path, summary, body)

	case r.valuesSkipped:
		heading := lipgloss.NewStyle().
			Foreground(lipgloss.Color(valuesBoxAccentYellow)).
			Bold(true).
			Render("terragrunt.values.hcl left untouched")

		summary := "Referenced values.* keys: " + strings.Join(r.references.allNames(), ", ")

		body := "An existing file was found at the destination, so no stub was written.\n" +
			"Make sure each referenced key above has a real value before running terragrunt."

		return renderValuesBox(valuesBoxAccentYellow, heading, path, summary, body)
	}

	return ""
}

// renderValuesBox composes the four-section bordered callout used by both
// the "written" and "skipped" exit messages.
func renderValuesBox(accent, heading, path, summary, body string) string {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		heading,
		"",
		valuesBoxPathStyle.Render(path),
		"",
		valuesBoxMuteStyle.Render(summary),
		"",
		body,
	)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(accent)).
		Padding(bodyPaddingVertical, bodyPaddingHorizontal).
		Render(content)
}

// formatScaffoldMessage returns the post-exit callout for a successful
// scaffold run, pointing the user at the generated terragrunt.hcl and the
// `# TODO: fill in value` markers the scaffold template leaves behind.
func formatScaffoldMessage(opts *options.TerragruntOptions) string {
	outputDir := opts.ScaffoldOutputFolder
	if outputDir == "" {
		outputDir = opts.WorkingDir
	}

	if outputDir == "" {
		return ""
	}

	absPath := filepath.Join(outputDir, config.DefaultTerragruntConfigPath)
	path := displayPath(outputDir, absPath)

	heading := lipgloss.NewStyle().
		Foreground(lipgloss.Color(valuesBoxAccentGreen)).
		Bold(true).
		Render("terragrunt.hcl scaffolded")

	summary := "Inputs are marked with `# TODO: fill in value` comments."

	body := "Open the file and replace each TODO placeholder with a real value\n" +
		"before running terragrunt."

	return renderValuesBox(valuesBoxAccentGreen, heading, path, summary, body)
}

// pluralize returns singular when n == 1 and plural otherwise.
func pluralize(singular, plural string, n int) string {
	if n == 1 {
		return singular
	}

	return plural
}

// displayPath returns abs rewritten relative to baseDir when that yields a
// cleaner string. Falls back to abs when baseDir is empty, when Rel returns
// an error, or when the relative form would escape baseDir (e.g., a long
// ../../../ chain). baseDir should be the terragrunt working directory
// (typically opts.WorkingDir), not the process's os.Getwd(), so that
// --working-dir is honored.
func displayPath(baseDir, abs string) string {
	if baseDir == "" {
		return abs
	}

	rel, err := filepath.Rel(baseDir, abs)
	if err != nil {
		return abs
	}

	// If the relative form escapes baseDir, the absolute path is easier to read.
	// filepath.Rel emits an OS-appropriate separator after the leading `..`,
	// so checking for the prefix is platform-agnostic.
	parentPrefix := ".." + string(filepath.Separator)
	if rel == ".." || strings.HasPrefix(rel, parentPrefix) {
		return abs
	}

	// Prefix with `.` + the OS separator (`./` on POSIX, `.\` on Windows) so
	// the string reads obviously as a relative path. filepath.Join/Clean both
	// strip this prefix, so we compose it explicitly.
	return "." + string(filepath.Separator) + rel
}

// primaryActionCmd dispatches to scaffold or copy based on the component's
// kind. Units and stacks copy; modules and templates scaffold.
func primaryActionCmd(l log.Logger, m Model, c *Component) tea.Cmd {
	if c.Kind.IsCopyable() {
		return copyComponentCmd(l, m, c)
	}

	return scaffoldComponentCmd(l, m, c)
}
