package tui_test

import (
	"testing"

	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/browse/tui"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWarnHookForwardsEntries(t *testing.T) {
	t.Parallel()

	ch := make(chan tui.Warning, 1)
	hook := tui.NewWarnHook(ch)

	require.NoError(t, hook.Fire(&logrus.Entry{Message: "cycle detected"}))

	select {
	case w := <-ch:
		assert.Equal(t, tui.Warning{Message: "cycle detected"}, w)
	default:
		t.Fatal("expected a warning on the channel")
	}
}

func TestWarnHookDropsInsteadOfBlocking(t *testing.T) {
	t.Parallel()

	ch := make(chan tui.Warning, 1)
	hook := tui.NewWarnHook(ch)

	require.NoError(t, hook.Fire(&logrus.Entry{Message: "kept"}))

	// The buffer is full: a second fire must return immediately, dropping the
	// entry rather than stalling the logging goroutine.
	require.NoError(t, hook.Fire(&logrus.Entry{Message: "dropped"}))

	assert.Equal(t, tui.Warning{Message: "kept"}, <-ch)

	select {
	case w := <-ch:
		t.Fatalf("expected the overflowing warning to be dropped, got %q", w.Message)
	default:
	}
}

// TestWarnHookFiresForWarnAndErrorOnly pins the hook's level registration in
// Terragrunt's (shifted) logrus level space: warn and error entries fire, and
// the stdout/stderr levels that relay tool output do not.
func TestWarnHookFiresForWarnAndErrorOnly(t *testing.T) {
	t.Parallel()

	levels := tui.NewWarnHook(make(chan tui.Warning, 1)).Levels()

	assert.Contains(t, levels, log.WarnLevel.ToLogrusLevel())
	assert.Contains(t, levels, log.ErrorLevel.ToLogrusLevel())
	assert.NotContains(t, levels, log.InfoLevel.ToLogrusLevel())
	assert.NotContains(t, levels, log.StdoutLevel.ToLogrusLevel())
	assert.NotContains(t, levels, log.StderrLevel.ToLogrusLevel())
}
