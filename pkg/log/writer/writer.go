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
	parseFunc    WriterParseFunc
	msgSeparator string
	defaultLevel log.Level
}

// New returns a new Writer instance with fields assigned to default values.
func New(opts ...Option) *Writer {
	writer := &Writer{
		logger:       log.Default(),
		defaultLevel: log.InfoLevel,
		parseFunc: func(str string) (msg string, time *time.Time, level *log.Level, err error) {
			return str, nil, nil, nil
		},
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
	str := string(p)

	if writer.msgSeparator == "" {
		if err := writer.logRecord(str); err != nil {
			return 0, err
		}

		return len(p), nil
	}

	// Every byte of tofu output crosses this writer, so range over the records as
	// they are split instead of collecting them into a slice first.
	for record := range strings.SplitSeq(str, writer.msgSeparator) {
		if err := writer.logRecord(record); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (writer *Writer) logRecord(str string) error {
	if len(str) == 0 {
		return nil
	}

	msg, time, level, err := writer.parseFunc(str)
	if err != nil {
		return err
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

	return nil
}
