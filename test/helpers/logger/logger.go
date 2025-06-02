// Package logger provides a convenient logger for tests.
package logger

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
)

func CreateLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}
