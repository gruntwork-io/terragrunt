package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pkg/browser"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/components/buttonbar"
	"github.com/gruntwork-io/terragrunt/internal/cli/commands/scaffold"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

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
	var cmd tea.Cmd

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
		case key.Matches(msg, m.delegateKeys.Choose, m.delegateKeys.ScaffoldInteractive):
			selectedEntry, ok := m.lists[m.activeTab].SelectedItem().(*ComponentEntry)
			if !ok {
				break
			}

			selectedComponent := selectedEntry.Component

			switch {
			case key.Matches(msg, m.delegateKeys.Choose):
				tagsStyle := ResolveTagsDetailStyle()
				tags := selectedComponent.Tags()

				var (
					content string
					err     error
				)

				m, content, err = m.renderComponentContent(selectedComponent, tagsStyle, tags)
				if err != nil {
					return m, rendererErrCmd(err)
				}

				m.viewport.SetContent(content)

				pagerButtons := []button{scaffoldBtn}
				buttonNames := []string{scaffoldBtn.String()}

				if selectedComponent.URL() != "" {
					pagerButtons = append(pagerButtons, viewSourceBtn)
					buttonNames = append(buttonNames, viewSourceBtn.String())
				}

				m.currentPagerButtons = pagerButtons
				m.buttonBar = buttonbar.New(buttonNames)

				m.selectedComponent = selectedComponent
				m.State = PagerState

				return m, nil
			case key.Matches(msg, m.delegateKeys.ScaffoldInteractive):
				return enterFormState(m, selectedComponent, ListState)
			}

		case key.Matches(msg, m.listKeys.Quit):
			return m, tea.Quit
		}
	}

	m.lists[m.activeTab], cmd = m.lists[m.activeTab].Update(msg)

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
		m.buttonBar = bbModel.(*buttonbar.ButtonBar)

		if barCmd != nil {
			cmds = append(cmds, barCmd)
		}

		switch {
		case key.Matches(msg, m.pagerKeys.Choose):
			currentAction := m.activeButton

			switch currentAction {
			case scaffoldBtn:
				return enterFormState(m, m.selectedComponent, PagerState)
			case viewSourceBtn:
				if m.selectedComponent.URL() != "" {
					if err := browser.OpenURL(m.selectedComponent.URL()); err != nil {
						m.viewport.SetContent(fmt.Sprintf("could not open url in browser: %s. got error: %s", m.selectedComponent.URL(), err))
					}
				}
			default:
				m.logger.Warnf("Unknown button pressed: %s", currentAction)
			}

		case key.Matches(msg, m.pagerKeys.ScaffoldInteractive):
			return enterFormState(m, m.selectedComponent, PagerState)
		case key.Matches(msg, m.pagerKeys.ToggleWrap):
			m.softWrap = !m.softWrap
			m.mdRenderer = nil

			if m.selectedComponent != nil {
				updated, content, err := m.renderComponentContent(m.selectedComponent, ResolveTagsDetailStyle(), m.selectedComponent.Tags())
				if err != nil {
					return m, rendererErrCmd(err)
				}

				m = updated
				m.viewport.SetContent(content)
			}

			return m, nil

		case key.Matches(msg, m.pagerKeys.Quit):
			m.State = ListState
			return m, nil
		}
	case buttonbar.ActiveBtnMsg:
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
		var cmd tea.Cmd

		m, cmd = m.insertComponentSorted(msg.entry)

		return m, tea.Batch(cmd, m.listenForComponent())
	case DiscoveryCompleteMsg:
		m.loading = false

		if msg.Err != nil {
			// Components already streamed in, so the failure is partial and
			// the session stays usable. Logging here would shred the
			// alt-screen rendering (see discoverModuleFields), so surface
			// it as a notice line in the list view and stash the per-source
			// detail for the post-exit message.
			m.loadErr = msg.Err
			m.exitMessage = formatSourceFailureNotice(msg.Err, valuesBoxAccentYellow)
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

		viewportHeight := msg.Height - v - lipgloss.Height(m.footerView())

		if !m.ready {
			m.viewport = viewport.New(viewport.WithWidth(msg.Width), viewport.WithHeight(viewportHeight))
			m.ready = true
		}

		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(viewportHeight)

		// m.form is nil until a formReadyMsg arrives.
		if m.form != nil {
			m.form.SetSize(msg.Width-h, msg.Height-v)
		}

	case formReadyMsg:
		m.form = msg.form
		m.scaffoldPlan = msg.plan

		if msg.refs != nil {
			refs := *msg.refs
			m.valuesRefs = &refs
		}

		if m.width > 0 && m.height > 0 {
			h, v := AppStyle.GetFrameSize()
			m.form.SetSize(m.width-h, m.height-v)
		}

		return m, nil

	case formDiscoveryErrMsg:
		m.exitMessage = formatActionFailure("discovering variables", msg.err)
		m.terminalErr = fmt.Errorf("discovering variables: %w", msg.err)

		return m, tea.Quit

	case FormSubmitMsg:
		return m.handleFormSubmit(msg.Values)

	case FormCancelMsg:
		m.abandonForm()

		m.State = m.priorState

		return m, nil

	case ScaffoldFinishedMsg:
		if msg.Err != nil {
			m.exitMessage = formatActionFailure("scaffolding component", msg.Err)
			m.terminalErr = fmt.Errorf("scaffolding component: %w", msg.Err)

			return m, tea.Quit
		}

		m.exitMessage = formatScaffoldMessage(m.terragruntOptions, msg.Interactive)

		return m, tea.Quit

	case CopyFinishedMsg:
		if msg.Err != nil {
			m.exitMessage = formatActionFailure("copying component", msg.Err)
			m.terminalErr = fmt.Errorf("copying component: %w", msg.Err)

			return m, tea.Quit
		}

		m.exitMessage = formatCopyValuesMessage(msg.Result, msg.Interactive)

		return m, tea.Quit

	case RendererErrMsg:
		m.viewport.SetContent("there was an error rendering markdown: " + msg.Err.Error())
		m.State = PagerState
	}

	switch m.State {
	case ListState:
		return updateList(msg, m)
	case PagerState:
		return updatePager(msg, m)
	case FormState:
		return updateForm(msg, m)
	case ScaffoldState:
		// Discard further input while the scaffold subprocess is running;
		// the model resumes on ScaffoldFinishedMsg or CopyFinishedMsg.
		return m, nil
	}

	return m, nil
}

