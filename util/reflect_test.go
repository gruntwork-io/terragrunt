package util_test

import (
	"math"
	"reflect"
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

func TestKindOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value    any
		expected reflect.Kind
	}{
		{1, reflect.Int},
		{2.0, reflect.Float64},
		{'A', reflect.Int32},
		{math.Pi, reflect.Float64},
		{true, reflect.Bool},
		{nil, reflect.Invalid},
		{"Hello World!", reflect.String},
		{new(string), reflect.Ptr},
		{"", reflect.String},
		{any(false), reflect.Bool},
	}
	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.KindOf(tc.value).String()
			assert.Equal(t, tc.expected.String(), actual, "For value %v", tc.value)
			t.Logf("%v passed", tc.value)
		})
	}
}

func TestMustWalkTerraformOutput(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		value    any
		expected any
		path     []string
	}{
		{
			value: map[string]map[string]string{
				"a": {
					"b": "c",
				},
			},
			path:     []string{"a", "b"},
			expected: "c",
		},
		{
			value: map[string]map[string]string{
				"a": {
					"b": "c",
				},
			},
			path:     []string{"a", "d"},
			expected: nil,
		},
		{
			value:    []string{"a", "b", "c"},
			path:     []string{"1"},
			expected: "b",
		},
		{
			value:    []string{"a", "b", "c"},
			path:     []string{"10"},
			expected: nil,
		},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.MustWalkTerraformOutput(tc.value, tc.path...)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
