// Package buttonbar provides a bubbletea component that displays an inline list of buttons.
package buttonbar

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SelectBtnMsg is a message that contains the index of the button to select.
type SelectBtnMsg int

// ActiveBtnMsg is a message that contains the index of the current active button.
type ActiveBtnMsg int

const (
	defaultButtonNameFmt = "[ %s ]"
)

var (
	defaultButtonSeparatorStyle = lipgloss.NewStyle().Padding(0, 0, 0, 1)
	defaultButtonFocusedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	defaultButtonBlurredStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// ButtonBar is bubbletea component that displays an inline list of buttons.
type ButtonBar struct {
	buttons        []string
	activeButton   int
	nameFmt        string
	SeparatorStyle lipgloss.Style
	FocusedStyle   lipgloss.Style
	BlurredStyle   lipgloss.Style
}

// New creates a new ButtonBar component.
func New(buttons []string) *ButtonBar {
	return &ButtonBar{
		buttons:        buttons,
		activeButton:   0,
		nameFmt:        defaultButtonNameFmt,
		SeparatorStyle: defaultButtonSeparatorStyle,
		FocusedStyle:   defaultButtonFocusedStyle,
		BlurredStyle:   defaultButtonBlurredStyle,
	}
}

// Init implements tea.Model.
func (b *ButtonBar) Init() tea.Cmd {
	b.activeButton = 0
	return nil
}

// Update implements tea.Model.
func (b *ButtonBar) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmds := make([]tea.Cmd, 0)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			b.activeButton = (b.activeButton + 1) % len(b.buttons)
			cmds = append(cmds, b.activeBtnCmd)
		case "shift+tab":
			b.activeButton = (b.activeButton - 1 + len(b.buttons)) % len(b.buttons)
			cmds = append(cmds, b.activeBtnCmd)
		}
	case SelectBtnMsg:
		btn := int(msg)
		if btn >= 0 && btn < len(b.buttons) {
			b.activeButton = int(msg)
		}
	}

	return b, tea.Batch(cmds...)
}

// View implements tea.Model.
func (b *ButtonBar) View() string {
	s := strings.Builder{}

	for i, btn := range b.buttons {
		style := b.BlurredStyle
		if i == b.activeButton {
			style = b.FocusedStyle
		}

		s.WriteString(fmt.Sprintf(b.nameFmt, style.Render(btn)))

		if i != len(b.buttons)-1 {
			s.WriteString(b.SeparatorStyle.String())
		}
	}

	return s.String()
}

func (b *ButtonBar) activeBtnCmd() tea.Msg {
	return ActiveBtnMsg(b.activeButton)
}
