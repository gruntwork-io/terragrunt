package options_test

import (
	"encoding/json"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionsGet(t *testing.T) {
	t.Parallel()

	caseOpt := options.Case(options.UpperCase)
	opts := options.Options{options.Prefix("["), caseOpt}

	assert.Same(t, caseOpt, opts.Get(options.CaseOptionName))
	assert.Nil(t, opts.Get(options.ColorOptionName))
}

func TestOptionsNames(t *testing.T) {
	t.Parallel()

	opts := options.Options{options.Prefix("["), options.Case(options.UpperCase), options.Suffix("]")}

	assert.Equal(
		t,
		[]string{options.PrefixOptionName, options.CaseOptionName, options.SuffixOptionName},
		opts.Names(),
	)
	assert.Empty(t, options.Options{}.Names())
}

func TestOptionsMerge(t *testing.T) {
	t.Parallel()

	opts := options.Options{options.Case(options.UpperCase), options.Prefix("[")}

	merged := opts.Merge(options.Case(options.LowerCase), options.Suffix("]"))

	assert.Equal(
		t,
		[]string{options.CaseOptionName, options.PrefixOptionName, options.SuffixOptionName},
		merged.Names(),
	)

	out, err := merged.Format(nil, "MiXeD")
	require.NoError(t, err)
	assert.Equal(t, "[mixed]", out)
}

func TestOptionsFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    any
		name     string
		expected string
		opts     options.Options
	}{
		{
			name:     "applies_options_in_order",
			opts:     options.Options{options.Case(options.UpperCase), options.Prefix("["), options.Suffix("]")},
			value:    "msg",
			expected: "[MSG]",
		},
		{
			name:     "empty_result_stops_the_chain",
			opts:     options.Options{options.Content(""), options.Suffix("]")},
			value:    "",
			expected: "",
		},
		{
			name:     "no_options_joins_string_slice",
			opts:     options.Options{},
			value:    []string{"a", "b", "c"},
			expected: "a b c",
		},
		{
			name:     "no_options_stringifies_other_types",
			opts:     options.Options{},
			value:    3.5,
			expected: "3.5",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			out, err := tc.opts.Format(nil, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

func TestOptionsFormatError(t *testing.T) {
	t.Parallel()

	opts := options.Options{options.Escape(options.JSONEscape), options.Suffix("]")}

	out, err := opts.Format(nil, make(chan int))

	var typeErr *json.UnsupportedTypeError

	require.ErrorAs(t, err, &typeErr)
	assert.Empty(t, out)
}

func TestOptionsConfigure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		rest       string
		formatIn   string
		formatOut  string
		setOptions bool
	}{
		{
			name:      "empty_string",
			input:     "",
			rest:      "",
			formatIn:  "Msg",
			formatOut: "Msg",
		},
		{
			name:      "no_option_block_returns_input",
			input:     " text (case=upper)",
			rest:      " text (case=upper)",
			formatIn:  "Msg",
			formatOut: "Msg",
		},
		{
			name:      "empty_option_block",
			input:     "() text",
			rest:      " text",
			formatIn:  "Msg",
			formatOut: "Msg",
		},
		{
			name:      "single_option",
			input:     "(case=upper) text",
			rest:      " text",
			formatIn:  "Msg",
			formatOut: "MSG",
		},
		{
			name:      "several_options_with_spaces",
			input:     "( case = lower , prefix=< , suffix=>)text",
			rest:      "text",
			formatIn:  "Msg",
			formatOut: "<msg>",
		},
		{
			name:      "single_quoted_value_keeps_separators",
			input:     "(prefix=' a, (b) ')",
			rest:      "",
			formatIn:  "Msg",
			formatOut: " a, (b) Msg",
		},
		{
			name:      "double_quoted_value_keeps_separators",
			input:     `(suffix=" x,y) ")`,
			rest:      "",
			formatIn:  "Msg",
			formatOut: "Msg x,y) ",
		},
		{
			name:      "escaped_quote_does_not_close_the_value",
			input:     `(prefix='it\'s, ') text`,
			rest:      " text",
			formatIn:  "Msg",
			formatOut: `it\'s, Msg`,
		},
		{
			name:      "other_quote_inside_quoted_value",
			input:     `(prefix='say "hi", ')`,
			rest:      "",
			formatIn:  "Msg",
			formatOut: `say "hi", Msg`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts := newConfigurableOptions(t)

			rest, err := opts.Configure(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.rest, rest)

			out, err := opts.Format(nil, tc.formatIn)
			require.NoError(t, err)
			assert.Equal(t, tc.formatOut, out)
		})
	}
}

func newConfigurableOptions(t *testing.T) options.Options {
	t.Helper()

	return options.Options{
		options.Case(options.NoneCase),
		options.Prefix(""),
		options.Suffix(""),
	}
}
