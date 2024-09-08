package log

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type Formatters []Formatter

func (formatters Formatters) Names() []string {
	strs := make([]string, len(formatters))

	for i, formatter := range formatters {
		strs[i] = formatter.Name()
	}

	return strs
}

func (formatters Formatters) String() string {
	return strings.Join(formatters.Names(), ", ")
}

type Formatter interface {
	logrus.Formatter

	Name() string
	SetOption(name string, value any) error
	SupportedOption() []string
}

type Logger interface {
	//
	Clone() Logger

	//
	SetOptions(opts ...Option)

	//
	WithOptions(opts ...Option) Logger

	// WithField adds a single field to the Logger.
	WithField(key string, value any) Logger

	// WithFields adds a struct of fields to the log entry. All it does is call `WithField` for each `Field`.
	WithFields(fields Fields) Logger

	// WithError adds an error as single field to the log entry.  All it does is call `WithError` for the given `error`.
	WithError(err error) Logger

	// WithContext adds a context to the log entry.
	WithContext(ctx context.Context) Logger

	// WithTime overrides the time of the log entry.
	WithTime(t time.Time) Logger

	//
	Writer() *io.PipeWriter

	//
	WriterLevel(Level) *io.PipeWriter

	Logf(level Level, format string, args ...any)
	Tracef(format string, args ...any)
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Printf(format string, args ...any)
	Warnf(format string, args ...any)
	Warningf(format string, args ...any)
	Errorf(format string, args ...any)

	Log(level Level, args ...any)
	Trace(args ...any)
	Debug(args ...any)
	Info(args ...any)
	Print(args ...any)
	Warn(args ...any)
	Warning(args ...any)
	Error(args ...any)

	Logln(level Level, args ...any)
	Traceln(args ...any)
	Debugln(args ...any)
	Infoln(args ...any)
	Println(args ...any)
	Warnln(args ...any)
	Warningln(args ...any)
	Errorln(args ...any)
}

type logger struct {
	*logrus.Entry
}

func New(opts ...Option) Logger {
	logger := &logger{
		Entry: logrus.NewEntry(logrus.New()),
	}
	logger.SetOptions(opts...)

	return logger
}

// Clone implements Logger.Clone
func (logger *logger) Clone() Logger {
	return logger.clone()
}

// SetOptions implements Logger.SetOptions
func (logger *logger) SetOptions(opts ...Option) {
	if len(opts) == 0 {
		return
	}

	for _, opt := range opts {
		opt(logger)
	}
}

// WithOptions implements Logger.WithOptions
func (logger *logger) WithOptions(opts ...Option) Logger {
	if len(opts) == 0 {
		return logger
	}

	logger = logger.clone()
	logger.SetOptions(opts...)

	return logger
}

func (logger *logger) WriterLevel(level Level) *io.PipeWriter {
	return logger.Logger.WriterLevel(level.ToLogrusLevel())
}

func (logger *logger) WithField(key string, value any) Logger {
	return logger.WithFields(Fields(Fields{key: value}))
}

func (logger *logger) WithFields(fields Fields) Logger {
	return logger.setEntry(logger.Entry.WithFields(logrus.Fields(fields)))
}

func (logger *logger) WithError(err error) Logger {
	return logger.setEntry(logger.Entry.WithError(err))
}

func (logger *logger) WithContext(ctx context.Context) Logger {
	return logger.setEntry(logger.Entry.WithContext(ctx))
}

func (logger *logger) WithTime(t time.Time) Logger {
	return logger.setEntry(logger.Entry.WithTime(t))
}

func (logger *logger) Logf(level Level, format string, args ...any) {
	logger.Entry.Logf(level.ToLogrusLevel(), format, args...)
}

func (logger *logger) Log(level Level, args ...any) {
	logger.Entry.Log(level.ToLogrusLevel(), args...)
}

func (logger *logger) Logln(level Level, args ...any) {
	logger.Entry.Logln(level.ToLogrusLevel(), args...)
}

func (logger *logger) Trace(args ...interface{}) {
	logger.Log(TraceLevel, args...)
}

func (logger *logger) Debug(args ...interface{}) {
	logger.Log(DebugLevel, args...)
}

func (logger *logger) Print(args ...interface{}) {
	logger.Info(args...)
}

func (logger *logger) Info(args ...interface{}) {
	logger.Log(InfoLevel, args...)
}

func (logger *logger) Warn(args ...interface{}) {
	logger.Log(WarnLevel, args...)
}

func (logger *logger) Warning(args ...interface{}) {
	logger.Warn(args...)
}

func (logger *logger) Error(args ...interface{}) {
	logger.Log(ErrorLevel, args...)
}

// Entry Printf family functions

func (logger *logger) Tracef(format string, args ...interface{}) {
	logger.Logf(TraceLevel, format, args...)
}

func (logger *logger) Debugf(format string, args ...interface{}) {
	logger.Logf(DebugLevel, format, args...)
}

func (logger *logger) Infof(format string, args ...interface{}) {
	logger.Logf(InfoLevel, format, args...)
}

func (logger *logger) Printf(format string, args ...interface{}) {
	logger.Infof(format, args...)
}

func (logger *logger) Warnf(format string, args ...interface{}) {
	logger.Logf(WarnLevel, format, args...)
}

func (logger *logger) Warningf(format string, args ...interface{}) {
	logger.Warnf(format, args...)
}

func (logger *logger) Errorf(format string, args ...interface{}) {
	logger.Logf(ErrorLevel, format, args...)
}

// Entry Println family functions

func (logger *logger) Traceln(args ...interface{}) {
	logger.Logln(TraceLevel, args...)
}

func (logger *logger) Debugln(args ...interface{}) {
	logger.Logln(DebugLevel, args...)
}

func (logger *logger) Infoln(args ...interface{}) {
	logger.Logln(InfoLevel, args...)
}

func (logger *logger) Println(args ...interface{}) {
	logger.Infoln(args...)
}

func (logger *logger) Warnln(args ...interface{}) {
	logger.Logln(WarnLevel, args...)
}

func (logger *logger) Warningln(args ...interface{}) {
	logger.Warnln(args...)
}

func (logger *logger) Errorln(args ...interface{}) {
	logger.Logln(ErrorLevel, args...)
}

func (logger logger) setEntry(entry *logrus.Entry) *logger {
	logger.Entry = entry
	return &logger
}

func (logger logger) clone() *logger {
	parentLogger := logger.Logger

	logger.Logger = logrus.New()
	logger.Logger.SetOutput(parentLogger.Out)
	logger.Logger.SetLevel(parentLogger.Level)
	logger.Logger.SetFormatter(parentLogger.Formatter)
	logger.Logger.ReplaceHooks(parentLogger.Hooks)
	logger.Entry = logger.Entry.Dup()

	return &logger
}
