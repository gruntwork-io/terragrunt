package configstack

import "github.com/sirupsen/logrus"

// ForceLogLevelHook - log hook which can change log level for messages which contains specific substrings
type ForceLogLevelHook struct {
	TriggerLevels []logrus.Level
	ForcedLevel   logrus.Level
}

// NewForceLogLevelHook - create default log reduction hook
func NewForceLogLevelHook(forcedLevel logrus.Level) *ForceLogLevelHook {
	return &ForceLogLevelHook{
		ForcedLevel:   forcedLevel,
		TriggerLevels: logrus.AllLevels,
	}
}

// Levels - return log levels on which hook will be triggered
func (hook *ForceLogLevelHook) Levels() []logrus.Level {
	return hook.TriggerLevels
}

// Fire - function invoked against log entries when entry will match loglevel from Levels()
func (hook *ForceLogLevelHook) Fire(entry *logrus.Entry) error {
	entry.Level = hook.ForcedLevel
	// special formatter to skip printing of log entries since after hook evaluation, entries are printed directly
	formatter := LogEntriesDropperFormatter{OriginalFormatter: entry.Logger.Formatter}
	entry.Logger.Formatter = &formatter
	return nil
}

// LogEntriesDropperFormatter - custom formatter which will ignore log entries which has lower level than preconfigured in logger
type LogEntriesDropperFormatter struct {
	OriginalFormatter logrus.Formatter
}

// Format - custom entry formatting function which will drop entries with lower level than set in logger
func (formatter *LogEntriesDropperFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry.Logger.Level >= entry.Level {
		return formatter.OriginalFormatter.Format(entry)
	}
	return []byte(""), nil
}
