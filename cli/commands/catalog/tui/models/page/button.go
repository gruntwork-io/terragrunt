package page

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gruntwork-io/go-commons/collections"
)

const (
	defaultButtonNameFmt = "[ %s ]"
)

var (
	defaultButtonFocusedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	defatulButtonBlurredStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

type ButtonActionFunc func(msg tea.Msg) tea.Cmd

type Button struct {
	focusedStyle lipgloss.Style
	blurredStyle lipgloss.Style
	index        int
	selected     bool
	nameFmt      string
	name         string
	action       ButtonActionFunc
}

func NewButton(name string, action ButtonActionFunc) *Button {
	return &Button{
		name:         name,
		action:       action,
		nameFmt:      defaultButtonNameFmt,
		focusedStyle: defaultButtonFocusedStyle,
		blurredStyle: defatulButtonBlurredStyle,
	}
}

func (btn *Button) Action(msg tea.Msg) tea.Cmd {
	return btn.action(msg)
}

type Buttons []*Button

func NewButtons(btns ...*Button) Buttons {
	for i, btn := range btns {
		if btn.index == 0 {
			btn.index = i + 1
		}
	}
	return btns
}

func (btns Buttons) Len() int {
	return len(btns)
}

func (btns Buttons) Focus(index ...int) Buttons {
	for i, btn := range btns {
		btn.selected = collections.ListContainsElement(index, i+1)
	}
	return btns
}

func (btns Buttons) Get(index ...int) *Button {
	for i, btn := range btns {
		if collections.ListContainsElement(index, i+1) {
			return btn
		}
	}
	return nil
}

func (btns Buttons) GetByName(name string) *Button {
	for _, btn := range btns {
		if btn.name == name {
			return btn
		}
	}
	return nil
}

func (btns Buttons) View() string {
	names := make([]string, btns.Len())

	for i, btn := range btns {
		if btn.selected {
			names[i] = fmt.Sprintf(btn.nameFmt, btn.focusedStyle.Render(btn.name))
		} else {
			names[i] = fmt.Sprintf(btn.nameFmt, btn.blurredStyle.Render(btn.name))
		}
	}

	leftPadding := 2
	return lipgloss.NewStyle().Padding(0, 0, 0, leftPadding).Render(names...)
}
