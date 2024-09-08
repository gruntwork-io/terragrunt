package writer

import (
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// startASNISeq is the ANSI start escape sequence
	startASNISeq = "\033["
	// resetANSISeq is the ANSI reset escape sequence
	resetANSISeq = "\033[0m"
)

type WriterParseFunc func(str string) (msg string, time *time.Time, level *log.Level, err error)

// Writer redirects Write requests to configured logger and level
type Writer struct {
	logger       log.Logger
	defaultLevel log.Level
	splitLines   bool
	parseFunc    WriterParseFunc
}

func New(opts ...Option) *Writer {
	writer := &Writer{
		logger:       log.DefaultLogger,
		defaultLevel: log.InfoLevel,
		parseFunc:    func(str string) (msg string, time *time.Time, level *log.Level, err error) { return str, nil, nil, nil },
	}
	writer.SetOption(opts...)

	return writer
}

func (writer *Writer) SetOption(opts ...Option) {
	for _, opt := range opts {
		opt(writer)
	}
}

func (writer *Writer) Write(p []byte) (n int, err error) {
	var (
		str  = string(p)
		strs = []string{str}
	)

	if writer.splitLines {
		strs = strings.Split(str, "\n")
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
		if strings.Contains(msg, startASNISeq) {
			msg += resetANSISeq
		}

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
