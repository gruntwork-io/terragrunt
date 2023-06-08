package maps

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoin(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		val              any
		sliceSep, mapSep string
		expected         string
	}{
		{map[string]string{"color": "white", "number": "two"}, ",", "=", "color=white,number=two"},
		{map[int]int{10: 100, 20: 200}, " ", ":", "10:100 20:200"},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(fmt.Sprintf("test-%d-val-%v-expected-%s", i, testCase.val, testCase.expected), func(t *testing.T) {
			t.Parallel()

			var actual string

			switch val := testCase.val.(type) {
			case map[string]string:
				actual = Join(val, testCase.sliceSep, testCase.mapSep)
			case map[int]int:
				actual = Join(val, testCase.sliceSep, testCase.mapSep)
			}
			assert.Equal(t, testCase.expected, actual)
		})
	}
}

func TestSlice(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		val      any
		sep      string
		expected []string
	}{
		{map[string]string{"color": "white", "number": "two"}, "=", []string{"color=white", "number=two"}},
		{map[int]int{10: 100, 20: 200}, ":", []string{"10:100", "20:200"}},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(fmt.Sprintf("test-%d-val-%v-expected-%s", i, testCase.val, testCase.expected), func(t *testing.T) {
			t.Parallel()

			var actual []string

			switch val := testCase.val.(type) {
			case map[string]string:
				actual = Slice(val, testCase.sep)
			case map[int]int:
				actual = Slice(val, testCase.sep)
			}
			assert.Equal(t, testCase.expected, actual)
		})
	}
}
