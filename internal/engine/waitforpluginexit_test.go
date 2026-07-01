package engine_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/engine"
	"github.com/stretchr/testify/assert"
)

// TestWaitForPluginExit drives the graceful-exit window on synctest's fake clock: a plugin that
// exits inside it is awaited, one that outlives it falls through to the caller's force-kill.
func TestWaitForPluginExit(t *testing.T) {
	t.Parallel()

	const window = time.Second

	t.Run("plugin exits within the grace window", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			start := time.Now()
			exited := func() bool { return time.Since(start) >= window/2 }

			assert.True(t, engine.WaitForPluginExit(exited, window))
		})
	})

	t.Run("plugin outlives the grace window", func(t *testing.T) {
		t.Parallel()

		synctest.Test(t, func(t *testing.T) {
			neverExits := func() bool { return false }

			assert.False(t, engine.WaitForPluginExit(neverExits, window))
		})
	})
}
