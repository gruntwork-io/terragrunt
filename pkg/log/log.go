// Package log provides a leveled logger with structured logging support.
package log

var (
	// std is the name of the default logger.
	std = New()
)

// Default returns the standard logger used by the package-level output functions.
// Typically used as the default logger for various packages.
// It is highly recommended not to use it to avoid conflicts in tests.
func Default() Logger {
	return std
}

// Debug logs a message at level Debug on the standard logger.
func Debug(args ...any) {
	std.Debug(args...)
}

// Trace logs a message at level Trace on the standard logger.
func Trace(args ...any) {
	std.Trace(args...)
}

// Info logs a message at level Info on the standard logger.
func Info(args ...any) {
	std.Info(args...)
}

// Print logs a message at level Info on the standard logger.
func Print(args ...any) {
	std.Print(args...)
}

// Warn logs a message at level Warn on the standard logger.
func Warn(args ...any) {
	std.Warn(args...)
}

// Error logs a message at level Error on the standard logger.
func Error(args ...any) {
	std.Error(args...)
}

// Debugln logs a message at level Debug on the standard logger.
func Debugln(args ...any) {
	std.Debugln(args...)
}

// Infoln logs a message at level Info on the standard logger.
func Infoln(args ...any) {
	std.Infoln(args...)
}

// Println logs a message at level Info on the standard logger.
func Println(args ...any) {
	std.Println(args...)
}

// Warnln logs a message at level Warn on the standard logger.
func Warnln(args ...any) {
	std.Warnln(args...)
}

// Errorln logs a message at level Error on the standard logger.
func Errorln(args ...any) {
	std.Errorln(args...)
}

// Debugf logs a message at level Debug on the standard logger.
func Debugf(format string, args ...any) {
	std.Debugf(format, args...)
}

// Tracef logs a message at level Trace on the standard logger.
func Tracef(format string, args ...any) {
	std.Tracef(format, args...)
}

// Infof logs a message at level Info on the standard logger.
func Infof(format string, args ...any) {
	std.Infof(format, args...)
}

// Printf logs a message at level Info on the standard logger.
func Printf(args ...any) {
	std.Print(args...)
}

// Warnf logs a message at level Warn on the standard logger.
func Warnf(format string, args ...any) {
	std.Warnf(format, args...)
}

// Errorf logs a message at level Error on the standard logger.
func Errorf(format string, args ...any) {
	std.Errorf(format, args...)
}

// WithField allocates a new entry and adds a field to it.
func WithField(key string, value interface{}) Logger {
	return std.WithField(key, value)
}

// WithFields adds a struct of fields to the logger. All it does is call `WithField` for each `Field`.
func WithFields(fields Fields) Logger {
	return std.WithFields(fields)
}

// WithError adds an error to log entry, using the value defined in ErrorKey as key.
func WithError(err error) Logger {
	return std.WithError(err)
}

// WithOptions returns a new logger with the given options.
func WithOptions(opts ...Option) Logger {
	return std.WithOptions(opts...)
}

// SetOptions sets the options for the standard logger.
func SetOptions(opts ...Option) {
	std.SetOptions(opts...)
}
