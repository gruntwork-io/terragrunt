package log

import (
	"bytes"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
)

const (
	tfTimestampFormat = "2006-01-02T15:04:05.000-0700"

	// startASNISeq is the ANSI start escape sequence
	startASNISeq = "\033["
	// resetANSISeq is the ANSI reset escape sequence
	resetANSISeq = "\033[0m"
)

var extractTimeAndLevelReg = regexp.MustCompile(`(?i)(\S+)\s*\[(trace|debug|warn|info|error)\]\s*(.+\S)`)

type tfWriter struct {
	logger   Logger
	tfPath   string
	isStderr bool
}

func TFWriter(logger Logger, tfPath string, isStderr bool) io.Writer {
	return &tfWriter{
		logger:   logger,
		tfPath:   filepath.Base(tfPath),
		isStderr: isStderr,
	}
}

func (writer *tfWriter) Write(p []byte) (int, error) {
	lines := bytes.Split(p, []byte{'\n'})

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var (
			timeStr, levelStr string
			msg               = string(line)
			tfPath            = writer.tfPath
			logger            = writer.logger
		)

		if writer.isStderr {
			timeStr, levelStr, msg = extractTimeAndLevel(msg)
			if timeStr != "" {
				tfPath += " TF_LOG"
			}
		}

		// Reset ANSI styles at the end of a line so that TG log does not inherit them on a new line
		if strings.Contains(msg, startASNISeq) {
			msg += resetANSISeq
		}

		if tfPath != "" {
			logger = logger.WithField(FieldKeyTFBinary, tfPath)
		}

		if timeStr != "" {
			t, err := time.Parse(tfTimestampFormat, timeStr)
			if err != nil {
				return 0, errors.WithStackTrace(err)
			}

			logger = logger.WithTime(t)
		}

		level, err := ParseLevel(strings.ToLower(levelStr))
		if err != nil {
			if writer.isStderr {
				level = StderrLevel
			} else {
				level = StdoutLevel
			}
		}

		logger.Log(level, msg)
	}

	return len(p), nil
}

func extractTimeAndLevel(msg string) (string, string, string) {
	const numberOfValues = 4

	if extractTimeAndLevelReg.MatchString(msg) {
		if match := extractTimeAndLevelReg.FindStringSubmatch(msg); len(match) == numberOfValues {
			return match[1], match[2], match[3]
		}
	}

	return "", "", msg
}
