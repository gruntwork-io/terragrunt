package tui_test

import (
	"testing"

	tui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/stretchr/testify/require"
)

func TestEnsureTTY_TerminalStdin(t *testing.T) {
	t.Parallel()

	require.NoError(t, tui.EnsureTTY(func() bool { return true }))
}

func TestEnsureTTY_NoTerminal(t *testing.T) {
	t.Parallel()

	require.ErrorIs(t, tui.EnsureTTY(func() bool { return false }), tui.ErrNoTerminal)
}
