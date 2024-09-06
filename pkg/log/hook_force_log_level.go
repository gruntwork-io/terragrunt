package log

import "github.com/sirupsen/logrus"

// ForceLogLevelHook is a log hook which can change log level for messages which contains specific substrings
type ForceLogLevelHook struct {
	ForcedLevel Level
}

// NewForceLogLevelHook creates default log reduction hook
func NewForceLogLevelHook(forcedLevel Level) *ForceLogLevelHook {
	return &ForceLogLevelHook{
		ForcedLevel: forcedLevel,
	}
}

// Levels implements logrus.Hook.Levels()
func (hook *ForceLogLevelHook) Levels() []logrus.Level {
	return AllLevels.toLogrusLevels()
}

// Fire implements logrus.Hook.Fire()
func (hook *ForceLogLevelHook) Fire(entry *logrus.Entry) error {
	entry.Level = hook.ForcedLevel.toLogrusLevel()
	// special formatter to skip printing of log entries since after hook evaluation, entries are printed directly
	formatter := LogEntriesDropperFormatter{OriginalFormatter: entry.Logger.Formatter}
	entry.Logger.Formatter = &formatter

	return nil
}

// LogEntriesDropperFormatter is a custom formatter which will ignore log entries which has lower level than preconfigured in logger
type LogEntriesDropperFormatter struct {
	OriginalFormatter logrus.Formatter
}

// Format implements logrus.Formatter
func (formatter *LogEntriesDropperFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry.Logger.Level >= entry.Level {
		return formatter.OriginalFormatter.Format(entry)
	}

	return []byte(""), nil
}
