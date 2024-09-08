package terraform

import (
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	// logTimestampFormat is TF_LOG timestamp format
	logTimestampFormat = "2006-01-02T15:04:05.000-0700"
)

var tfLogTimeLevelMsgReg = regexp.MustCompile(`(?i)(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}[-+]\d{4})\s*\[(trace|debug|warn|info|error)\]\s*(.+\S)`)

func ParseLog(str string) (msg string, ptrTime *time.Time, ptrLevel *log.Level, err error) {
	const numberOfValues = 4

	if !tfLogTimeLevelMsgReg.MatchString(str) {
		return str, nil, nil, nil
	}

	match := tfLogTimeLevelMsgReg.FindStringSubmatch(str)
	if len(match) != numberOfValues {
		return str, nil, nil, nil
	}

	timeStr, levelStr, msg := match[1], match[2], match[3]

	if timeStr != "" {
		time, err := time.Parse(logTimestampFormat, timeStr)
		if err != nil {
			return "", nil, nil, errors.WithStackTrace(err)
		}

		ptrTime = &time
	}

	if levelStr != "" {
		level, err := log.ParseLevel(strings.ToLower(levelStr))
		if err != nil {
			return "", nil, nil, errors.WithStackTrace(err)
		}

		ptrLevel = &level
	}

	return "TF_LOG " + msg, ptrTime, ptrLevel, nil

}
