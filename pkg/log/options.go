package log

import (
	"io"

	"github.com/sirupsen/logrus"
)

type Option func(logger *logger)

func WithLevel(level Level) Option {
	return func(logger *logger) {
		logger.Logger.SetLevel(level.ToLogrusLevel())
	}
}

func WithOutput(output io.Writer) Option {
	return func(logger *logger) {
		logger.Logger.SetOutput(output)
	}
}

func WithFormatter(formatter logrus.Formatter) Option {
	return func(logger *logger) {
		logger.Logger.SetFormatter(formatter)
	}
}

func WithHooks(hooks ...logrus.Hook) Option {
	return func(logger *logger) {
		for _, hook := range hooks {
			logger.Logger.AddHook(hook)
		}
	}
}
