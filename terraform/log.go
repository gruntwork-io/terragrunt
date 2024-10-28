package terraform

import (
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/writer"
)

const parseLogNumberOfValues = 4

var (
	// logTimestampFormat is TF_LOG timestamp formats.
	logTimestampFormat = "2006-01-02T15:04:05.000Z0700"
)

var (
	// tfLogTimeLevelMsgReg is a regular expression that matches TF_LOG output, example output:
	//
	// 2024-09-08T13:44:31.229+0300 [DEBUG] using github.com/zclconf/go-cty v1.14.3
	// 2024-09-08T13:44:31.229+0300 [INFO]  Go runtime version: go1.22.1
	tfLogTimeLevelMsgReg = regexp.MustCompile(`(?i)(^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}\S*)\s*\[(trace|debug|warn|info|error)\]\s*(.+\S)$`)
)

// ParseLogFunc wraps `ParseLog` to add msg prefix and bypasses the parse error if `returnError` is false,
// since returning the error for `log/writer` will cause TG to fall with a `broken pipe` error.
func ParseLogFunc(msgPrefix string, returnError bool) writer.WriterParseFunc {
	return func(str string) (msg string, ptrTime *time.Time, ptrLevel *log.Level, err error) {
		if msg, ptrTime, ptrLevel, err = ParseLog(str); err != nil {
			if returnError {
				return str, nil, nil, err
			}

			return str, nil, nil, nil
		}

		return msgPrefix + msg, ptrTime, ptrLevel, nil
	}
}

func ParseLog(str string) (msg string, ptrTime *time.Time, ptrLevel *log.Level, err error) {
	if !tfLogTimeLevelMsgReg.MatchString(str) {
		return str, nil, nil, errors.Errorf("could not parse string %q: does not match a known format", str)
	}

	match := tfLogTimeLevelMsgReg.FindStringSubmatch(str)
	if len(match) != parseLogNumberOfValues {
		return str, nil, nil, errors.Errorf("could not parse string %q: does not match a known format", str)
	}

	timeStr, levelStr, msg := match[1], match[2], match[3]

	if levelStr != "" {
		level, err := log.ParseLevel(strings.ToLower(levelStr))
		if err != nil {
			return str, nil, nil, errors.Errorf("could not parse level %q: %w", levelStr, err)
		}

		ptrLevel = &level
	}

	if timeStr != "" {
		time, err := time.Parse(logTimestampFormat, timeStr)
		if err != nil {
			return str, nil, nil, errors.Errorf("could not parse time %q: %w", timeStr, err)
		}

		ptrTime = &time
	}

	return msg, ptrTime, ptrLevel, nil
}
