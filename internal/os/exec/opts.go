package exec

import (
	"fmt"
	"slices"
	"time"
)

const envVarsListFormat = "%s=%s"

// envMapToSortedSlice formats env as "KEY=VALUE" strings sorted alphabetically.
func envMapToSortedSlice(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf(envVarsListFormat, k, v))
	}

	slices.Sort(out)

	return out
}

// Option is type for passing options to the Cmd.
type Option func(*Cmd)

// WithUsePTY enables a pty for the Cmd.
func WithUsePTY(state bool) Option {
	return func(cmd *Cmd) {
		cmd.usePTY = state
	}
}

// WithEnv sets envs to the Cmd.
func WithEnv(env map[string]string) Option {
	return func(cmd *Cmd) {
		cmd.SetEnv(envMapToSortedSlice(env))
	}
}

// WithForwardSignalDelay sets forwarding signal delay to the Cmd.
func WithForwardSignalDelay(delay time.Duration) Option {
	return func(cmd *Cmd) {
		cmd.forwardSignalDelay = delay
	}
}

// WithGracefulShutdownDelay sets the time to wait for a process to exit gracefully
// after sending an interrupt signal before escalating to SIGKILL.
// This allows processes like Terraform to clean up child processes (e.g., provider plugins).
func WithGracefulShutdownDelay(delay time.Duration) Option {
	return func(cmd *Cmd) {
		cmd.vc.SetWaitDelay(delay)
	}
}
