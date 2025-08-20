package log

import (
	"context"
	"io"
	"time"

	"github.com/sirupsen/logrus"
)

// Logger wraps the logrus package to have full control over implementing the required functionality,
// such as adding or removing log levels etc. This also provides developers with an easier way to clone and set parameters.
type Logger interface {
	// Clone creates a new Logger instance with a copy of the fields from the current one.
	Clone() Logger

	// SetOptions sets the given options to the instance.
	SetOptions(opts ...Option)

	// Level returns log level.
	Level() Level

	// SetLevel parses and sets log level.
	SetLevel(str string) error

	// SetFormatter sets the logger formatter.
	SetFormatter(formatter Formatter)

	// Formatter returns the logger formatter.
	Formatter() Formatter

	// WithOptions clones and sets the given options for the new instance.
	// In other words, it is a combination of two methods, `log.Clone().SetOptions(...)`, but
	// unlike `SetOptions(...)`, it returns the instance, which is convenient for further actions.
	WithOptions(opts ...Option) Logger

	// WithField adds a single field to the Logger and returns partly cloning instance, the `Entry` structure.
	// This way the field is added to the returned instance only.
	WithField(key string, value any) Logger

	// WithFields adds a struct of fields to the Logger. All it does is call `WithField` for each `Field`.
	WithFields(fields Fields) Logger

	// WithError adds an error as single field to the Logger. The error is added to the returned instance only.
	WithError(err error) Logger

	// WithContext adds a context to the Logger. The context is added to the returned instance only.
	WithContext(ctx context.Context) Logger

	// WithTime overrides the time of the Logger. This only affects the returned instance.
	WithTime(t time.Time) Logger

	// Writer returns an io.Writer that writes to the Logger at the info log level.
	Writer() *io.PipeWriter

	// WriterLevel returns an io.Writer that writes to the Logger at the given log level.
	WriterLevel(level Level) *io.PipeWriter

	// Logf logs a message at the level given as parameter on the Logger.
	Logf(level Level, format string, args ...any)

	// Tracef logs a message at level Trace on the Logger.
	Tracef(format string, args ...any)

	// Debugf logs a message at level Debug on the Logger.
	Debugf(format string, args ...any)

	// Infof logs a message at level Info on the Logger.
	Infof(format string, args ...any)

	// Printf logs a message at level Info on the Logger.
	Printf(format string, args ...any)

	// Warnf logs a message at level Warn on the Logger.
	Warnf(format string, args ...any)

	// Errorf logs a message at level Error on the Logger.
	Errorf(format string, args ...any)

	// Log logs a message at the level given as parameter on the Logger.
	Log(level Level, args ...any)

	// Trace logs a message at level Trace on the Logger.
	Trace(args ...any)

	// Debug logs a message at level Debug on the Logger.
	Debug(args ...any)

	// Info logs a message at level Info on the Logger.
	Info(args ...any)

	// Print logs a message at level Info on the Logger.
	Print(args ...any)

	// Warn logs a message at level Warn on the Logger.
	Warn(args ...any)

	// Error logs a message at level Error on the Logger.
	Error(args ...any)

	// Logln logs a message at the level given as parameter on the Logger.
	Logln(level Level, args ...any)

	// Traceln logs a message at level Trace on the Logger.
	Traceln(args ...any)

	// Debugln logs a message at level Debug on the Logger.
	Debugln(args ...any)

	// Infoln logs a message at level Info on the Logger.
	Infoln(args ...any)

	// Println logs a message at level Info on the Logger.
	Println(args ...any)

	// Warnln logs a message at level Warn on the Logger.
	Warnln(args ...any)

	// Errorln logs a message at level Error on the Logger.
	Errorln(args ...any)
}

type logger struct {
	*logrus.Entry
	formatter Formatter
}

// New returns a new Logger instance.
func New(opts ...Option) Logger {
	logger := &logger{
		Entry: logrus.NewEntry(logrus.New()),
	}
	logger.SetOptions(opts...)

	return logger
}

// Clone implements the Logger interface method.
func (logger *logger) Clone() Logger {
	return logger.clone()
}

// SetOptions implements the Logger interface method.
func (logger *logger) SetOptions(opts ...Option) {
	if len(opts) == 0 {
		return
	}

	for _, opt := range opts {
		opt(logger)
	}
}

// SetFormatter sets the logger formatter.
func (logger *logger) SetFormatter(formatter Formatter) {
	logger.formatter = formatter
	logger.Logger.SetFormatter(&fromLogrusFormatter{Formatter: formatter})
}

// SetFormatter returns the logger formatter.
func (logger *logger) Formatter() Formatter {
	return logger.formatter
}

// WithOptions implements the Logger interface method.
func (logger *logger) WithOptions(opts ...Option) Logger {
	if len(opts) == 0 {
		return logger
	}

	logger = logger.clone()
	logger.SetOptions(opts...)

	return logger
}

// Level returns log level.
func (logger *logger) Level() Level {
	return FromLogrusLevel(logger.Logger.Level)
}

