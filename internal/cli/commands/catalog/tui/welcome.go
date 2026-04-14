package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/pkg/browser"
)

const welcomeDocsURL = "https://docs.terragrunt.com/features/catalog/"

var (
	welcomeTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#A8ACB1")).
				Background(lipgloss.Color("#1D252F")).
				Padding(0, 1)

	welcomeBodyStyle = lipgloss.NewStyle().
				Padding(1, 2)

	welcomeCodeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8BE9FD"))

	welcomeLinkStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#50FA7B")).
				Underline(true)

	welcomeHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6272A4"))
)

// WelcomeModel is a simple BubbleTea model that displays a welcome screen
// when no module sources are discovered and no catalog block exists.
type WelcomeModel struct {
	width  int
	height int
}

// NewWelcomeModel creates a new WelcomeModel.
func NewWelcomeModel() WelcomeModel {
	return WelcomeModel{}
}

// Init implements tea.Model.
func (m WelcomeModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m WelcomeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		case "h":
			_ = browser.OpenURL(welcomeDocsURL)
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}

	return m, nil
}

// View implements tea.Model.
func (m WelcomeModel) View() tea.View {
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

	content := lipgloss.JoinVertical(lipgloss.Center, title, body)

	// Center the content in the terminal if dimensions are known
	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	v := tea.NewView(content)
	v.AltScreen = true

	return v
}

// RunWelcome launches the welcome screen TUI.
func RunWelcome(ctx context.Context) error {
	if _, err := tea.NewProgram(NewWelcomeModel(), tea.WithContext(ctx)).Run(); err != nil {
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
