package redesign

import (
	"context"
	"errors"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pkg/browser"

	"github.com/gruntwork-io/terragrunt/internal/services/catalog"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/module"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

const welcomeDocsURL = "https://docs.terragrunt.com/features/catalog/"

// StatusFunc receives progress updates during discovery and loading.
type StatusFunc func(msg string)

// LoadFunc performs source discovery and module loading in the background.
// It calls status with human-readable progress updates and sends each
// discovered module on moduleCh as it is found. It returns a ready
// CatalogService (for scaffolding), or nil if no sources were found.
type LoadFunc func(ctx context.Context, status StatusFunc, moduleCh chan<- *module.Module) (catalog.CatalogService, error)

// OpenURLFunc opens a URL in the user's browser. Injected so tests can
// substitute a no-op or a recording stub.
type OpenURLFunc func(url string) error

type welcomeState int

const (
	welcomeLoading welcomeState = iota
	welcomeNoSources
	welcomeDiscoveryError
)

// DiscoveryCompleteMsg is sent when background discovery finishes.
type DiscoveryCompleteMsg struct {
	Svc catalog.CatalogService
	Err error
}

// moduleMsg carries a single newly-discovered module from the LoadFunc.
type moduleMsg struct {
	module *module.Module
}

// ModuleMsg creates a moduleMsg for testing.
func ModuleMsg(mod *module.Module) tea.Msg {
	return moduleMsg{module: mod}
}

// StatusUpdateMsg carries a progress update from the LoadFunc.
type StatusUpdateMsg string

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
	ctx              context.Context
	logger           log.Logger
	lastDiscoveryErr error
	moduleCh         chan *module.Module
	openURL          OpenURLFunc
	statusCh         chan string
	loadFunc         LoadFunc
	opts             *options.TerragruntOptions
	statusText       string
	spinner          spinner.Model
	state            welcomeState
	width            int
	height           int
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
		moduleCh:   make(chan *module.Module, statusChannelSize),
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
	return tea.Batch(m.spinner.Tick, m.startDiscovery(), m.listenForStatus(), m.listenForModule())
}

func (m WelcomeModel) startDiscovery() tea.Cmd { //nolint:gocritic
	ctx := m.ctx
	loadFunc := m.loadFunc
	statusCh := m.statusCh
	moduleCh := m.moduleCh

	return func() tea.Msg {
		defer close(statusCh)
		defer close(moduleCh)

		svc, err := loadFunc(ctx, func(msg string) {
			select {
			case statusCh <- msg:
			default:
			}
		}, moduleCh)

		return DiscoveryCompleteMsg{Svc: svc, Err: err}
	}
}

func (m WelcomeModel) listenForStatus() tea.Cmd { //nolint:gocritic
	ch := m.statusCh

	return func() tea.Msg {
		status, ok := <-ch
		if !ok {
			return nil
		}

		return StatusUpdateMsg(status)
	}
}

func (m WelcomeModel) listenForModule() tea.Cmd { //nolint:gocritic
	ch := m.moduleCh

	return func() tea.Msg {
		mod, ok := <-ch
		if !ok {
			return nil
		}

		return moduleMsg{module: mod}
	}
}

// Update implements tea.Model.
func (m WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocritic
	switch msg := msg.(type) {
	case DiscoveryCompleteMsg:
		return m.handleDiscoveryComplete(msg)
	case moduleMsg:
		return m.handleModuleMsg(msg)
	case StatusUpdateMsg:
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

func (m WelcomeModel) handleModuleMsg(msg moduleMsg) (tea.Model, tea.Cmd) { //nolint:gocritic
	// First module: transition to the catalog list immediately
	newModel := NewModelStreaming(m.logger, m.opts, msg.module, m.moduleCh)
	width, height := m.width, m.height

	initCmds := []tea.Cmd{newModel.Init()}
	if width > 0 && height > 0 {
		initCmds = append(initCmds, func() tea.Msg {
			return tea.WindowSizeMsg{Width: width, Height: height}
		})
	}

	return newModel, tea.Batch(initCmds...)
}

func (m WelcomeModel) handleDiscoveryComplete(msg DiscoveryCompleteMsg) (tea.Model, tea.Cmd) { //nolint:gocritic
	if msg.Err != nil {
		m.logger.Warnf("Discovery error: %v", msg.Err)
		m.lastDiscoveryErr = msg.Err
		m.state = welcomeDiscoveryError

		return m, nil
	}

	// Defensive: if we're still on the loading screen but the service has
	// modules, transition to the catalog list. This shouldn't normally
	// happen since moduleMsg transitions first.
	if msg.Svc != nil && len(msg.Svc.Modules()) > 0 {
		newModel := NewModel(m.logger, m.opts, msg.Svc)
		width, height := m.width, m.height

		initCmds := []tea.Cmd{newModel.Init()}
		if width > 0 && height > 0 {
			initCmds = append(initCmds, func() tea.Msg {
				return tea.WindowSizeMsg{Width: width, Height: height}
			})
		}

		return newModel, tea.Batch(initCmds...)
	}

	// No modules were ever discovered — show the welcome screen.
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
	case welcomeDiscoveryError:
		content = m.discoveryErrorView()
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

func (m WelcomeModel) discoveryErrorView() string { //nolint:gocritic
	title := welcomeTitleStyle.Render(" Terragrunt Catalog ")

	errMsg := "unknown error"
	if m.lastDiscoveryErr != nil {
		errMsg = m.lastDiscoveryErr.Error()
	}

	body := welcomeBodyStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		"",
		"An error occurred while discovering catalog sources:",
		"",
		welcomeCodeStyle.Render("  "+errMsg),
		"",
		"Please check your network connection, authentication, and",
		"catalog configuration, then try again.",
		"",
		welcomeHintStyle.Render("q/esc: exit"),
	))

	return lipgloss.JoinVertical(lipgloss.Center, title, body)
}

// RunRedesign launches the redesigned catalog experience. It shows a loading
// screen immediately while discovery runs in the background, then transitions
// to the module list if modules are found.
func RunRedesign(ctx context.Context, l log.Logger, opts *options.TerragruntOptions, loadFunc LoadFunc) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

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
