package log

// var (
// 	logger       *logrus.Logger
// 	logLevelLock = sync.Mutex{}
// )

// func SetLogLevel(level logrus.Level) {
// 	// We need to lock here as this function may be called from multiple threads concurrently (e.g. especially at test time)
// 	defer logLevelLock.Unlock()
// 	logLevelLock.Lock()

// 	logger.Level = level
// }

// // Logger returns logger
// func Logger() *logrus.Logger {
// 	return logger
// }

// // Logger returns logger
// func SetLogger(l *logrus.Logger) {
// 	logger = l
// }

// func init() {
// 	logger = logrus.New()
// 	logger.Formatter = formatter.NewTextFormatter()
// }
