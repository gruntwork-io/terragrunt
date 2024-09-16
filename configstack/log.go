package configstack

import (
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
)

// ForceLogLevelHook is a log hook which can change log level for messages which contains specific substrings
type ForceLogLevelHook struct {
	forcedLevel   logrus.Level
	triggerLevels []logrus.Level
}

// NewForceLogLevelHook creates default log reduction hook
func NewForceLogLevelHook(forcedLevel log.Level) *ForceLogLevelHook {
	return &ForceLogLevelHook{
		forcedLevel:   forcedLevel.ToLogrusLevel(),
		triggerLevels: log.AllLevels.ToLogrusLevels(),
	}
}

// Levels implements logrus.Hook.Levels()
func (hook *ForceLogLevelHook) Levels() []logrus.Level {
	return hook.triggerLevels
}

// Fire implements logrus.Hook.Fire()
func (hook *ForceLogLevelHook) Fire(entry *logrus.Entry) error {
	entry.Level = hook.forcedLevel
	// special formatter to skip printing of log entries since after hook evaluation, entries are printed directly
	formatter := LogEntriesDropperFormatter{originalFormatter: entry.Logger.Formatter}
	entry.Logger.Formatter = &formatter

	return nil
}

// LogEntriesDropperFormatter is a custom formatter which will ignore log entries which has lower level than preconfigured in logger
type LogEntriesDropperFormatter struct {
	originalFormatter logrus.Formatter
}

// Format implements logrus.Formatter
func (formatter *LogEntriesDropperFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	if entry.Logger.Level >= entry.Level {
		return formatter.originalFormatter.Format(entry)
	}

	return []byte(""), nil
}
