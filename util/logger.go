package util

import (
	"io"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/gruntwork-io/terragrunt/internal/log/formatter"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/sirupsen/logrus"
)

// used in integration tests
const (
	defaultLogLevel        = logrus.InfoLevel
	defaultTimestampFormat = time.RFC3339

	logLevelEnvVar        = "TERRAGRUNT_LOG_LEVEL"
	timestampFormatEnvVar = "TERRAGRUNT_LOG_TIMESTAMP_FORMAT"
)

var (
	// GlobalFallbackLogEntry is a global fallback logentry for the application
	// Should be used in cases when more specific logger can't be created (like in the very beginning, when we have not yet
	// parsed command line arguments).
	//
	// This might go away once we migrate toproper cli library
	// (see https://github.com/gruntwork-io/terragrunt/blob/master/cli/args.go#L29)
	GlobalFallbackLogEntry *logrus.Entry

	globalDisableColors     bool
	globalDisableFormatting bool
	jsonLogFormat           bool
)

func init() {
	setGlobalFallbackLogEntry()
}

func setGlobalFallbackLogEntry() {
	logLevel := GetDefaultLogLevel()
	GlobalFallbackLogEntry = CreateLogEntry("", logLevel, nil, globalDisableColors, globalDisableFormatting)
}

func DisableLogColors() {
	globalDisableColors = true

	setGlobalFallbackLogEntry()
}

func DisableLogFormatting() {
	globalDisableFormatting = true

	setGlobalFallbackLogEntry()
}

func JsonFormat() {
	jsonLogFormat = true

	setGlobalFallbackLogEntry()
}

func DisableJsonFormat() {
	jsonLogFormat = false

	setGlobalFallbackLogEntry()
}

// CreateLogger creates a logger. If debug is set, we use ErrorLevel to enable verbose output, otherwise - only errors are shown
func CreateLogger(lvl logrus.Level, prefixStyle formatter.PrefixStyle, disableColors, disableFormatting bool) *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(lvl)
	logger.SetOutput(os.Stderr) // Terragrunt should output all it's logs to stderr by default

	if jsonLogFormat {
		logger.SetFormatter(&logrus.JSONFormatter{})
	} else {
		logFormatter := formatter.NewFormatter()
		logFormatter.DisableColors = disableColors
		logFormatter.DisableLogFormatting = disableFormatting

		if prefixStyle != nil {
			logFormatter.PrefixStyle = prefixStyle
		}

		if timestampFormat := os.Getenv(timestampFormatEnvVar); timestampFormat != "" {
			logFormatter.TimestampFormat = timestampFormat
		}

		logger.SetFormatter(logFormatter)
	}

	return logger
}

// CreateLogEntry creates a logger entry with the given prefix field
func CreateLogEntry(prefix string, level logrus.Level, prefixStyle formatter.PrefixStyle, disableColors, disableFormatting bool) *logrus.Entry {
	logger := CreateLogger(level, prefixStyle, disableColors, disableFormatting)
	fields := logrus.Fields{
		formatter.PrefixKeyName: prefix,
	}

	return logger.WithFields(fields)
}

// CreateLogEntryWithWriter Create a logger around the given output stream and prefix
func CreateLogEntryWithWriter(writer io.Writer, prefix string, level logrus.Level, hooks logrus.LevelHooks, prefixStyle formatter.PrefixStyle, disableColors, disableFormatting bool) *logrus.Entry {
	logger := CreateLogEntry(prefix, level, prefixStyle, disableColors, disableFormatting)
	logger.Logger.SetOutput(writer)
	logger.Logger.ReplaceHooks(hooks)

	return logger
}

// GetDiagnosticsWriter returns a hcl2 parsing diagnostics emitter for the current terminal.
func GetDiagnosticsWriter(writer io.Writer, parser *hclparse.Parser, disableColor bool) hcl.DiagnosticWriter {
	termColor := !disableColor && term.IsTerminal(int(os.Stderr.Fd()))

	termWidth, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		termWidth = 80
	}

	return hcl.NewDiagnosticTextWriter(writer, parser.Files(), uint(termWidth), termColor)
}

// GetDefaultLogLevel returns the default log level to use. The log level is resolved based on the environment variable
// with name from LogLevelEnvVar, falling back to info if unspecified or there is an error parsing the given log level.
func GetDefaultLogLevel() logrus.Level {
	defaultLogLevelStr := os.Getenv(logLevelEnvVar)
	if defaultLogLevelStr == "" {
		return defaultLogLevel
	}

	return ParseLogLevel(defaultLogLevelStr)
}

func ParseLogLevel(logLevelStr string) logrus.Level {
	parsedLogLevel, err := logrus.ParseLevel(logLevelStr)
	if err != nil {
		CreateLogEntry("", defaultLogLevel, nil, globalDisableColors, globalDisableFormatting).Errorf(
			"Could not parse log level from environment variable %s (%s) - falling back to default %s",
			logLevelEnvVar,
			logLevelStr,
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
