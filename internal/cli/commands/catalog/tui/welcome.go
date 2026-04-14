package tui

import (
	"context"
	"errors"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pkg/browser"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const welcomeDocsURL = "https://docs.terragrunt.com/features/catalog/"

// StatusFunc receives progress updates during discovery and loading.
type StatusFunc func(msg string)

// LoadFunc performs source discovery and module loading in the background.
// It calls status with human-readable progress updates.
// It returns a ready CatalogService, or nil if no sources were found.
type LoadFunc func(ctx context.Context, status StatusFunc) (catalog.CatalogService, error)

// OpenURLFunc opens a URL in the user's browser. Injected so tests can
// substitute a no-op or a recording stub.
type OpenURLFunc func(url string) error

type welcomeState int

const (
	welcomeLoading welcomeState = iota
	welcomeNoSources
)

// discoveryCompleteMsg is sent when background discovery finishes.
type discoveryCompleteMsg struct {
	svc catalog.CatalogService
	err error
}

// statusUpdateMsg carries a progress update from the LoadFunc.
type statusUpdateMsg string

const statusChannelSize = 10

var (
	welcomeTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#A8ACB1")).
				Background(lipgloss.Color("#1D252F")).
				Padding(0, 1)

	welcomeBodyStyle = lipgloss.NewStyle().
				Padding(1, 2) //nolint:mnd

	welcomeCodeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8BE9FD"))

	welcomeHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6272A4"))
)

// WelcomeModel is a BubbleTea model that shows a loading screen while
// discovery runs in the background, then either transitions to the module
// list TUI or settles into a "no sources found" help screen.
type WelcomeModel struct {
	ctx        context.Context
	logger     log.Logger
	opts       *options.TerragruntOptions
	loadFunc   LoadFunc
	openURL    OpenURLFunc
	statusCh   chan string
	statusText string
	spinner    spinner.Model
	state      welcomeState
	width      int
	height     int
}

// NewWelcomeModel creates a WelcomeModel that immediately begins discovery.
func NewWelcomeModel(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, loadFunc LoadFunc) WelcomeModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))

	return WelcomeModel{
		ctx:        ctx,
		logger:     l,
		opts:       opts,
		loadFunc:   loadFunc,
		openURL:    browser.OpenURL,
		statusCh:   make(chan string, statusChannelSize),
		spinner:    s,
		statusText: "Discovering modules from your infrastructure...",
		state:      welcomeLoading,
	}
}

// WithOpenURL replaces the function used to open URLs in the browser.
func (m WelcomeModel) WithOpenURL(fn OpenURLFunc) WelcomeModel { //nolint:gocritic
	m.openURL = fn

	return m
}

// Init implements tea.Model. It starts the spinner and kicks off discovery.
func (m WelcomeModel) Init() tea.Cmd { //nolint:gocritic
	return tea.Batch(m.spinner.Tick, m.startDiscovery(), m.listenForStatus())
}

func (m WelcomeModel) startDiscovery() tea.Cmd { //nolint:gocritic
	ctx := m.ctx
	loadFunc := m.loadFunc
	ch := m.statusCh

	return func() tea.Msg {
		defer close(ch)

		svc, err := loadFunc(ctx, func(msg string) {
			select {
			case ch <- msg:
			default:
			}
		})

		return discoveryCompleteMsg{svc: svc, err: err}
	}
}

func (m WelcomeModel) listenForStatus() tea.Cmd { //nolint:gocritic
	ch := m.statusCh

	return func() tea.Msg {
		status, ok := <-ch
		if !ok {
			return nil
		}

		return statusUpdateMsg(status)
	}
}

// Update implements tea.Model.
func (m WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocritic
	switch msg := msg.(type) {
	case discoveryCompleteMsg:
		return m.handleDiscoveryComplete(msg)
	case statusUpdateMsg:
		m.statusText = string(msg)

		return m, m.listenForStatus()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "h":
			if m.state == welcomeNoSources {
				_ = m.openURL(welcomeDocsURL)
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	if m.state == welcomeLoading {
		var cmd tea.Cmd

		m.spinner, cmd = m.spinner.Update(msg)

		return m, cmd
	}

	return m, nil
}

func (m WelcomeModel) handleDiscoveryComplete(msg discoveryCompleteMsg) (tea.Model, tea.Cmd) { //nolint:gocritic
	if msg.err != nil {
		m.logger.Warnf("Discovery error: %v", msg.err)
	}

	if msg.svc != nil && len(msg.svc.Modules()) > 0 {
		// Transition to the module list TUI
		newModel := NewModel(m.logger, m.opts, msg.svc)
		width, height := m.width, m.height

		return newModel, tea.Batch(
			newModel.Init(),
			// Forward current dimensions so the new model sizes itself
			func() tea.Msg {
				return tea.WindowSizeMsg{Width: width, Height: height}
			},
		)
	}

	m.state = welcomeNoSources

	return m, nil
}

// View implements tea.Model.
func (m WelcomeModel) View() tea.View { //nolint:gocritic
	var content string

	switch m.state {
	case welcomeLoading:
		content = m.loadingView()
	case welcomeNoSources:
		content = m.noSourcesView()
	}

	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	v := tea.NewView(content)
	v.AltScreen = true

	return v
}

func (m WelcomeModel) loadingView() string { //nolint:gocritic
	title := welcomeTitleStyle.Render(" Terragrunt Catalog ")

	body := welcomeBodyStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		"",
		m.spinner.View()+" "+m.statusText,
	))

	return lipgloss.JoinVertical(lipgloss.Center, title, body)
}

func (m WelcomeModel) noSourcesView() string { //nolint:gocritic
	title := welcomeTitleStyle.Render(" Terragrunt Catalog ")

	body := welcomeBodyStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		"",
		"No module sources were discovered in your infrastructure.",
		"",
		"To get started, you can either:",
		"",
		"  1. Add a catalog block to your root terragrunt.hcl:",
		"",
		welcomeCodeStyle.Render("     catalog {"),
		welcomeCodeStyle.Render(`       urls = ["github.com/your-org/your-modules"]`),
		welcomeCodeStyle.Render("     }"),
		"",
		"  2. Add terraform.source attributes to your unit configurations.",
		"     The catalog will automatically discover referenced modules.",
		"",
		welcomeHintStyle.Render("h: open docs in browser  q/esc: exit"),
	))

	return lipgloss.JoinVertical(lipgloss.Center, title, body)
}

// RunRedesign launches the redesigned catalog experience. It shows a loading
// screen immediately while discovery runs in the background, then transitions
// to the module list if modules are found.
func RunRedesign(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, loadFunc LoadFunc) error {
	model := NewWelcomeModel(ctx, l, opts, loadFunc)

	if _, err := tea.NewProgram(model, tea.WithContext(ctx)).Run(); err != nil {
		if cause := context.Cause(ctx); errors.Is(cause, context.Canceled) {
			return nil
		}

		if cause := context.Cause(ctx); cause != nil {
			return cause
		}

		return err
	}

	return nil
}
