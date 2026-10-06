package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSuffixOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    any
		name     string
		suffix   string
		expected string
	}{
		{
			name:     "adds_suffix",
			suffix:   "]",
			value:    "msg",
			expected: "msg]",
		},
		{
			name:     "stringifies_number",
			suffix:   "ms",
			value:    15,
			expected: "15ms",
		},
		{
			name:     "empty_suffix",
			suffix:   "",
			value:    "msg",
			expected: "msg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.Suffix(tc.suffix)
			assert.Equal(t, options.SuffixOptionName, opt.Name())

			out, err := opt.Format(nil, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}
