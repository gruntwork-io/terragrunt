package writer

import "github.com/gruntwork-io/terragrunt/pkg/log"

type Option func(writer *Writer)

func WithLogger(logger log.Logger) Option {
	return func(writer *Writer) {
		writer.logger = logger
	}
}

func WithDefaultLevel(level log.Level) Option {
	return func(writer *Writer) {
		writer.defaultLevel = level
	}
}

func WithSplitLines() Option {
	return func(writer *Writer) {
		writer.splitLines = true
	}
}

func WithParseFunc(fn WriterParseFunc) Option {
	return func(writer *Writer) {
		writer.parseFunc = fn
	}
}
