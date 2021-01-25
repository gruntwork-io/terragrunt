package util

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
)

// CreateLogger creates a logger. If debug is set, we use ErrorLevel to enable verbose output, otherwise - only errors are shown
func CreateLogger(lvl string) *logrus.Logger {
	logger := logrus.New()
	level, err := logrus.ParseLevel(lvl)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	logger.SetLevel(level)
	return logger
}

// CreateLogEntry creates a logger entry with the given prefix field
func CreateLogEntry(prefix string, level string) *logrus.Entry {
	logger := CreateLogger(level)
	var fields logrus.Fields
	if prefix != "" {
		fields = logrus.Fields{"prefix": prefix}
	} else {
		fields = logrus.Fields{}
	}
	return logger.WithFields(fields)
}

// CreateLoggerWithWriter Create a logger around the given output stream and prefix
func CreateLogEntryWithWriter(writer io.Writer, prefix string, level string) *logrus.Entry {
	if prefix != "" {
		prefix = fmt.Sprintf("[%s] ", prefix)
	} else {
		prefix = fmt.Sprintf("[terragrunt] %s", prefix)
	}
	logger := CreateLogEntry(prefix, level)
	logger.Logger.SetOutput(writer)
	return logger
}

// ColorLogf
func ColorLogf(logger *logrus.Entry, colorCode *color.Color, fmtString string, fmtArgs ...interface{}) {
	logOut := fmt.Sprintf(fmtString, fmtArgs...)

	allowColor := terminal.IsTerminal(int(os.Stderr.Fd()))
	if allowColor {
		logOut = colorCode.SprintFunc()(logOut)
	}
	logger.Errorf(logOut)
}

// GetDiagnosticsWriter returns a hcl2 parsing diagnostics emitter for the current terminal.
func GetDiagnosticsWriter(parser *hclparse.Parser) hcl.DiagnosticWriter {
	termColor := terminal.IsTerminal(int(os.Stderr.Fd()))
	termWidth, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		termWidth = 80
	}
	return hcl.NewDiagnosticTextWriter(os.Stderr, parser.Files(), uint(termWidth), termColor)
}
