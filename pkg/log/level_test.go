package log_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expected  log.Level
		expectErr bool
	}{
		{name: "stderr", input: "stderr", expected: log.StderrLevel},
		{name: "stdout", input: "stdout", expected: log.StdoutLevel},
		{name: "error", input: "error", expected: log.ErrorLevel},
		{name: "warn", input: "warn", expected: log.WarnLevel},
		{name: "info", input: "info", expected: log.InfoLevel},
		{name: "debug", input: "debug", expected: log.DebugLevel},
		{name: "trace", input: "trace", expected: log.TraceLevel},
		{name: "upper_INFO", input: "INFO", expected: log.InfoLevel},
		{name: "mixed_Debug", input: "Debug", expected: log.DebugLevel},
		{name: "upper_WARN", input: "WARN", expected: log.WarnLevel},
		{name: "upper_ERROR", input: "ERROR", expected: log.ErrorLevel},
		{name: "upper_TRACE", input: "TRACE", expected: log.TraceLevel},
		{name: "empty", input: "", expectErr: true},
		{name: "invalid_banana", input: "banana", expectErr: true},
		{name: "invalid_inf", input: "inf", expectErr: true},
		{name: "padded_info", input: " info ", expectErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			level, err := log.ParseLevel(tc.input)
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, level)
			}
		})
	}
}

func TestLevelString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		level    log.Level
	}{
		{expected: "stderr", level: log.StderrLevel},
		{expected: "stdout", level: log.StdoutLevel},
		{expected: "error", level: log.ErrorLevel},
		{expected: "warn", level: log.WarnLevel},
		{expected: "info", level: log.InfoLevel},
		{expected: "debug", level: log.DebugLevel},
		{expected: "trace", level: log.TraceLevel},
		{expected: "", level: log.Level(99)},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, tc.level.String())
		})
	}
}

func TestLevelShortName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		level    log.Level
	}{
		{expected: "std", level: log.StderrLevel},
		{expected: "std", level: log.StdoutLevel},
		{expected: "err", level: log.ErrorLevel},
		{expected: "wrn", level: log.WarnLevel},
		{expected: "inf", level: log.InfoLevel},
		{expected: "deb", level: log.DebugLevel},
		{expected: "trc", level: log.TraceLevel},
		{expected: "", level: log.Level(99)},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, tc.level.ShortName())
		})
	}
}

func TestLevelTinyName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expected string
		level    log.Level
	}{
		{expected: "s", level: log.StderrLevel},
		{expected: "s", level: log.StdoutLevel},
		{expected: "e", level: log.ErrorLevel},
		{expected: "w", level: log.WarnLevel},
		{expected: "i", level: log.InfoLevel},
		{expected: "d", level: log.DebugLevel},
		{expected: "t", level: log.TraceLevel},
		{expected: "", level: log.Level(99)},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, tc.level.TinyName())
		})
	}
}

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	t.Parallel()

	for _, level := range log.AllLevels {
		t.Run(level.String(), func(t *testing.T) {
			t.Parallel()

			data, err := level.MarshalText()
			require.NoError(t, err)
			assert.NotEmpty(t, data)

			var unmarshaled log.Level

			err = unmarshaled.UnmarshalText(data)
			require.NoError(t, err)
			assert.Equal(t, level, unmarshaled)
		})
	}

	t.Run("marshal_unknown_level", func(t *testing.T) {
		t.Parallel()

		unknown := log.Level(99)
		_, err := unknown.MarshalText()
		assert.Error(t, err)
	})

	t.Run("unmarshal_invalid_text", func(t *testing.T) {
		t.Parallel()

		var level log.Level

		err := level.UnmarshalText([]byte("banana"))
		assert.Error(t, err)
	})
}

func TestToLogrusLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		level         log.Level
		expectedShift uint32
	}{
		{level: log.StderrLevel, expectedShift: 2},
		{level: log.StdoutLevel, expectedShift: 3},
		{level: log.ErrorLevel, expectedShift: 4},
		{level: log.WarnLevel, expectedShift: 5},
		{level: log.InfoLevel, expectedShift: 6},
		{level: log.DebugLevel, expectedShift: 7},
		{level: log.TraceLevel, expectedShift: 8},
	}

	for _, tc := range tests {
		t.Run(tc.level.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, logrus.Level(tc.expectedShift), tc.level.ToLogrusLevel())
		})
	}

	t.Run("unknown_level", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, logrus.Level(0), log.Level(99).ToLogrusLevel())
	})
}

func TestFromLogrusLevel(t *testing.T) {
	t.Parallel()

	for _, level := range log.AllLevels {
		t.Run(level.String(), func(t *testing.T) {
			t.Parallel()

			logrusLevel := level.ToLogrusLevel()
			roundTripped := log.FromLogrusLevel(logrusLevel)
			assert.Equal(t, level, roundTripped)
		})
	}

	t.Run("unknown_logrus_level", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, log.Level(0), log.FromLogrusLevel(logrus.Level(99)))
	})
}

func TestLevelsContains(t *testing.T) {
	t.Parallel()

	t.Run("contains_known_level", func(t *testing.T) {
		t.Parallel()
		assert.True(t, log.AllLevels.Contains(log.InfoLevel))
	})

	t.Run("does_not_contain_unknown_level", func(t *testing.T) {
		t.Parallel()
		assert.False(t, log.AllLevels.Contains(log.Level(99)))
	})
}

func TestLevelsNames(t *testing.T) {
	t.Parallel()

	names := log.AllLevels.Names()
	assert.Len(t, names, 7)

	for _, name := range names {
		assert.NotEmpty(t, name)
	}
}

func TestLevelsString(t *testing.T) {
	t.Parallel()

	str := log.AllLevels.String()
	assert.Contains(t, str, ",")

	for _, level := range log.AllLevels {
		assert.Contains(t, str, level.String())
	}
}

func TestLevelsToLogrusLevels(t *testing.T) {
	t.Parallel()

	logrusLevels := log.AllLevels.ToLogrusLevels()
	assert.Len(t, logrusLevels, 7)

	for i, logrusLevel := range logrusLevels {
		// Each level should be shifted by 2 from its index
		assert.Equal(t, logrus.Level(uint32(i)+2), logrusLevel)
	}
}

func FuzzParseLevel(f *testing.F) {
	// Seed with valid names, mixed case, and garbage
	seeds := []string{
		"stderr", "stdout", "error", "warn", "info", "debug", "trace",
		"INFO", "Debug", "WARN", "ERROR", "TRACE",
		"", "banana", "inf", " info ", "12345",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		level, err := log.ParseLevel(input)
		if err == nil {
			// Valid level: round-trip must produce the same level
			reparsed, err := log.ParseLevel(level.String())
			require.NoError(t, err)
			assert.Equal(t, level, reparsed)
		}
	})
}
