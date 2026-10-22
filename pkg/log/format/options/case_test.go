package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaseOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    string
		expected string
		caseVal  options.CaseValue
	}{
		{
			name:     "upper",
			caseVal:  options.UpperCase,
			value:    "Hello World",
			expected: "HELLO WORLD",
		},
		{
			name:     "lower",
			caseVal:  options.LowerCase,
			value:    "Hello World",
			expected: "hello world",
		},
		{
			name:     "capitalize_titles_every_word",
			caseVal:  options.CapitalizeCase,
			value:    "hello wORLD",
			expected: "Hello World",
		},
		{
			name:     "none_is_passthrough",
			caseVal:  options.NoneCase,
			value:    "Hello wORLD",
			expected: "Hello wORLD",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.Case(tc.caseVal)
			assert.Equal(t, options.CaseOptionName, opt.Name())

			out, err := opt.Format(nil, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}