// SetLevel parses and sets log level.
func (logger *logger) SetLevel(str string) error {
	level, err := ParseLevel(str)
	if err != nil {
		return err
	}

	logger.Logger.SetLevel(level.ToLogrusLevel())

	return nil
}

// WriterLevel implements the Logger interface method.
func (logger *logger) WriterLevel(level Level) *io.PipeWriter {
	return logger.Logger.WriterLevel(level.ToLogrusLevel())
}

// // WithField implements the Logger interface method.
func (logger *logger) WithField(key string, value any) Logger {
	return logger.WithFields(Fields{key: value})
}

// WithFields implements the Logger interface method.
func (logger *logger) WithFields(fields Fields) Logger {
	return logger.setEntry(logger.Entry.WithFields(logrus.Fields(fields)))
}

// WithError implements the Logger interface method.
func (logger *logger) WithError(err error) Logger {
	return logger.setEntry(logger.Entry.WithError(err))
}

// WithContext implements the Logger interface method.
func (logger *logger) WithContext(ctx context.Context) Logger {
	return logger.setEntry(logger.Entry.WithContext(ctx))
}

// WithTime implements the Logger interface method.
func (logger *logger) WithTime(t time.Time) Logger {
	return logger.setEntry(logger.Entry.WithTime(t))
}

// Logf implements the Logger interface method.
func (logger *logger) Logf(level Level, format string, args ...any) {
	logger.Entry.Logf(level.ToLogrusLevel(), format, args...)
}

// Log implements the Logger interface method.
func (logger *logger) Log(level Level, args ...any) {
	logger.Entry.Log(level.ToLogrusLevel(), args...)
}

// Logln implements the Logger interface method.
func (logger *logger) Logln(level Level, args ...any) {
	logger.Entry.Logln(level.ToLogrusLevel(), args...)
}

// Trace implements the Logger interface method.
func (logger *logger) Trace(args ...any) {
	logger.Log(TraceLevel, args...)
}

// Debug implements the Logger interface method.
func (logger *logger) Debug(args ...any) {
	logger.Log(DebugLevel, args...)
}

// Print implements the Logger interface method.
func (logger *logger) Print(args ...any) {
	logger.Info(args...)
}

// Info implements the Logger interface method.
func (logger *logger) Info(args ...any) {
	logger.Log(InfoLevel, args...)
}

// Warn implements the Logger interface method.
func (logger *logger) Warn(args ...any) {
	logger.Log(WarnLevel, args...)
}

// Error implements the Logger interface method.
func (logger *logger) Error(args ...any) {
	logger.Log(ErrorLevel, args...)
}

// Entry Printf family functions.

// Tracef implements the Logger interface method.
func (logger *logger) Tracef(format string, args ...any) {
	logger.Logf(TraceLevel, format, args...)
}

// Debugf implements the Logger interface method.
func (logger *logger) Debugf(format string, args ...any) {
	logger.Logf(DebugLevel, format, args...)
}

// Infof implements the Logger interface method.
func (logger *logger) Infof(format string, args ...any) {
	logger.Logf(InfoLevel, format, args...)
}

// Printf implements the Logger interface method.
func (logger *logger) Printf(format string, args ...any) {
	logger.Infof(format, args...)
}

// Warnf implements the Logger interface method.
func (logger *logger) Warnf(format string, args ...any) {
	logger.Logf(WarnLevel, format, args...)
}

// Errorf implements the Logger interface method.
func (logger *logger) Errorf(format string, args ...any) {
	logger.Logf(ErrorLevel, format, args...)
}

// Entry Println family functions

// Traceln implements the Logger interface method.
func (logger *logger) Traceln(args ...any) {
	logger.Logln(TraceLevel, args...)
}

// Debugln implements the Logger interface method.
func (logger *logger) Debugln(args ...any) {
	logger.Logln(DebugLevel, args...)
}

// Infoln implements the Logger interface method.
func (logger *logger) Infoln(args ...any) {
	logger.Logln(InfoLevel, args...)
}

// Println implements the Logger interface method.
func (logger *logger) Println(args ...any) {
	logger.Infoln(args...)
}

// Warnln implements the Logger interface method.
func (logger *logger) Warnln(args ...any) {
	logger.Logln(WarnLevel, args...)
}

// Errorln implements the Logger interface method.
func (logger *logger) Errorln(args ...any) {
	logger.Logln(ErrorLevel, args...)
}

func (logger *logger) setEntry(entry *logrus.Entry) *logger {
	newLogger := *logger
	newLogger.Entry = entry

	return &newLogger
}

func (logger *logger) clone() *logger {
	newLogger := *logger

	parentLogger := newLogger.Logger

	newLogger.Logger = logrus.New()
	newLogger.Logger.SetOutput(parentLogger.Out)
	newLogger.Logger.SetLevel(parentLogger.Level)
	newLogger.Logger.SetFormatter(parentLogger.Formatter)
	newLogger.Logger.ReplaceHooks(parentLogger.Hooks)
	newLogger.Entry = newLogger.Dup()

	return &newLogger
}
