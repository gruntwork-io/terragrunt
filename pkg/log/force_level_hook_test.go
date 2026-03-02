package log_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForceLogLevelHookLevels(t *testing.T) {
	t.Parallel()

	hook := log.NewForceLogLevelHook(log.WarnLevel)
	levels := hook.Levels()
	assert.Len(t, levels, 7)
}

func TestForceLogLevelHookFire(t *testing.T) {
	t.Parallel()

	hook := log.NewForceLogLevelHook(log.WarnLevel)
	logger := logrus.New()
	logger.SetLevel(logrus.TraceLevel)

	entry := logrus.NewEntry(logger)
	entry.Level = logrus.InfoLevel

	err := hook.Fire(entry)
	require.NoError(t, err)

	// Entry level should be changed to the forced level (WarnLevel = 3, + shift 2 = logrus.Level(5))
	assert.Equal(t, log.WarnLevel.ToLogrusLevel(), entry.Level)
}

func TestLogEntriesDropperFormatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		loggerLevel log.Level
		forcedLevel log.Level
		expectEmpty bool
	}{
		{
			name:        "entry_at_logger_level_produces_output",
			loggerLevel: log.InfoLevel,
			forcedLevel: log.InfoLevel,
			expectEmpty: false,
		},
		{
			name:        "entry_above_logger_level_produces_output",
			loggerLevel: log.TraceLevel,
			forcedLevel: log.InfoLevel,
			expectEmpty: false,
		},
		{
			name:        "entry_below_logger_level_produces_empty",
			loggerLevel: log.ErrorLevel,
			forcedLevel: log.InfoLevel,
			expectEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger := logrus.New()
			logger.SetLevel(tc.loggerLevel.ToLogrusLevel())

			hook := log.NewForceLogLevelHook(tc.forcedLevel)

			entry := logrus.NewEntry(logger)
			entry.Level = logrus.InfoLevel
			entry.Message = "test message"

			err := hook.Fire(entry)
			require.NoError(t, err)

			// After Fire, the logger's formatter is a LogEntriesDropperFormatter.
			// The dropper checks entry.Logger.Level >= entry.Level.
			output, err := logger.Formatter.Format(entry)
			require.NoError(t, err)

			if tc.expectEmpty {
				assert.Empty(t, string(output))
			} else {
				assert.NotEmpty(t, string(output))
			}
		})
	}
}
