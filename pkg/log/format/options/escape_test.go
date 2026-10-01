package options_test

import (
	"encoding/json"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEscapeOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    any
		expected any
		name     string
		escape   options.EscapeValue
	}{
		{
			name:     "json_escapes_quotes_and_control_chars",
			escape:   options.JSONEscape,
			value:    "say \"hi\"\n\tback\\slash",
			expected: `say \"hi\"\n\tback\\slash`,
		},
		{
			name:     "json_plain_text_is_unchanged",
			escape:   options.JSONEscape,
			value:    "plain",
			expected: "plain",
		},
		{
			name:     "none_is_passthrough",
			escape:   options.NoneEscape,
			value:    "say \"hi\"",
			expected: "say \"hi\"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.Escape(tc.escape)
			assert.Equal(t, options.EscapeOptionName, opt.Name())

			out, err := opt.Format(nil, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

// TestEscapeOptionFormatUnmarshalable checks a value JSON cannot encode returns the encoder error.
func TestEscapeOptionFormatUnmarshalable(t *testing.T) {
	t.Parallel()

	out, err := options.Escape(options.JSONEscape).Format(nil, make(chan int))

	var typeErr *json.UnsupportedTypeError

	require.ErrorAs(t, err, &typeErr)
	assert.Empty(t, out)
}
