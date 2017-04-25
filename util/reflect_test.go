package util

import (
	"github.com/stretchr/testify/assert"
	"math"
	"reflect"
	"testing"
)

func TestKindOf(t *testing.T) {
	t.Parallel()

	testCases := []struct {
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
		{*new(string), reflect.String},
		{interface{}(false), reflect.Bool},
	}
	for _, testCase := range testCases {
		actual := KindOf(testCase.value).String()
		assert.Equal(t, testCase.expected.String(), actual, "For value %v", testCase.value)
		t.Logf("%v passed", testCase.value)
	}
}
