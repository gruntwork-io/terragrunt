package exec

import (
	"time"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const envVarsListFormat = "%s=%s"

// Option is type for passing options to the Cmd.
type Option func(*Cmd)

// WithLogger sets Logger to the Cmd.
func WithLogger(logger log.Logger) Option {
	return func(cmd *Cmd) {
		cmd.logger = logger
	}
}

// WithUsePTY enables a pty for the Cmd.
func WithUsePTY(state bool) Option {
	return func(cmd *Cmd) {
		cmd.usePTY = state
	}
}

// WithEnv sets envs to the Cmd.
func WithEnv(env map[string]string) Option {
	return func(cmd *Cmd) {
		cmd.Env = collections.KeyValueStringSliceWithFormat(env, envVarsListFormat)
	}
}

// WithForwardSignalDelay sets forwarding signal delay to the Cmd.
func WithForwardSignalDelay(delay time.Duration) Option {
	return func(cmd *Cmd) {
		cmd.forwardSignalDelay = delay
	}
}
