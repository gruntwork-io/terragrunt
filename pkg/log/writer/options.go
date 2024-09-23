package writer

import "github.com/gruntwork-io/terragrunt/pkg/log"

// Option is a function to set options for Writer.
type Option func(writer *Writer)

// WithLogger sets Logger to the Writer.
func WithLogger(logger log.Logger) Option {
	return func(writer *Writer) {
		writer.logger = logger
	}
}

// WithDefaultLevel sets the default log level for Writer in case the log level cannot be extracted from the message.
func WithDefaultLevel(level log.Level) Option {
	return func(writer *Writer) {
		writer.defaultLevel = level
	}
}

// WithMsgSeparator configures Writer to split the received text into string and log them as separate records.
func WithMsgSeparator(sep string) Option {
	return func(writer *Writer) {
		writer.msgSeparator = sep
	}
}

// WithParseFunc sets the parser func.
func WithParseFunc(fn WriterParseFunc) Option {
	return func(writer *Writer) {
		writer.parseFunc = fn
	}
}
