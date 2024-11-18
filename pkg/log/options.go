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

// WithFormatter sets the logger formatter.
func WithFormatter(formatter Formatter) Option {
	return func(logger *logger) {
		logger.Logger.SetFormatter(&fromLogrusFormatter{Formatter: formatter})
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
