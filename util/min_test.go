package util_test

import (
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
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

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.expected, util.Min(tc.x, tc.y))
		})
	}
}
