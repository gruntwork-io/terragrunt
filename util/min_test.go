package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMin(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		x        int
		y        int
		expected int
	}{
		{1, 2, 1},
		{100, 4, 4},
		{0, 25, 0},
		{-1, 1, -1},
		{0, -1, -1},
		{1, 1, 1},
	}

	for _, testCase := range testCases {
		assert.Equal(t, testCase.expected, Min(testCase.x, testCase.y))
	}
}
