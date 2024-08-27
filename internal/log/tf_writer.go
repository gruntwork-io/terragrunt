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
	"github.com/mgutz/ansi"
	"github.com/sirupsen/logrus"
)

const tfTimestampFormat = "2006-01-02T15:04:05.000-0700"

var extractTimeAndLevelReg = regexp.MustCompile(`(\S+)\s*\[(\S+)\]\s*(.+\S)`)

type tfWriter struct {
	io.Writer
	formatter logrus.Formatter
	prefix    string
	tfPath    string
	isStdout  bool
}

func TFStdoutWriter(writer io.Writer, formatter logrus.Formatter, prefix, tfpath string) io.Writer {
	return &tfWriter{
		Writer:    writer,
		formatter: formatter,
		prefix:    prefix,
		tfPath:    tfpath,
		isStdout:  true,
	}
}

func TFStderrWriter(writer io.Writer, formatter logrus.Formatter, prefix, tfpath string) io.Writer {
	return &tfWriter{
		Writer:    writer,
		formatter: formatter,
		prefix:    prefix,
		tfPath:    tfpath,
		isStdout:  false,
	}
}

func (writer *tfWriter) Write(p []byte) (int, error) {
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

		if !writer.isStdout {
			timeStr, levelStr, msg = extractTimeAndLevel(msg)
		}

		entry := &logrus.Entry{
			Logger:  &logrus.Logger{Out: writer.Writer},
			Time:    time.Now(),
			Data:    make(map[string]any),
			Message: msg,
		}

		if writer.prefix != "" {
			entry.Data[formatter.PrefixKeyName] = writer.prefix
		}

		if writer.tfPath != "" {
			entry.Data[formatter.TFBinaryKeyName] = filepath.Base(writer.tfPath)
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

		b, err := writer.formatter.Format(entry)
		if err != nil {
			return 0, err
		}

		msgs = append(msgs, b...)
		msgs = append(msgs, []byte(ansi.Reset)...)
	}

	if _, err := writer.Writer.Write(msgs); err != nil {
		return 0, errors.WithStackTrace(err)
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
