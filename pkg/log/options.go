package log

import (
	"io"
)

type Option func(logger *logger)

func SetLevel(level Level) Option {
	return func(logger *logger) {
		logger.Logger.SetLevel(level.toLogrusLevel())
	}
}

func SetOutput(output io.Writer) Option {
	return func(logger *logger) {
		logger.Logger.SetOutput(output)
	}
}

func SetFormat(format Format) Option {
	return func(logger *logger) {
		logger.formatter.OutputFormat = format
	}
}

func ReplaceAbsPathsWithRel(basePath string) (Option, error) {
	hook, err := NewRelativePathHook(basePath)
	if err != nil {
		return nil, err
	}

	return func(logger *logger) {
		logger.Logger.AddHook(hook)
	}, nil
}

func ForceLogLevel(forcedLevel Level) Option {
	hook := NewForceLogLevelHook(forcedLevel)

	return func(logger *logger) {
		logger.Logger.AddHook(hook)
	}
}
