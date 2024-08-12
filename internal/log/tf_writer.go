package log

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/internal/log/formatter"
	"github.com/sirupsen/logrus"
)

const tfTimestampFormat = "2006-01-02T15:04:05.000-0700"
const prefixFmt = "%s:tf"

var extractTimeAndLevelReg = regexp.MustCompile(`(\S+)\s*\[(\S+)\]\s*(.+\S)`)

func TFStdoutWriter(writer io.Writer, formatter logrus.Formatter, prefix string) io.Writer {
	return &tfWriter{
		formatter: formatter,
		writer:    writer,
		prefix:    prefix,
		isStdout:  true,
	}
}

func TFStderrWriter(writer io.Writer, formatter logrus.Formatter, prefix string) io.Writer {
	return &tfWriter{
		formatter: formatter,
		writer:    writer,
		prefix:    prefix,
		isStdout:  false,
	}
}

type tfWriter struct {
	formatter logrus.Formatter
	writer    io.Writer
	prefix    string
	isStdout  bool
}

func (tf *tfWriter) Write(p []byte) (int, error) {
	var (
		msgs  []byte
		lines = bytes.Split(p, []byte{'\n'})
	)

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		var (
			timeStr, levelStr string
			msg               = string(line)
		)
		if !tf.isStdout {
			timeStr, levelStr, msg = extractTimeAndLevel(msg)
		}

		entry := &logrus.Entry{
			Logger:  &logrus.Logger{Out: tf.writer},
			Time:    time.Now(),
			Data:    make(map[string]any),
			Message: msg,
		}

		if tf.prefix != "" {
			entry.Data[formatter.PrefixKeyName] = fmt.Sprintf(prefixFmt, tf.prefix)
		}

		level, err := logrus.ParseLevel(strings.ToLower(levelStr))
		if err != nil {
			level = formatter.NoneLevel
		}
		entry.Level = level

		if timeStr != "" {
			t, err := time.Parse(tfTimestampFormat, timeStr)
			if err != nil {
				return 0, errors.WithStackTrace(err)
			}
			entry.Time = t
		}

		b, err := tf.formatter.Format(entry)
		if err != nil {
			return 0, err
		}
		msgs = append(msgs, b...)
	}

	if _, err := tf.writer.Write(msgs); err != nil {
		return 0, errors.WithStackTrace(err)
	}
	return len(p), nil
}

func extractTimeAndLevel(msg string) (string, string, string) {
	if extractTimeAndLevelReg.MatchString(msg) {
		match := extractTimeAndLevelReg.FindStringSubmatch(msg)
		return match[1], match[2], match[3]
	}
	return "", "", msg
}
