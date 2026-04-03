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

func newV1Adapter(m tui.Model) v1ModelAdapter { //nolint:gocritic
	return v1ModelAdapter{inner: m}
}

// translateV1ToV2 converts v1 message types into their v2 equivalents so the
// inner model's type switches match correctly.
func translateV1ToV2(msg teav1.Msg) tea.Msg {
	switch msg := msg.(type) {
	case teav1.KeyMsg:
		return translateKeyMsg(msg)
	case teav1.WindowSizeMsg:
		return tea.WindowSizeMsg{Width: msg.Width, Height: msg.Height}
	default:
		return msg
	}
}

// translateKeyMsg converts a v1 KeyMsg into a v2 KeyPressMsg.
func translateKeyMsg(msg teav1.KeyMsg) tea.KeyPressMsg {
	if msg.Type == teav1.KeyRunes {
		text := string(msg.Runes)

		var code rune
		if len(msg.Runes) > 0 {
			code = msg.Runes[0]
		}

		k := tea.KeyPressMsg{Code: code, Text: text}
		if msg.Alt {
			k.Mod = tea.ModAlt
		}

		return k
	}

	code := v1TypeToV2Code(msg.Type)
	k := tea.KeyPressMsg{Code: code}

	if msg.Alt {
		k.Mod = tea.ModAlt
	}

	return k
}

// v1TypeToV2Code maps v1 KeyType constants to v2 rune codes.
//
//nolint:cyclop
func v1TypeToV2Code(t teav1.KeyType) rune {
	switch t {
	case teav1.KeyEnter:
		return tea.KeyEnter
	case teav1.KeyEsc:
		return tea.KeyEsc
	case teav1.KeyTab:
		return tea.KeyTab
	case teav1.KeyBackspace:
		return tea.KeyBackspace
	case teav1.KeyUp:
		return tea.KeyUp
	case teav1.KeyDown:
		return tea.KeyDown
	case teav1.KeyLeft:
		return tea.KeyLeft
	case teav1.KeyRight:
		return tea.KeyRight
	case teav1.KeyHome:
		return tea.KeyHome
	case teav1.KeyEnd:
		return tea.KeyEnd
	case teav1.KeyPgUp:
		return tea.KeyPgUp
	case teav1.KeyPgDown:
		return tea.KeyPgDown
	case teav1.KeyDelete:
		return tea.KeyDelete
	case teav1.KeySpace:
		return tea.KeySpace
	default:
		return 0
	}
}

// translateV2ToV1 converts v2 message types that the v1 tea.Program needs to
// recognize (like QuitMsg) into their v1 equivalents.
func translateV2ToV1(msg any) teav1.Msg {
	switch msg.(type) {
	case tea.QuitMsg:
		return teav1.QuitMsg{}
	default:
		return msg
	}
}

// wrapCmd wraps a v2 Cmd so that its return message is translated to v1 types.
func wrapCmd(cmd tea.Cmd) teav1.Cmd {
	if cmd == nil {
		return nil
	}

	return func() teav1.Msg {
		return translateV2ToV1(cmd())
	}
}

// The Init, Update, and View methods use value receivers to satisfy the
// teav1.Model interface which requires value-receiver methods.

func (a v1ModelAdapter) Init() teav1.Cmd { //nolint:gocritic
	return wrapCmd(a.inner.Init())
}

func (a v1ModelAdapter) Update(msg teav1.Msg) (teav1.Model, teav1.Cmd) { //nolint:gocritic
	m, cmd := a.inner.Update(translateV1ToV2(msg))
	return v1ModelAdapter{inner: m.(tui.Model)}, wrapCmd(cmd)
}

func (a v1ModelAdapter) View() string { //nolint:gocritic
	return a.inner.View().Content
}

// unwrap returns the underlying v2 tui.Model from a v1 adapter.
func unwrap(m teav1.Model) tui.Model {
	return m.(v1ModelAdapter).inner
}