// renderComponentContent prepares the pager body, prepending tag pills
// when configured. Markdown components run through the model's cached
// glamour renderer, which may itself be (re)allocated.
func (m Model) renderComponentContent(c *Component, tagsStyle TagsDetailStyle, tags []string) (Model, string, error) {
	if !c.IsMarkDown() {
		content := c.Content(true)
		if pills := RenderDetailTagPills(tags); pills != "" {
			content = pills + "\n\n" + content
		}

		return m, content, nil
	}

	m, renderer, err := m.markdownRenderer()
	if err != nil {
		return m, "", err
	}

	body := c.Content(false)
	if tagsStyle == TagsDetailStyleSection {
		body += TagsMarkdownSection(tags)
	}

	md, err := renderer.Render(body)
	if err != nil {
		return m, "", err
	}

	if tagsStyle == TagsDetailStylePills {
		if pills := RenderDetailTagPills(tags); pills != "" {
			md = lipgloss.NewStyle().PaddingLeft(glamourDocumentMargin).Render(pills) + "\n\n" + md
		}
	}

	return m, md, nil
}

// RendererErrMsg signals that the glamour markdown renderer failed to
// build or render a component's content.
type RendererErrMsg struct{ Err error }

func rendererErrCmd(err error) tea.Cmd {
	return func() tea.Msg {
		return RendererErrMsg{Err: err}
	}
}

// ScaffoldFinishedMsg is delivered when a scaffold subprocess completes.
// Interactive distinguishes the form-submit path from the placeholder path.
type ScaffoldFinishedMsg struct {
	Err         error
	Interactive bool
}

// CopyFinishedMsg is delivered when a copy subprocess completes.
// Interactive distinguishes the form-submit path from the placeholder path.
type CopyFinishedMsg struct {
	Err         error
	Result      CopyResult
	Interactive bool
}

const (
	valuesBoxAccentGreen  = "#50FA7B"
	valuesBoxAccentYellow = "#F1FA8C"
	valuesBoxAccentRed    = "#FF5555"
	valuesBoxPathColor    = "#8BE9FD"
	valuesBoxMutedColor   = "#A8ACB1"
)

// formatActionFailure renders a bordered callout describing a failed
// scaffold or copy action. action is a verb phrase ("scaffolding component",
// "copying component"). The message is stashed on the model so Run
// can print it after the alt screen is restored; tea.Printf lines emitted
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

