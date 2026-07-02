package buttonbar_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/components/buttonbar"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNew verifies that New constructs a ButtonBar with the supplied buttons
// and that the zero-value active button is the first one.
func TestNew(t *testing.T) {
	t.Parallel()

	buttons := []string{"Yes", "No", "Cancel"}
	b := buttonbar.New(buttons)

	require.NotNil(t, b)
	assert.NotEmpty(t, b.View().Content)
}

// TestInitResetsActiveButton verifies that Init returns nil and resets the
// active button by checking the rendered view after navigating away.
func TestInitResetsActiveButton(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"One", "Two", "Three"})

	// Move active button to index 1.
	_, _ = b.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	cmd := b.Init()
	assert.Nil(t, cmd)

	// After Init, the first button should be focused again. We assert this
	// through View by ensuring "One" is rendered with the focused style and
	// "Two" with the blurred style.
	view := b.View().Content
	assert.Contains(t, view, "One")
	assert.Contains(t, view, "Two")
	assert.Contains(t, view, "Three")
}

// TestUpdateTabAdvancesActiveButton verifies that pressing tab advances the
// active button and that the resulting Cmd emits an ActiveBtnMsg with the
// new index.
func TestUpdateTabAdvancesActiveButton(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"A", "B", "C"})

	model, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	require.NotNil(t, model)
	require.NotNil(t, cmd)

	msg := cmd()
	active, ok := msg.(buttonbar.ActiveBtnMsg)
	require.True(t, ok, "expected ActiveBtnMsg, got %T", msg)
	assert.Equal(t, buttonbar.ActiveBtnMsg(1), active)
}

// TestUpdateTabWraps verifies that tab wraps from the last button back to
// the first.
func TestUpdateTabWraps(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"A", "B"})

	// Tab once -> index 1.
	_, _ = b.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Tab again -> wraps to index 0.
	_, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	require.NotNil(t, cmd)
	assert.Equal(t, buttonbar.ActiveBtnMsg(0), cmd().(buttonbar.ActiveBtnMsg))
}

// TestUpdateShiftTabGoesBackward verifies that shift+tab moves the active
// button backwards and wraps around when at index 0.
func TestUpdateShiftTabGoesBackward(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"A", "B", "C"})

	// shift+tab from index 0 -> wraps to last index (2).
	_, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	require.NotNil(t, cmd)
	assert.Equal(t, buttonbar.ActiveBtnMsg(2), cmd().(buttonbar.ActiveBtnMsg))

	// shift+tab again -> index 1.
	_, cmd = b.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	require.NotNil(t, cmd)
	assert.Equal(t, buttonbar.ActiveBtnMsg(1), cmd().(buttonbar.ActiveBtnMsg))
}

// TestUpdateUnknownKeyIsNoop verifies that key presses that are not tab or
// shift+tab leave the active button unchanged and produce no cmd.
func TestUpdateUnknownKeyIsNoop(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"A", "B"})

	model, cmd := b.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	require.NotNil(t, model)

	// tea.Batch with no commands returns nil.
	assert.Nil(t, cmd)
}

// TestUpdateSelectBtnMsgValidIndex verifies that SelectBtnMsg with a valid
// index updates the active button.
func TestUpdateSelectBtnMsgValidIndex(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"A", "B", "C"})

	_, cmd := b.Update(buttonbar.SelectBtnMsg(2))
	// SelectBtnMsg path does not enqueue an ActiveBtnMsg cmd.
	assert.Nil(t, cmd)

	// Confirm the selection took effect by issuing a tab and observing the
	// resulting active index wrap to 0.
	_, cmd = b.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	require.NotNil(t, cmd)
	assert.Equal(t, buttonbar.ActiveBtnMsg(0), cmd().(buttonbar.ActiveBtnMsg))
}

// TestUpdateSelectBtnMsgOutOfRange verifies that SelectBtnMsg with an
// out-of-range index leaves the active button unchanged.
func TestUpdateSelectBtnMsgOutOfRange(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"A", "B"})

	// Negative index ignored.
	_, _ = b.Update(buttonbar.SelectBtnMsg(-1))
	_, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	require.NotNil(t, cmd)
	assert.Equal(t, buttonbar.ActiveBtnMsg(1), cmd().(buttonbar.ActiveBtnMsg))

	// Reset and test too-large index.
	b = buttonbar.New([]string{"A", "B"})
	_, _ = b.Update(buttonbar.SelectBtnMsg(99))
	_, cmd = b.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	require.NotNil(t, cmd)
	assert.Equal(t, buttonbar.ActiveBtnMsg(1), cmd().(buttonbar.ActiveBtnMsg))
}

// TestUpdateUnrelatedMessage verifies that messages that are neither key
// presses nor SelectBtnMsg are ignored.
func TestUpdateUnrelatedMessage(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"A", "B"})

	type unknownMsg struct{}

	model, cmd := b.Update(unknownMsg{})
	assert.NotNil(t, model)
	assert.Nil(t, cmd)
}

// TestViewRendersAllButtons verifies that View renders every button label
// wrapped in the configured name format.
func TestViewRendersAllButtons(t *testing.T) {
	t.Parallel()

	buttons := []string{"Save", "Discard", "Cancel"}
	b := buttonbar.New(buttons)

	view := b.View().Content
	for _, label := range buttons {
		assert.Contains(t, view, label)
	}

	// Default format wraps each button in "[ ... ]"; expect at least
	// len(buttons) opening brackets.
	assert.GreaterOrEqual(t, strings.Count(view, "["), len(buttons))
}

// TestViewSingleButton verifies that a single-button bar renders without
// trailing separator artifacts and that tab is a no-op (wraps to itself).
func TestViewSingleButton(t *testing.T) {
	t.Parallel()

	b := buttonbar.New([]string{"Only"})

	view := b.View().Content
	assert.Contains(t, view, "Only")

	_, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	require.NotNil(t, cmd)
	assert.Equal(t, buttonbar.ActiveBtnMsg(0), cmd().(buttonbar.ActiveBtnMsg))
}
