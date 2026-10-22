package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    any
		expected any
		name     string
		content  string
	}{
		{
			name:     "content_replaces_value",
			content:  "fixed",
			value:    "original",
			expected: "fixed",
		},
		{
			name:     "empty_content_keeps_value",
			content:  "",
			value:    "original",
			expected: "original",
		},
		{
			name:     "empty_content_keeps_value_type",
			content:  "",
			value:    []string{"a", "b"},
			expected: []string{"a", "b"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.Content(tc.content)
			assert.Equal(t, options.ContentOptionName, opt.Name())

			out, err := opt.Format(nil, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}
