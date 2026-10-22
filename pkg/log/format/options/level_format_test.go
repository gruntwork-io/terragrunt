package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLevelFormatOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expected string
		format   options.LevelFormatValue
		level    log.Level
	}{
		{
			name:     "full",
			format:   options.LevelFormatFull,
			level:    log.WarnLevel,
			expected: "warn",
		},
		{
			name:     "short",
			format:   options.LevelFormatShort,
			level:    log.WarnLevel,
			expected: "wrn",
		},
		{
			name:     "tiny",
			format:   options.LevelFormatTiny,
			level:    log.ErrorLevel,
			expected: "e",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.LevelFormat(tc.format)
			assert.Equal(t, options.LevelFormatOptionName, opt.Name())

			out, err := opt.Format(newLevelData(t, tc.level), "ignored")
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

func newLevelData(t *testing.T, level log.Level) *options.Data {
	t.Helper()

	return &options.Data{Entry: &log.Entry{Level: level}}
}
