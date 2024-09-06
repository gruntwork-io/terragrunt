package log

var DefaultLogger Logger

func init() {
	DefaultLogger = New()
}

// Debug logs a message at level Debug on the standard logger.
func Debug(args ...any) {
	DefaultLogger.Debug(args...)
}

// Trace logs a message at level Trace on the standard logger.
func Trace(args ...any) {
	DefaultLogger.Trace(args...)
}

// Info logs a message at level Info on the standard logger.
func Info(args ...any) {
	DefaultLogger.Info(args...)
}

// Print logs a message at level Info on the standard logger.
func Print(args ...any) {
	DefaultLogger.Print(args...)
}

// Warn logs a message at level Warn on the standard logger.
func Warn(args ...any) {
	DefaultLogger.Warn(args...)
}

// Error logs a message at level Error on the standard logger.
func Error(args ...any) {
	DefaultLogger.Error(args...)
}

// Debugln logs a message at level Debug on the standard logger.
func Debugln(args ...any) {
	DefaultLogger.Debugln(args...)
}

// Infoln logs a message at level Info on the standard logger.
func Infoln(args ...any) {
	DefaultLogger.Infoln(args...)
}

// Println logs a message at level Info on the standard logger.
func Println(args ...any) {
	DefaultLogger.Println(args...)
}

// Warnln logs a message at level Warn on the standard logger.
func Warnln(args ...any) {
	DefaultLogger.Warnln(args...)
}

// Errorln logs a message at level Error on the standard logger.
func Errorln(args ...any) {
	DefaultLogger.Errorln(args...)
}

// Debugf logs a message at level Debug on the standard logger.
func Debugf(format string, args ...any) {
	DefaultLogger.Debugf(format, args...)
}

// Tracef logs a message at level Trace on the standard logger.
func Tracef(format string, args ...any) {
	DefaultLogger.Tracef(format, args...)
}

// Infof logs a message at level Info on the standard logger.
func Infof(format string, args ...any) {
	DefaultLogger.Infof(format, args...)
}

// Printf logs a message at level Info on the standard logger.
func Printf(args ...any) {
	DefaultLogger.Print(args...)
}

// Warnf logs a message at level Warn on the standard logger.
func Warnf(format string, args ...any) {
	DefaultLogger.Warnf(format, args...)
}

// Errorf logs a message at level Error on the standard logger.
func Errorf(format string, args ...any) {
	DefaultLogger.Errorf(format, args...)
}

// WithField allocates a new entry and adds a field to it.
func WithField(key string, value interface{}) Logger {
	return DefaultLogger.WithField(key, value)
}

// WithFields adds a struct of fields to the logger. All it does is call `WithField` for each `Field`.
func WithFields(fields Fields) Logger {
	return DefaultLogger.WithFields(fields)
}

// WithError adds an error to log entry, using the value defined in ErrorKey as key.
func WithError(err error) Logger {
	return DefaultLogger.WithError(err)
}

// WithOptions
func WithOptions(opts ...Option) Logger {
	return DefaultLogger.WithOptions(opts...)
}

// SetOptions
func SetOptions(opts ...Option) {
	DefaultLogger.SetOptions(opts...)
}
