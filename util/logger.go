package util

import (
	"fmt"
	"io"
	"os"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh/terminal"
)

const defaultLogLevel = logrus.InfoLevel
const logLevelEnvVar = "TERRAGRUNT_LOG_LEVEL"

// GlobalFallbackLogEntry is a global fallback logentry for the application
// Should be used in cases when more specific logger can't be created (like in the very beginning, when we have not yet
// parsed command line arguments).
//
// This might go away once we migrate toproper cli library
// (see https://github.com/gruntwork-io/terragrunt/blob/master/cli/args.go#L29)
var GlobalFallbackLogEntry *logrus.Entry

func init() {
	defaultLogLevel := GetDefaultLogLevel()
	GlobalFallbackLogEntry = CreateLogEntry("", defaultLogLevel)
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

// CreateLoggerWithWriter Create a logger around the given output stream and prefix
func CreateLogEntryWithWriter(writer io.Writer, prefix string, level logrus.Level, hooks logrus.LevelHooks) *logrus.Entry {
	if prefix != "" {
		prefix = fmt.Sprintf("[%s] ", prefix)
	} else {
		prefix = fmt.Sprintf("[terragrunt] %s", prefix)
	}
	logger := CreateLogEntry(prefix, level)
	logger.Logger.SetOutput(writer)
	logger.Logger.ReplaceHooks(hooks)
	return logger
}

// GetDiagnosticsWriter returns a hcl2 parsing diagnostics emitter for the current terminal.
func GetDiagnosticsWriter(logger *logrus.Entry, parser *hclparse.Parser) hcl.DiagnosticWriter {
	termColor := terminal.IsTerminal(int(os.Stderr.Fd()))
	termWidth, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		termWidth = 80
	}
	var writer = LogWriter{Logger: logger, Level: logrus.ErrorLevel}
	return hcl.NewDiagnosticTextWriter(&writer, parser.Files(), uint(termWidth), termColor)
}

// GetDefaultLogLevel returns the default log level to use. The log level is resolved based on the environment variable
// with name from LogLevelEnvVar, falling back to info if unspecified or there is an error parsing the given log level.
func GetDefaultLogLevel() logrus.Level {
	defaultLogLevelStr := os.Getenv(logLevelEnvVar)
	if defaultLogLevelStr == "" {
		return defaultLogLevel
	}

	parsedLogLevel, err := logrus.ParseLevel(defaultLogLevelStr)
	if err != nil {
		CreateLogEntry("", defaultLogLevel).Errorf(
			"Could not parse log level from environment variable %s (%s) - falling back to default %s",
			logLevelEnvVar,
			defaultLogLevelStr,
			defaultLogLevel,
		)
		return defaultLogLevel
	}
	return parsedLogLevel
}

// LogWriter - Writer implementation which redirect Write requests to configured logger and level
type LogWriter struct {
	Logger *logrus.Entry
	Level  logrus.Level
}

func (w *LogWriter) Write(p []byte) (n int, err error) {
	w.Logger.Log(w.Level, string(p))
	return len(p), nil
}
