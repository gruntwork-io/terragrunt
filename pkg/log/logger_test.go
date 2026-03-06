package log_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	logger := log.New()
	assert.NotNil(t, logger)
}

func TestLoggerLevelFiltering(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		loggerLevel log.Level
		msgLevel    log.Level
		expectEmpty bool
	}{
		{name: "info_at_info_visible", loggerLevel: log.InfoLevel, msgLevel: log.InfoLevel},
		{name: "debug_at_info_hidden", loggerLevel: log.InfoLevel, msgLevel: log.DebugLevel, expectEmpty: true},
		{name: "error_at_info_visible", loggerLevel: log.InfoLevel, msgLevel: log.ErrorLevel},
		{name: "trace_at_trace_visible", loggerLevel: log.TraceLevel, msgLevel: log.TraceLevel},
		{name: "warn_at_error_hidden", loggerLevel: log.ErrorLevel, msgLevel: log.WarnLevel, expectEmpty: true},
		{name: "stderr_at_info_visible", loggerLevel: log.InfoLevel, msgLevel: log.StderrLevel},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			logger, buf := newTestLogger(tc.loggerLevel)
			logger.Log(tc.msgLevel, "test message")

			if tc.expectEmpty {
				assert.Empty(t, buf.String())
			} else {
				assert.Contains(t, buf.String(), "test message")
			}
		})
	}
}

func TestLoggerClone(t *testing.T) {
	t.Parallel()

	original := log.New(log.WithLevel(log.InfoLevel))
	clone := original.Clone()

	assert.NotNil(t, clone)
	assert.Equal(t, log.InfoLevel, clone.Level())

	// Clone preserves output independence: writing to clone doesn't affect a separate buffer
	buf := new(bytes.Buffer)
	cloneWithBuf := clone.WithOptions(log.WithLevel(log.DebugLevel), log.WithOutput(buf))
	cloneWithBuf.Debug("clone message")
	assert.Contains(t, buf.String(), "clone message")
}

func TestLoggerWithOptions(t *testing.T) {
	t.Parallel()

	original := log.New(log.WithLevel(log.InfoLevel))
	modified := original.WithOptions(log.WithLevel(log.DebugLevel))

	assert.NotNil(t, modified)
	assert.Equal(t, log.DebugLevel, modified.Level())
}

func TestLoggerSetLevel(t *testing.T) {
	t.Parallel()

	t.Run("valid_level", func(t *testing.T) {
		t.Parallel()

		logger := log.New(log.WithLevel(log.InfoLevel))
		err := logger.SetLevel("debug")
		require.NoError(t, err)
		assert.Equal(t, log.DebugLevel, logger.Level())
	})

	t.Run("invalid_level", func(t *testing.T) {
		t.Parallel()

		logger := log.New(log.WithLevel(log.InfoLevel))
		err := logger.SetLevel("banana")
		require.Error(t, err)
		assert.Equal(t, log.InfoLevel, logger.Level())
	})
}

func TestLoggerWithField(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.InfoLevel)
	loggerWithField := logger.WithField("key", "value")
	loggerWithField.Info("field test")

	output := buf.String()
	assert.Contains(t, output, "field test")
	assert.Contains(t, output, "key")
}

func TestLoggerWithFields(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.InfoLevel)
	loggerWithFields := logger.WithFields(log.Fields{"k1": "v1", "k2": "v2"})
	loggerWithFields.Info("fields test")

	output := buf.String()
	assert.Contains(t, output, "fields test")
	assert.Contains(t, output, "k1")
	assert.Contains(t, output, "k2")
}

func TestLoggerWithError(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.InfoLevel)
	loggerWithErr := logger.WithError(errors.New("test error"))
	loggerWithErr.Info("error test")

	output := buf.String()
	assert.Contains(t, output, "error test")
	assert.Contains(t, output, "test error")
}

func TestLoggerFormattedOutput(t *testing.T) {
	t.Parallel()

	logger, buf := newTestLogger(log.InfoLevel)
	logger.Infof("hello %s", "world")

	assert.Contains(t, buf.String(), "hello world")
}

// newTestLogger creates a logger that writes to a buffer using the default logrus text formatter.
func newTestLogger(level log.Level) (log.Logger, *bytes.Buffer) {
	buf := new(bytes.Buffer)
	logger := log.New(
		log.WithLevel(level),
		log.WithOutput(buf),
	)

	return logger, buf
}
