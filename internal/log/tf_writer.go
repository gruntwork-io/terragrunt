package log

import (
	"bytes"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/internal/log/formatter"
	"github.com/sirupsen/logrus"
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
	entry    *logrus.Entry
	tfPath   string
	isStderr bool
}

func TFWriter(entry *logrus.Entry, tfPath string, isStderr bool) io.Writer {
	return &tfWriter{
		entry:    entry,
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

		if writer.tfPath != "" {
			writer.entry.Data[formatter.TFBinaryKeyName] = tfPath
		}

		level, err := logrus.ParseLevel(strings.ToLower(levelStr))
		if err != nil {
			level = formatter.StdoutLevel
		}

		writer.entry.Logger.Level = level

		if timeStr != "" {
			t, err := time.Parse(tfTimestampFormat, timeStr)
			if err != nil {
				return 0, errors.WithStackTrace(err)
			}

			writer.entry.Time = t
		}

		writer.entry.Log(level, msg)
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
