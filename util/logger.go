package util

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"golang.org/x/crypto/ssh/terminal"
)

// Create a logger with the given prefix
func CreateLogger(prefix string) *log.Logger {
	return CreateLoggerWithWriter(os.Stderr, prefix)
}

// CreateLoggerWithWriter Create a logger around the given output stream and prefix
func CreateLoggerWithWriter(writer io.Writer, prefix string) *log.Logger {
	if prefix != "" {
		prefix = fmt.Sprintf("[%s] ", prefix)
	}
	return log.New(writer, fmt.Sprintf("[terragrunt] %s", prefix), log.LstdFlags)
}

// MAINTAINER'S NOTE: This is a temporary solution for logging levels in terragrunt. This is not a permanent debug
// logging solution.
// Debugf will only print out terragrunt logs if the TG_LOG environment variable is set to DEBUG.
func Debugf(logger *log.Logger, fmtString string, fmtArgs ...interface{}) {
	if strings.ToLower(os.Getenv("TG_LOG")) == "debug" {
		logger.Printf(fmtString, fmtArgs...)
	}
}

// ColorLogf
func ColorLogf(logger *log.Logger, colorCode *color.Color, fmtString string, fmtArgs ...interface{}) {
	logOut := fmt.Sprintf(fmtString, fmtArgs...)

	allowColor := terminal.IsTerminal(int(os.Stderr.Fd()))
	if allowColor {
		logOut = colorCode.SprintFunc()(logOut)
	}
	logger.Println(logOut)
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
