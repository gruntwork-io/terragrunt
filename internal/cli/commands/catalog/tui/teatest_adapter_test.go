package tui_test

import (
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

// The Init, Update, and View methods use value receivers to satisfy the
// teav1.Model interface which requires value-receiver methods.

func (a v1ModelAdapter) Init() teav1.Cmd { //nolint:gocritic
	cmd := a.inner.Init()
	if cmd == nil {
		return nil
	}

	return func() teav1.Msg {
		return cmd()
	}
}

func (a v1ModelAdapter) Update(msg teav1.Msg) (teav1.Model, teav1.Cmd) { //nolint:gocritic
	m, cmd := a.inner.Update(msg)
	adapted := v1ModelAdapter{inner: m.(tui.Model)}

	if cmd == nil {
		return adapted, nil
	}

	return adapted, func() teav1.Msg {
		return cmd()
	}
}

func (a v1ModelAdapter) View() string { //nolint:gocritic
	return a.inner.View().Content
}

// unwrap returns the underlying v2 tui.Model from a v1 adapter.
func unwrap(m teav1.Model) tui.Model {
	return m.(v1ModelAdapter).inner
}
