package maps

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJoin(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		vals             any
		sliceSep, mapSep string
		expectedOneOf    []string
	}{
		{map[string]string{"color": "white", "number": "two"}, ",", "=", []string{"color=white,number=two", "number=two,color=white"}},
		{map[int]int{10: 100, 20: 200}, " ", ":", []string{"10:100 20:200", "20:200 10:100"}},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(fmt.Sprintf("test-%d-vals-%v-expected-%s", i, testCase.vals, testCase.expectedOneOf), func(t *testing.T) {
			t.Parallel()

			var actual string

			switch vals := testCase.vals.(type) {
			case map[string]string:
				actual = Join(vals, testCase.sliceSep, testCase.mapSep)
			case map[int]int:
				actual = Join(vals, testCase.sliceSep, testCase.mapSep)
			}
			assert.Contains(t, testCase.expectedOneOf, actual)
		})
	}
}

func TestSlice(t *testing.T) {
	t.Parallel()

	var testCases = []struct {
		vals     any
		sep      string
		expected []string
	}{
		{map[string]string{"color": "white", "number": "two"}, "=", []string{"color=white", "number=two"}},
		{map[int]int{10: 100, 20: 200}, ":", []string{"10:100", "20:200"}},
	}

	for i, testCase := range testCases {
		// to make sure testCase's values don't get updated due to concurrency within the scope of t.Run(..) below
		testCase := testCase

		t.Run(fmt.Sprintf("test-%d-vals-%v-expected-%s", i, testCase.vals, testCase.expected), func(t *testing.T) {
			t.Parallel()

			var actual []string

			switch vals := testCase.vals.(type) {
			case map[string]string:
				actual = Slice(vals, testCase.sep)
			case map[int]int:
				actual = Slice(vals, testCase.sep)
			}
			for _, exp := range testCase.expected {
				assert.Contains(t, actual, exp)
			}
		})
	}
}
