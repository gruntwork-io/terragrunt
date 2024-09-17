// Package writer provides a writer that redirects Write requests to configured logger and level.
package writer

import (
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// WriterParseFunc is a function used to parse records to extract the time and level from them.
type WriterParseFunc func(str string) (msg string, time *time.Time, level *log.Level, err error)

// Writer redirects Write requests to configured logger and level
type Writer struct {
	logger       log.Logger
	defaultLevel log.Level
	msgSeparator string
	parseFunc    WriterParseFunc
}

// New returns a new Writer instance with fields assigned to default values.
func New(opts ...Option) *Writer {
	writer := &Writer{
		logger:       log.Default(),
		defaultLevel: log.InfoLevel,
		parseFunc:    func(str string) (msg string, time *time.Time, level *log.Level, err error) { return str, nil, nil, nil },
	}
	writer.SetOption(opts...)

	return writer
}

// SetOption sets options to the `Writer`.
func (writer *Writer) SetOption(opts ...Option) {
	for _, opt := range opts {
		opt(writer)
	}
}

// Write implements `io.Writer` interface.
func (writer *Writer) Write(p []byte) (n int, err error) {
	var (
		str  = string(p)
		strs = []string{str}
	)

	if writer.msgSeparator != "" {
		strs = strings.Split(str, writer.msgSeparator)
	}

	for _, str := range strs {
		if len(str) == 0 {
			continue
		}

		msg, time, level, err := writer.parseFunc(str)
		if err != nil {
			return 0, err
		}

		// Reset ANSI styles at the end of a line so that the new line does not inherit them
		msg = log.ResetASCISeq(msg)

		logger := writer.logger

		if time != nil {
			logger = logger.WithTime(*time)
		}

		if level == nil {
			level = &writer.defaultLevel
		}

		logger.Log(*level, msg)
	}

	return len(p), nil
}
