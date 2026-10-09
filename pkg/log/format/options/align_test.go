package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlignOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    any
		expected string
		align    options.AlignValue
	}{
		{
			name:     "left_moves_spaces_to_the_right",
			align:    options.LeftAlign,
			value:    "  ab ",
			expected: "ab   ",
		},
		{
			name:     "right_moves_spaces_to_the_left",
			align:    options.RightAlign,
			value:    " ab  ",
			expected: "   ab",
		},
		{
			name:     "center_puts_the_odd_space_on_the_left",
			align:    options.CenterAlign,
			value:    "ab   ",
			expected: "  ab ",
		},
		{
			name:     "center_splits_even_spaces",
			align:    options.CenterAlign,
			value:    "ab    ",
			expected: "  ab  ",
		},
		{
			name:     "none_is_passthrough",
			align:    options.NoneAlign,
			value:    "  ab ",
			expected: "  ab ",
		},
		{
			name:     "non_string_value_is_stringified",
			align:    options.RightAlign,
			value:    42,
			expected: "42",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.Align(tc.align)
			assert.Equal(t, options.AlignOptionName, opt.Name())

			out, err := opt.Format(nil, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

func TestAlignOptionParseValue(t *testing.T) {
	t.Parallel()

	opt := options.Align(options.NoneAlign)

	require.NoError(t, opt.ParseValue("right"))

	out, err := opt.Format(nil, "ab  ")
	require.NoError(t, err)
	assert.Equal(t, "  ab", out)

	err = opt.ParseValue("top")
	require.Error(t, err)
	assert.Equal(t, "available values: center,left,right", err.Error())
}
