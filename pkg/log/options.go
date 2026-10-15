package log

import (
	"io"

	"github.com/sirupsen/logrus"
)

// Option is a function to set options for logger.
type Option func(logger *logger)

// WithLevel sets the logger level.
func WithLevel(level Level) Option {
	return func(logger *logger) {
		logger.Logger.SetLevel(level.ToLogrusLevel())
	}
}

// WithOutput sets the logger output.
func WithOutput(output io.Writer) Option {
	return func(logger *logger) {
		logger.Logger.SetOutput(output)
	}
}

// WithOutputWrapper sets the logger output to what `wrap` returns for the
// output the logger has now. It must not run while another goroutine sets the
// output of the same logger.
func WithOutputWrapper(wrap func(output io.Writer) io.Writer) Option {
	return func(logger *logger) {
		logger.Logger.SetOutput(wrap(logger.Logger.Out))
	}
}

// WithFormatter sets the logger formatter.
func WithFormatter(formatter Formatter) Option {
	return func(logger *logger) {
		logger.SetFormatter(formatter)
	}
}

// WithHooks adds hooks to the logger hooks.
func WithHooks(hooks ...logrus.Hook) Option {
	return func(logger *logger) {
		for _, hook := range hooks {
			logger.Logger.AddHook(hook)
		}
	}
}
