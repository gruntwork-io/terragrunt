package tui_test

import (
	tea "charm.land/bubbletea/v2"
	teav1 "github.com/charmbracelet/bubbletea"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

// v1ModelAdapter wraps a bubbletea v2 tui.Model to satisfy the v1 tea.Model
// interface, allowing us to use the teatest package (which depends on v1) with
// our v2 model.
type v1ModelAdapter struct {
	inner tui.Model
}

func newV1Adapter(m tui.Model) v1ModelAdapter {
	return v1ModelAdapter{inner: m}
}

func (a v1ModelAdapter) Init() teav1.Cmd {
	cmd := a.inner.Init()
	if cmd == nil {
		return nil
	}

	return func() teav1.Msg {
		return cmd()
	}
}

func (a v1ModelAdapter) Update(msg teav1.Msg) (teav1.Model, teav1.Cmd) {
	m, cmd := a.inner.Update(msg)
	adapted := v1ModelAdapter{inner: m.(tui.Model)}

	if cmd == nil {
		return adapted, nil
	}

	return adapted, func() teav1.Msg {
		return cmd()
	}
}

func (a v1ModelAdapter) View() string {
	return a.inner.View().Content
}

// unwrap returns the underlying v2 tui.Model from a v1 adapter.
func unwrap(m teav1.Model) tui.Model {
	return m.(v1ModelAdapter).inner
}

// v1KeyPress creates a v1 tea.KeyMsg from a v2 tea.KeyPressMsg.
func v1KeyPress(msg tea.KeyPressMsg) teav1.KeyMsg {
	k := tea.Key(msg)

	// For special keys (Enter, Esc, etc.)
	if k.Text == "" {
		keyType := teav1.KeyRunes

		switch k.Code {
		case tea.KeyEnter:
			keyType = teav1.KeyEnter
		case tea.KeyEsc:
			keyType = teav1.KeyEsc
		case tea.KeyTab:
			keyType = teav1.KeyTab
		case tea.KeyBackspace:
			keyType = teav1.KeyBackspace
		case tea.KeyUp:
			keyType = teav1.KeyUp
		case tea.KeyDown:
			keyType = teav1.KeyDown
		case tea.KeyLeft:
			keyType = teav1.KeyLeft
		case tea.KeyRight:
			keyType = teav1.KeyRight
		}

		return teav1.KeyMsg{Type: keyType}
	}

	return teav1.KeyMsg{
		Type:  teav1.KeyRunes,
		Runes: []rune(k.Text),
	}
}
