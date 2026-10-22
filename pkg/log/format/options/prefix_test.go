package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrefixOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    any
		name     string
		prefix   string
		expected string
	}{
		{
			name:     "adds_prefix",
			prefix:   "[",
			value:    "msg",
			expected: "[msg",
		},
		{
			name:     "joins_string_slice",
			prefix:   "> ",
			value:    []string{"a", "b"},
			expected: "> a b",
		},
		{
			name:     "empty_prefix",
			prefix:   "",
			value:    "msg",
			expected: "msg",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.Prefix(tc.prefix)
			assert.Equal(t, options.PrefixOptionName, opt.Name())

			out, err := opt.Format(nil, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}
