package tui

import (
	"github.com/sirupsen/logrus"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// WarnHook is a logrus hook that forwards warn-or-worse log entries as
// [Warning] messages, so warnings raised while the browser owns the screen
// surface as toasts instead of being written over the display. Sends never
// block: when the channel's buffer is full, the entry is dropped rather than
// stalling the goroutine that logged it.
type WarnHook struct {
	ch chan<- Warning
}

// NewWarnHook returns a WarnHook that delivers entries to ch.
func NewWarnHook(ch chan<- Warning) *WarnHook {
	return &WarnHook{ch: ch}
}

// Levels implements logrus.Hook. The hook fires for Terragrunt's warn and
// error levels only; the stdout/stderr levels that relay tool output pass by.
func (h *WarnHook) Levels() []logrus.Level {
	return log.Levels{log.ErrorLevel, log.WarnLevel}.ToLogrusLevels()
}

// Fire implements logrus.Hook.
func (h *WarnHook) Fire(entry *logrus.Entry) error {
	select {
	case h.ch <- Warning{Message: entry.Message}:
	default:
	}

	return nil
}
