package tui_test

import (
	"testing"
	"testing/synctest"
	"time"

	viewtui "github.com/gruntwork-io/terragrunt/internal/view/tui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToastStackPushDropAndCap(t *testing.T) {
	t.Parallel()

	var s viewtui.ToastStack

	// Each push schedules its own expiry.
	require.NotNil(t, s.Push("first"))
	require.NotNil(t, s.Push("second"))
	require.NotNil(t, s.Push("third"))
	require.NotNil(t, s.Push("fourth"))

	// The cap drops the oldest; the rest overlay in order.
	content := s.Overlay("base", 80, 24)
	assert.NotContains(t, content, "first")
	assert.Contains(t, content, "second")
	assert.Contains(t, content, "third")
	assert.Contains(t, content, "fourth")

	// IDs are sequential from 1; dropping an already-dropped toast is a no-op.
	s.Drop(2)
	s.Drop(2)

	content = s.Overlay("base", 80, 24)
	assert.NotContains(t, content, "second")
	assert.Contains(t, content, "third")
}

func TestToastSanitizesMessages(t *testing.T) {
	t.Parallel()

	var s viewtui.ToastStack

	// Warnings quote paths and parse errors, so a hostile file name reaches the
	// stack through the log.
	require.NotNil(t, s.Push("failed to parse /repo/mod\x1b]52;c;cHduZWQ=\a/terragrunt.hcl"))

	content := s.Overlay("base", 80, 24)
	assert.NotContains(t, content, "\x1b]", "toasts only ever emit SGR styling escapes")
	assert.NotContains(t, content, "\a")
}

func TestToastStackOverlayNoops(t *testing.T) {
	t.Parallel()

	var s viewtui.ToastStack

	// No toasts: content passes through untouched.
	assert.Equal(t, "base", s.Overlay("base", 80, 24))

	// Too narrow for a toast frame: content passes through untouched.
	require.NotNil(t, s.Push("warning"))
	assert.Equal(t, "base", s.Overlay("base", 4, 24))
}

func TestClipToPane(t *testing.T) {
	t.Parallel()

	// Height clips the line count.
	assert.Equal(t, "a\nb", viewtui.ClipToPane("a\nb\nc\nd", 10, 2))
	// Width truncates each line.
	assert.Equal(t, "hello", viewtui.ClipToPane("hello world", 5, 10))
	// Non-positive dimensions yield no content.
	assert.Empty(t, viewtui.ClipToPane("anything", 0, 0))
}

func TestToastStackPushSchedulesExpiry(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		var s viewtui.ToastStack

		start := time.Now()
		expireFirst := s.Push("first")
		require.NotNil(t, s.Push("second"))
		require.NotNil(t, expireFirst)

		// The command fires once the toast's TTL elapses and names the toast
		// it was scheduled for.
		msg := expireFirst()

		assert.Equal(t, 5*time.Second, time.Since(start))
		assert.Equal(t, viewtui.ToastExpired{ID: 1}, msg)

		expired, ok := msg.(viewtui.ToastExpired)
		require.True(t, ok)
		s.Drop(expired.ID)

		content := s.Overlay("base", 80, 24)
		assert.NotContains(t, content, "first")
		assert.Contains(t, content, "second")
	})
}

func TestListenForWarnings(t *testing.T) {
	t.Parallel()

	t.Run("nil channel", func(t *testing.T) {
		t.Parallel()

		assert.Nil(t, viewtui.ListenForWarnings(nil))
	})

	t.Run("delivers next warning", func(t *testing.T) {
		t.Parallel()

		ch := make(chan viewtui.Warning, 2)
		ch <- viewtui.Warning{Message: "one"}

		ch <- viewtui.Warning{Message: "two"}

		cmd := viewtui.ListenForWarnings(ch)
		require.NotNil(t, cmd)

		// Each call delivers one warning, so the handler re-arms the same
		// command to receive the next.
		assert.Equal(t, viewtui.Warning{Message: "one"}, cmd())
		assert.Equal(t, viewtui.Warning{Message: "two"}, cmd())
	})

	t.Run("closed channel", func(t *testing.T) {
		t.Parallel()

		ch := make(chan viewtui.Warning)
		close(ch)

		cmd := viewtui.ListenForWarnings(ch)
		require.NotNil(t, cmd)
		assert.Nil(t, cmd())
	})
}