// formatSourceFailureNotice renders the post-exit callout for source-load
// failures: a summary heading plus one line per failed source. The in-TUI
// surfaces (the list-view notice line, the welcome error screen) live in
// the alt screen and vanish on exit, so the detail lands in the user's
// scrollback here. accent picks the border color: yellow for partial
// failures that left the session usable, red for failures that ended it.
func formatSourceFailureNotice(err error, accent string) string {
	heading := lipgloss.NewStyle().
		Foreground(lipgloss.Color(accent)).
		Bold(true).
		Render(err.Error())

	rows := []string{heading}

	if srcErr, ok := errors.AsType[*SourceLoadError](err); ok {
		for _, f := range srcErr.Failures {
			rows = append(rows,
				"",
				valuesBoxPathStyle.Render(f.URL),
				valuesBoxMuteStyle.Render(f.Err.Error()),
			)
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, rows...)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(accent)).
		Padding(bodyPaddingVertical, bodyPaddingHorizontal).
		Render(content)
}

// formatCopyValuesMessage returns a lipgloss-bordered callout summarizing
// what happened with the values stub, or "" when there's nothing to say.
// The caller prints this to stderr after the TUI has torn down its alt
// screen, so the box lands in the user's scrollback. interactive controls
// the body copy: the form path tells the user which lines they still need
// to revisit, while the placeholder path describes the full TODO flow.
func formatCopyValuesMessage(r CopyResult, interactive bool) string {
	if r.References.IsEmpty() {
		return ""
	}

	path := displayPath(r.WorkingDir, filepath.Join(r.WorkingDir, valuesFileName))

	switch {
	case r.ValuesWritten:
		heading := lipgloss.NewStyle().
			Foreground(lipgloss.Color(valuesBoxAccentGreen)).
			Bold(true).
			Render("terragrunt.values.hcl generated")

		summary := fmt.Sprintf("%d required %s, %d optional %s",
			len(r.References.Required), pluralize("entry", "entries", len(r.References.Required)),
			len(r.References.Optional), pluralize("default", "defaults", len(r.References.Optional)))

		body := "Open the file and replace each \"TODO\" with a real value.\n" +
			"Optional defaults are pre-populated; edit or delete lines as needed\n" +
			"before running terragrunt."

		if interactive {
			body = "Values you filled in the form are populated.\n" +
				"Any line still set to \"TODO\" (or any default you want to override)\n" +
				"needs to be edited before running terragrunt."
		}

		return renderValuesBox(valuesBoxAccentGreen, heading, path, summary, body)

	case r.ValuesSkipped:
		heading := lipgloss.NewStyle().
			Foreground(lipgloss.Color(valuesBoxAccentYellow)).
			Bold(true).
			Render("terragrunt.values.hcl left untouched")

		summary := "Referenced values.* keys: " + strings.Join(r.References.allNames(), ", ")

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
// scaffold run, pointing the user at the generated terragrunt.hcl. When
// interactive is true the user came through the form so any unfilled
// fields landed as `# TODO` lines; otherwise every input is a TODO.
func formatScaffoldMessage(opts *options.TerragruntOptions, interactive bool) string {
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

	if interactive {
		summary = "Values you filled in the form are populated; unfilled inputs are " +
			"marked with `# TODO: fill in value`."

		body = "Open the file and replace any remaining TODO placeholder with a real\n" +
			"value before running terragrunt."
	}

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

// formReadyMsg is delivered once discovery has built a populated FormModel
// and (for module/template) the prepared scaffold.Plan, or (for unit/stack)
// the captured ValuesReferences.
type formReadyMsg struct {
	form *FormModel
	plan *scaffold.Plan
	refs *ValuesReferences
}

// formDiscoveryErrMsg signals that the pre-form discovery step failed
// (download error, HCL parse error, etc.). The outer Update treats this
// like a scaffold or copy failure: stash a styled message and quit.
type formDiscoveryErrMsg struct{ err error }

// enterFormState transitions the model into FormState and fires the
// discovery command appropriate for the component's kind. priorState is
// recorded so a cancel returns the user to wherever they invoked the
// scaffold from.
func enterFormState(m Model, c *Component, priorState sessionState) (tea.Model, tea.Cmd) {
	m.priorState = priorState
	m.State = FormState
	m.form = nil
	m.scaffoldPlan = nil
	m.valuesRefs = nil
	m.selectedComponent = c

	return m, discoverFormCmd(m.ctx, m.logger, m.terragruntOptions, c)
}

// discoverFormCmd runs the kind-appropriate variable discovery off the UI
// thread. For module/template that means downloading the source and
// parsing variables via scaffold.Prepare; for unit/stack it means reading
// the source HCL and walking it via CollectValuesReferences. ctx is the
// model's cancellable context so a Ctrl+C during discovery aborts the
// download instead of running it to completion.
func discoverFormCmd(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, c *Component) tea.Cmd {
	return func() tea.Msg {
		if c.Kind.IsCopyable() {
			return discoverValuesFields(c)
		}

		return discoverModuleFields(ctx, l, opts, c)
	}
}

// discoverModuleFields prepares the scaffold for a module/template and
// returns either a formReadyMsg (carrying both the form and the prepared
// plan) or a formDiscoveryErrMsg.
//
// The Prepare call runs inside a bubbletea Cmd, which means the alt-screen
// is still active. Any log writes here go straight to the terminal stderr
// and shred the form's rendering, so we feed Prepare a logger whose output
// is discarded. Real failures still surface: they come back as errors and
// turn into a formDiscoveryErrMsg, which the outer Update renders as a
// styled exit message after tea tears down.
func discoverModuleFields(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, c *Component) tea.Msg {
	quiet := l.WithOptions(log.WithOutput(io.Discard))

	plan, err := scaffold.Prepare(ctx, quiet, venv.OSVenv(), opts, c.TerraformSourcePath(), "")
	if err != nil {
		return formDiscoveryErrMsg{err: err}
	}

	fields := FieldsFromParsedVariables(plan.Required, plan.Optional)

	return formReadyMsg{
		form: NewFormModel(c, fields),
		plan: plan,
	}
}

// discoverValuesFields walks the unit/stack's HCL for `values.*` refs and
// returns a formReadyMsg. CollectValuesReferences operates on the already
// cloned local copy, so there's no download.
func discoverValuesFields(c *Component) tea.Msg {
	configName := configFileForKind(c.Kind)
	if configName == "" {
		return formDiscoveryErrMsg{err: fmt.Errorf("component kind %q has no associated HCL file", c.Kind)}
	}

	refs, err := CollectValuesReferences(vfs.NewOSFS(), filepath.Join(c.Repo.Path(), c.Dir, configName))
	if err != nil {
		return formDiscoveryErrMsg{err: err}
	}

	fields := FieldsFromValuesReferences(refs)

	return formReadyMsg{
		form: NewFormModel(c, fields),
		refs: &refs,
	}
}

// updateForm routes messages while the form is on screen. It delegates
// keypresses (and any other input) to the embedded FormModel, which may
// in turn emit FormSubmitMsg or FormCancelMsg for the outer Update.
func updateForm(msg tea.Msg, m Model) (tea.Model, tea.Cmd) {
	if m.form == nil {
		// Discovery is still in flight. Swallow input until the
		// formReadyMsg arrives so the user can't kick off a second action.
		return m, nil
	}

	updated, cmd := m.form.Update(msg)
	m.form = updated

	return m, cmd
}

// handleFormSubmit transitions the model from FormState to ScaffoldState
// and fires the kind-appropriate execution command, carrying the user's
// raw HCL fragments.
func (m Model) handleFormSubmit(values map[string]string) (tea.Model, tea.Cmd) {
	m.State = ScaffoldState

	c := m.selectedComponent

	if c.Kind.IsCopyable() {
		return m, copyComponentWithValuesCmd(m.logger, m, c, values)
	}

	plan := m.scaffoldPlan
	m.scaffoldPlan = nil

	return m, scaffoldComponentWithPlanCmd(m.logger, m, c, plan, values)
}

// abandonForm releases any prepared scaffold.Plan and clears the form
// pointer. Called when the user esc-cancels the form so we don't leak
// the temp dir Prepare allocated.
func (m *Model) abandonForm() {
	if m.scaffoldPlan != nil {
		m.scaffoldPlan.Cleanup()
		m.scaffoldPlan = nil
	}

	m.form = nil
	m.valuesRefs = nil
}

// scaffoldComponentWithPlanCmd schedules the prepared plan's Generate
// call, threading the user-supplied HCL values. tea.Exec is used so the
// generation (which formats HCL and writes files) runs outside the
// bubbletea event loop.
func scaffoldComponentWithPlanCmd(l log.Logger, m Model, c *Component, plan *scaffold.Plan, values map[string]string) tea.Cmd {
	cmd := newScaffoldCmd(l, m.terragruntOptions, c).WithPlan(plan, values)

	return tea.Exec(cmd, func(err error) tea.Msg {
		return ScaffoldFinishedMsg{Err: err, Interactive: true}
	})
}

// copyComponentWithValuesCmd schedules the unit/stack copy with the form's
// collected HCL values threaded through CopyCmd.WithValues.
func copyComponentWithValuesCmd(l log.Logger, m Model, c *Component, values map[string]string) tea.Cmd {
	cmd := NewCopyCmd(l, m.terragruntOptions, c).WithValues(values)

	return tea.Exec(cmd, func(err error) tea.Msg {
		return CopyFinishedMsg{Err: err, Result: cmd.Result(), Interactive: true}
	})
}
