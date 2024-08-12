package log

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// WithError adds an error to log entry, using the value defined in ErrorKey as key.
func WithError(err error) *logrus.Entry {
	return logger.WithError(err)
}

// Debug logs a message at level Debug on the standard logger.
func Debug(args ...any) {
	logger.Debug(args...)
}

// Trace logs a message at level Trace on the standard logger.
func Trace(args ...any) {
	logger.Trace(args...)
}

// Info logs a message at level Info on the standard logger.
func Info(args ...any) {
	logger.Info(args...)
}

// Print logs a message at level Info on the standard logger.
func Print(args ...any) {
	logger.Print(args...)
}

// Warn logs a message at level Warn on the standard logger.
func Warn(args ...any) {
	logger.Warn(args...)
}

// Error logs a message at level Error on the standard logger.
func Error(args ...any) {
	logger.Error(args...)
}

// Panic logs a message at level Panic on the standard logger.
func Panic(args ...any) {
	logger.Panic(args...)
}

// Fatal logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatal(args ...any) {
	logger.Fatal(args...)
}

// Debugln logs a message at level Debug on the standard logger.
func Debugln(args ...any) {
	logger.Debugln(args...)
}

// Infoln logs a message at level Info on the standard logger.
func Infoln(args ...any) {
	logger.Infoln(args...)
}

// Println logs a message at level Info on the standard logger.
func Println(args ...any) {
	logger.Println(args...)
}

// Warnln logs a message at level Warn on the standard logger.
func Warnln(args ...any) {
	logger.Warnln(args...)
}

// Errorln logs a message at level Error on the standard logger.
func Errorln(args ...any) {
	logger.Errorln(args...)
}

// Panicln logs a message at level Panic on the standard logger.
func Panicln(args ...any) {
	logger.Panicln(args...)
}

// Fatalln logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatalln(args ...any) {
	logger.Fatalln(args...)
}

// Debugf logs a message at level Debug on the standard logger.
func Debugf(format string, args ...any) {
	logger.Debugf(format, args...)
}

// Tracef logs a message at level Trace on the standard logger.
func Tracef(format string, args ...any) {
	logger.Tracef(format, args...)
}

// Infof logs a message at level Info on the standard logger.
func Infof(format string, args ...any) {
	logger.Infof(format, args...)
}

// Printf logs a message at level Info on the standard logger.
func Printf(args ...any) {
	logger.Print(args...)
}

// Warnf logs a message at level Warn on the standard logger.
func Warnf(format string, args ...any) {
	logger.Warnf(format, args...)
}

// Errorf logs a message at level Error on the standard logger.
func Errorf(format string, args ...any) {
	logger.Errorf(format, args...)
}

// Panicf logs a message at level Panic on the standard logger.
func Panicf(format string, args ...any) {
	logger.Panicf(format, args...)
}

// Fatalf logs a message at level Fatal on the standard logger then the process will exit with status set to 1.
func Fatalf(format string, args ...any) {
	logger.Fatalf(format, args...)
}

func Logf(level logrus.Level, format string, args ...interface{}) {
	if logger.IsLevelEnabled(level) {
		logger.Log(level, fmt.Sprintf(format, args...))
	}
}

// WithField allocates a new entry and adds a field to it.
func WithField(key string, value interface{}) *logrus.Entry {
	return logger.WithField(key, value)

}
