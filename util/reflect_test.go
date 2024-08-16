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

	tc := []struct {
		value    interface{}
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
		{interface{}(false), reflect.Bool},
	}
	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.KindOf(tt.value).String()
			assert.Equal(t, tt.expected.String(), actual, "For value %v", tt.value)
			t.Logf("%v passed", tt.value)
		})
	}
}

func TestMustWalkTerraformOutput(t *testing.T) {
	t.Parallel()

	tc := []struct {
		value    interface{}
		path     []string
		expected interface{}
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			actual := util.MustWalkTerraformOutput(tt.value, tt.path...)
			assert.Equal(t, tt.expected, actual)
		})
	}
}
