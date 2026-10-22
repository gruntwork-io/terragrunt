package options_test

import (
	"errors"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsConfigureErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check  func(t *testing.T, err error)
		name   string
		input  string
		errMsg string
	}{
		{
			name:   "missing_value_separator",
			input:  "(case)",
			errMsg: `invalid option syntax "case)"`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var target *options.InvalidOptionError

				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name:   "unterminated_option_block",
			input:  "(case=upper",
			errMsg: `invalid option syntax "upper"`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var target *options.InvalidOptionError

				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name:   "empty_option_name",
			input:  "( =upper)",
			errMsg: `empty option name "=upper)"`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var target *options.EmptyOptionNameError

				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name:   "unknown_option_name",
			input:  "(colour=red)",
			errMsg: `invalid option name "colour", available names: case,prefix`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var target *options.InvalidOptionNameError

				assert.ErrorAs(t, err, &target)
			},
		},
		{
			name:   "invalid_option_value",
			input:  "(case=title)",
			errMsg: `option "case", invalid value "title", available values: capitalize,lower,upper`,
			check: func(t *testing.T, err error) {
				t.Helper()

				var target *options.InvalidOptionValueError

				require.ErrorAs(t, err, &target)
				assert.EqualError(t, errors.Unwrap(err), "available values: capitalize,lower,upper")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := options.Options{options.Case(options.NoneCase), options.Prefix("")}

			rest, err := opts.Configure(tc.input)
			require.EqualError(t, err, tc.errMsg)
			assert.Empty(t, rest)
			tc.check(t, err)
		})
	}
}

func TestOptionErrorConstructors(t *testing.T) {
	t.Parallel()

	cause := errors.New("boom")
	opts := options.Options{options.Prefix(""), options.Suffix("")}

	tests := []struct {
		err      error
		name     string
		expected string
	}{
		{
			name:     "invalid_option",
			err:      options.NewInvalidOptionError("x"),
			expected: `invalid option syntax "x"`,
		},
		{
			name:     "empty_option_name",
			err:      options.NewEmptyOptionNameError("=x"),
			expected: `empty option name "=x"`,
		},
		{
			name:     "invalid_option_name",
			err:      options.NewInvalidOptionNameError("bad", opts),
			expected: `invalid option name "bad", available names: prefix,suffix`,
		},
		{
			name:     "invalid_option_value",
			err:      options.NewInvalidOptionValueError(options.Prefix(""), "v", cause),
			expected: `option "prefix", invalid value "v", boom`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.EqualError(t, tc.err, tc.expected)
		})
	}

	valueErr := options.NewInvalidOptionValueError(options.Prefix(""), "v", cause)
	assert.ErrorIs(t, valueErr, cause)
}
