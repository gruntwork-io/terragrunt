package options_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommonOption(t *testing.T) {
	t.Parallel()

	opt := options.NewCommonOption[string]("label", options.NewStringValue("initial"))

	assert.Equal(t, "label", opt.Name())
	assert.Equal(t, "initial", opt.String())

	out, err := opt.Format(nil, "unchanged")
	require.NoError(t, err)
	assert.Equal(t, "unchanged", out)

	require.NoError(t, opt.ParseValue("updated"))
	assert.Equal(t, "updated", opt.String())
}

// TestCommonOptionStringOfEmbeddingOption checks an option embedding CommonOption prints its value via String.
func TestCommonOptionStringOfEmbeddingOption(t *testing.T) {
	t.Parallel()

	opt, ok := options.Width(12).(*options.WidthOption)
	require.True(t, ok)

	assert.Equal(t, "12", opt.String())
}

func TestStringValue(t *testing.T) {
	t.Parallel()

	val := options.NewStringValue("a")
	assert.Equal(t, "a", val.Get())

	require.NoError(t, val.Parse("b"))
	assert.Equal(t, "b", val.Get())
}

func TestIntValueParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		errMsg   string
		expected int
	}{
		{
			name:     "valid_number",
			input:    "42",
			expected: 42,
		},
		{
			name:     "negative_number",
			input:    "-3",
			expected: -3,
		},
		{
			name:     "not_a_number_keeps_previous_value",
			input:    "ten",
			errMsg:   "incorrect option value: ten",
			expected: 7,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			val := options.NewIntValue(7)

			err := val.Parse(tc.input)
			if tc.errMsg != "" {
				require.EqualError(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.expected, val.Get())
		})
	}
}

func TestMapValueParse(t *testing.T) {
	t.Parallel()

	list := options.NewMapValue(map[int]string{
		1: "one",
		2: "two",
		3: "three",
	})

	val := list.Set(1)
	assert.Equal(t, 1, val.Get())

	require.NoError(t, val.Parse("three"))
	assert.Equal(t, 3, val.Get())

	err := val.Parse("four")
	require.EqualError(t, err, "available values: one,three,two")
	assert.Equal(t, 3, val.Get())
}

func TestMapValueFilter(t *testing.T) {
	t.Parallel()

	list := options.NewMapValue(map[int]string{
		1: "one",
		2: "two",
		3: "three",
	})

	// 4 is not in the list, so the filter drops it.
	filtered := list.Filter(1, 3, 4)

	val := filtered.Set(1)
	require.NoError(t, val.Parse("three"))
	assert.Equal(t, 3, val.Get())

	err := val.Parse("two")
	require.EqualError(t, err, "available values: one,three")
}
