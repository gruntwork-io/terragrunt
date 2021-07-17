package util

import (
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
)

const DEFAULT_LOG_LEVEL = logrus.InfoLevel

// GlobalFallbackLogEntry is a global fallback logentry for the application
// Should be used in cases when more specific logger can't be created (like in the very beginning, when we have not yet
// parsed command line arguments).
//
// This might go away once we migrate toproper cli library
// (see https://github.com/gruntwork-io/terragrunt/blob/master/cli/args.go#L29)
var GlobalFallbackLogEntry *logrus.Entry

func init() {
	GlobalFallbackLogEntry = CreateLogEntry("", DEFAULT_LOG_LEVEL)
}

// CreateLogger creates a logger. If debug is set, we use ErrorLevel to enable verbose output, otherwise - only errors are shown
func CreateLogger(lvl logrus.Level) *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(lvl)
	logger.SetOutput(os.Stderr) //Terragrunt should output all it's logs to stderr by default
	logger.SetFormatter(&logrus.TextFormatter{
		DisableQuote: true,
	})
	return logger
}

// CreateLogEntry creates a logger entry with the given prefix field
func CreateLogEntry(prefix string, level logrus.Level) *logrus.Entry {
	logger := CreateLogger(level)
	var fields logrus.Fields
	if prefix != "" {
		fields = logrus.Fields{"prefix": prefix}
	} else {
		fields = logrus.Fields{}
	}
	return logger.WithFields(fields)
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
