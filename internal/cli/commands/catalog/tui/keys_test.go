package tui_test

import (
	"testing"

	"charm.land/bubbles/v2/key"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
)

func TestKeyDelegateKeyMapShortHelp(t *testing.T) {
	t.Parallel()

	km := tui.NewDelegateKeyMap()
	got := km.ShortHelp()

	require.Len(t, got, 2)
	assert.Equal(t, km.Choose, got[0])
	assert.Equal(t, km.Scaffold, got[1])

	for i, b := range got {
		assert.NotEmpty(t, b.Keys(), "binding %d has no keys", i)
	}
}

func TestKeyDelegateKeyMapFullHelp(t *testing.T) {
	t.Parallel()

	km := tui.NewDelegateKeyMap()
	got := km.FullHelp()

	require.Len(t, got, 1)
	require.Len(t, got[0], 2)
	assert.Equal(t, km.Choose, got[0][0])
	assert.Equal(t, km.Scaffold, got[0][1])
}

func TestKeyPagerKeyMapShortHelp(t *testing.T) {
	t.Parallel()

	km := tui.NewPagerKeyMap()
	got := km.ShortHelp()

	want := []key.Binding{
		km.Up,
		km.Down,
		km.PageUp,
		km.PageDown,
		km.Navigation,
		km.NavigationBack,
		km.Choose,
		km.Scaffold,
		km.Help,
		km.Quit,
	}

	require.Len(t, got, len(want))

	for i := range want {
		assert.Equal(t, want[i], got[i], "binding at index %d differs", i)
	}
}

func TestKeyPagerKeyMapFullHelp(t *testing.T) {
	t.Parallel()

	km := tui.NewPagerKeyMap()
	got := km.FullHelp()

	require.Len(t, got, 3)

	require.Len(t, got[0], 4)
	assert.Equal(t, km.Up, got[0][0])
	assert.Equal(t, km.Down, got[0][1])
	assert.Equal(t, km.PageDown, got[0][2])
	assert.Equal(t, km.PageUp, got[0][3])

	require.Len(t, got[1], 4)
	assert.Equal(t, km.Navigation, got[1][0])
	assert.Equal(t, km.NavigationBack, got[1][1])
	assert.Equal(t, km.Choose, got[1][2])
	assert.Equal(t, km.Scaffold, got[1][3])

	require.Len(t, got[2], 3)
	assert.Equal(t, km.Help, got[2][0])
	assert.Equal(t, km.Quit, got[2][1])
	assert.Equal(t, km.ForceQuit, got[2][2])
}
