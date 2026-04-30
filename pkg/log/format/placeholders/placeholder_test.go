package placeholders_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/placeholders"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		input        string
		expectOutput string // if non-empty, format and check output contains this
		expectCount  int    // expected number of placeholders, -1 to skip
		expectErr    bool
	}{
		{
			name:        "single_level",
			input:       "%level",
			expectCount: 1,
		},
		{
			name:        "single_msg",
			input:       "%msg",
			expectCount: 1,
		},
		{
			name:        "single_time",
			input:       "%time",
			expectCount: 1,
		},
		{
			name:        "single_interval",
			input:       "%interval",
			expectCount: 1,
		},
		{
			name:         "plaintext_only",
			input:        "just plain text",
			expectCount:  1,
			expectOutput: "just plain text",
		},
		{
			name:        "mixed_text_and_placeholder",
			input:       "level=%level msg=%msg",
			expectCount: 4, // "level=" + level + " msg=" + msg
		},
		{
			name:        "with_options",
			input:       "%level(format=short)",
			expectCount: 1,
		},
		{
			name:         "escaped_percent",
			input:        "100%%",
			expectCount:  2, // "100" + "%" (from %%)
			expectOutput: "100%",
		},
		{
			name:         "tab",
			input:        "%t",
			expectCount:  1,
			expectOutput: "\t",
		},
		{
			name:         "newline",
			input:        "%n",
			expectCount:  1,
			expectOutput: "\n",
		},
		{
			name:        "field_prefix",
			input:       "%prefix",
			expectCount: 1,
		},
		{
			name:        "field_tf_path",
			input:       "%tf-path",
			expectCount: 1,
		},
		{
			name:        "field_tf_command_args",
			input:       "%tf-command-args",
			expectCount: 1,
		},
		{
			name:      "invalid_name_banana",
			input:     "%banana",
			expectErr: true,
		},
		{
			name:        "complex_multi_placeholder",
			input:       "%level %msg [%interval]",
			expectCount: 6, // level + " " + msg + " [" + interval + "]"
		},
		{
			name:         "empty_string",
			input:        "",
			expectCount:  0,
			expectOutput: "",
		},
		{
			name:        "unnamed_placeholder",
			input:       "%(content='hello')",
			expectCount: 1,
		},
		{
			name:        "duplicate_placeholders_different_options",
			input:       "%level(format=full) %level(format=short)",
			expectCount: 3, // level(full) + " " + level(short)
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			phs, err := placeholders.Parse(tc.input)
			if tc.expectErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.expectCount >= 0 {
				assert.Len(t, phs, tc.expectCount)
			}

			if tc.expectOutput != "" {
				data := newMinimalData("test", log.InfoLevel)
				output, err := phs.Format(data)
				require.NoError(t, err)
				assert.Contains(t, output, tc.expectOutput)
			}
		})
	}
}

func TestPlaceholderRegisterNames(t *testing.T) {
	t.Parallel()

	phs := placeholders.NewPlaceholderRegister()
	names := phs.Names()
	assert.NotEmpty(t, names)

	expectedNames := []string{"interval", "time", "level", "msg"}
	for _, name := range expectedNames {
		assert.Contains(t, names, name)
	}
}

func TestPlaceholdersGet(t *testing.T) {
	t.Parallel()

	phs := placeholders.NewPlaceholderRegister()

	t.Run("existing_level", func(t *testing.T) {
		t.Parallel()

		ph := phs.Get("level")
		assert.NotNil(t, ph)
		assert.Equal(t, "level", ph.Name())
	})

	t.Run("existing_msg", func(t *testing.T) {
		t.Parallel()

		ph := phs.Get("msg")
		assert.NotNil(t, ph)
	})

	t.Run("nonexistent", func(t *testing.T) {
		t.Parallel()

		ph := phs.Get("nonexistent")
		assert.Nil(t, ph)
	})
}

func TestPlaceholdersFormat(t *testing.T) {
	t.Parallel()

	phs := placeholders.Placeholders{
		placeholders.PlainText("hello"),
		placeholders.PlainText(" world"),
	}

	data := newMinimalData("", log.InfoLevel)
	output, err := phs.Format(data)
	require.NoError(t, err)
	assert.Equal(t, "hello world", output)
}

func TestLevelPlaceholderFormats(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		format   string
		contains string
		level    log.Level
	}{
		{name: "full_info", format: "%level", contains: "info", level: log.InfoLevel},
		{name: "full_error", format: "%level", contains: "error", level: log.ErrorLevel},
		{name: "short_info", format: "%level(format=short)", contains: "inf", level: log.InfoLevel},
		{name: "tiny_info", format: "%level(format=tiny)", contains: "i", level: log.InfoLevel},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			phs, err := placeholders.Parse(tc.format)
			require.NoError(t, err)

			data := newMinimalData("msg", tc.level)
			output, err := phs.Format(data)
			require.NoError(t, err)
			assert.Contains(t, output, tc.contains)
		})
	}
}

func TestMessagePlaceholder(t *testing.T) {
	t.Parallel()

	phs, err := placeholders.Parse("%msg")
	require.NoError(t, err)

	data := newMinimalData("hello world", log.InfoLevel)
	output, err := phs.Format(data)
	require.NoError(t, err)
	assert.Equal(t, "hello world", output)
}

func FuzzParse(f *testing.F) {
	seeds := []string{
		"%level", "%msg", "%time", "%interval",
		"%level(format=short)", "%level(format=tiny)",
		"plain text", "%level %msg",
		"%%", "%t", "%n",
		"%(content='hello')",
		"%prefix", "%tf-path", "%tf-command-args",
		"", "%", "%()", "%(content='unclosed",
		"%banana",
		"%level(format=full) some-text %level(format=tiny)",
	}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		phs, err := placeholders.Parse(input)
		if err != nil {
			return
		}

		// If parsing succeeded, formatting should not panic
		data := &options.Data{
			Entry: &log.Entry{
				Entry: logrus.NewEntry(logrus.New()),
				Level: log.InfoLevel,
			},
			DisabledColors: true,
		}

		_, _ = phs.Format(data)
	})
}

func newMinimalData(msg string, level log.Level) *options.Data {
	logrusLogger := logrus.New()
	logrusEntry := logrus.NewEntry(logrusLogger)
	logrusEntry.Level = level.ToLogrusLevel()
	logrusEntry.Message = msg

	return &options.Data{
		Entry: &log.Entry{
			Entry:  logrusEntry,
			Level:  level,
			Fields: log.Fields{},
		},
		DisabledColors: true,
	}
}
