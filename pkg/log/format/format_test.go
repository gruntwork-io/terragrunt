package format_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{name: "bare", input: "bare"},
		{name: "pretty", input: "pretty"},
		{name: "json", input: "json"},
		{name: "key-value", input: "key-value"},
		{name: "nonexistent", input: "nonexistent", expectErr: true},
		{name: "empty", input: "", expectErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			phs, err := format.ParseFormat(tc.input)
			if tc.expectErr {
				require.Error(t, err)
				assert.Nil(t, phs)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, phs)
				assert.NotEmpty(t, phs)
			}
		})
	}
}

func TestFormatterFormat(t *testing.T) {
	t.Parallel()

	phs := format.NewBareFormatPlaceholders()
	fmtr := format.NewFormatter(phs)
	fmtr.SetDisabledColors(true)

	logrusLogger := logrus.New()
	logrusEntry := logrus.NewEntry(logrusLogger)
	logrusEntry.Level = log.InfoLevel.ToLogrusLevel()
	logrusEntry.Message = "hello formatter"

	entry := &log.Entry{
		Entry: logrusEntry,
		Level: log.InfoLevel,
	}

	output, err := fmtr.Format(entry)
	require.NoError(t, err)
	assert.NotEmpty(t, output)
	assert.Contains(t, string(output), "hello formatter")
}

func TestFormatterDisabledOutput(t *testing.T) {
	t.Parallel()

	phs := format.NewBareFormatPlaceholders()
	fmtr := format.NewFormatter(phs)
	fmtr.SetDisabledOutput(true)

	logrusLogger := logrus.New()
	logrusEntry := logrus.NewEntry(logrusLogger)
	logrusEntry.Level = log.InfoLevel.ToLogrusLevel()
	logrusEntry.Message = "should not appear"

	entry := &log.Entry{
		Entry: logrusEntry,
		Level: log.InfoLevel,
	}

	output, err := fmtr.Format(entry)
	require.NoError(t, err)
	assert.Nil(t, output)
}

func TestFormatterSetFormat(t *testing.T) {
	t.Parallel()

	fmtr := format.NewFormatter(nil)

	err := fmtr.SetFormat("bare")
	require.NoError(t, err)

	logrusLogger := logrus.New()
	logrusEntry := logrus.NewEntry(logrusLogger)
	logrusEntry.Level = log.InfoLevel.ToLogrusLevel()
	logrusEntry.Message = "bare format"

	entry := &log.Entry{
		Entry: logrusEntry,
		Level: log.InfoLevel,
	}

	output, err := fmtr.Format(entry)
	require.NoError(t, err)
	assert.Contains(t, string(output), "bare format")
}

func TestFormatterSetCustomFormat(t *testing.T) {
	t.Parallel()

	fmtr := format.NewFormatter(nil)
	fmtr.SetDisabledColors(true)

	err := fmtr.SetCustomFormat("%level %msg")
	require.NoError(t, err)

	logrusLogger := logrus.New()
	logrusEntry := logrus.NewEntry(logrusLogger)
	logrusEntry.Level = log.InfoLevel.ToLogrusLevel()
	logrusEntry.Message = "custom msg"

	entry := &log.Entry{
		Entry:  logrusEntry,
		Level:  log.InfoLevel,
		Fields: log.Fields{},
	}

	output, err := fmtr.Format(entry)
	require.NoError(t, err)

	outputStr := string(output)
	assert.Contains(t, outputStr, "info")
	assert.Contains(t, outputStr, "custom msg")
}

func TestFormatterNilPlaceholders(t *testing.T) {
	t.Parallel()

	fmtr := format.NewFormatter(nil)

	logrusLogger := logrus.New()
	logrusEntry := logrus.NewEntry(logrusLogger)
	logrusEntry.Level = log.InfoLevel.ToLogrusLevel()
	logrusEntry.Message = "nil placeholders"

	entry := &log.Entry{
		Entry: logrusEntry,
		Level: log.InfoLevel,
	}

	output, err := fmtr.Format(entry)
	require.NoError(t, err)
	assert.Nil(t, output)
}

func TestPlaceholderFormatsAccessible(t *testing.T) {
	t.Parallel()

	t.Run("bare_format", func(t *testing.T) {
		t.Parallel()

		phs := format.NewBareFormatPlaceholders()
		assert.NotEmpty(t, phs)

		// Format should work with minimal data
		logrusLogger := logrus.New()
		logrusEntry := logrus.NewEntry(logrusLogger)
		logrusEntry.Level = log.InfoLevel.ToLogrusLevel()
		logrusEntry.Message = "test"

		data := &options.Data{
			Entry: &log.Entry{
				Entry: logrusEntry,
				Level: log.InfoLevel,
			},
		}

		result, err := phs.Format(data)
		require.NoError(t, err)
		assert.NotEmpty(t, result)
	})

	t.Run("pretty_format", func(t *testing.T) {
		t.Parallel()

		phs := format.NewPrettyFormatPlaceholders()
		assert.NotEmpty(t, phs)
	})

	t.Run("json_format", func(t *testing.T) {
		t.Parallel()

		phs := format.NewJSONFormatPlaceholders()
		assert.NotEmpty(t, phs)
	})

	t.Run("key_value_format", func(t *testing.T) {
		t.Parallel()

		phs := format.NewKeyValueFormatPlaceholders()
		assert.NotEmpty(t, phs)
	})
}

func TestFormatterSetFormatInvalid(t *testing.T) {
	t.Parallel()

	fmtr := format.NewFormatter(nil)
	err := fmtr.SetFormat("nonexistent")
	assert.Error(t, err)
}

func TestFormatterSetCustomFormatInvalid(t *testing.T) {
	t.Parallel()

	fmtr := format.NewFormatter(nil)
	err := fmtr.SetCustomFormat("%banana")
	assert.Error(t, err)
}

func TestFormatterPlaceholderRegisterNames(t *testing.T) {
	t.Parallel()

	phs := placeholders.NewPlaceholderRegister()
	names := phs.Names()
	assert.NotEmpty(t, names)
	assert.Contains(t, names, "level")
	assert.Contains(t, names, "msg")
	assert.Contains(t, names, "time")
	assert.Contains(t, names, "interval")
}
