// Package logger provides a convenient logger for tests.
package logger

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
)

// CreateLogger returns a debug-level logger with key-value formatting and colors off.
func CreateLogger() log.Logger {
	formatter := format.NewFormatter(format.NewKeyValueFormatPlaceholders())
	formatter.SetDisabledColors(true)

	return log.New(log.WithLevel(log.DebugLevel), log.WithFormatter(formatter))
}
