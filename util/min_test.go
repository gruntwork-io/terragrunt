package util_test

import (
	"strconv"
	"testing"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
)

func TestMin(t *testing.T) {
	t.Parallel()

	tc := []struct {
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

	for i, tt := range tc {
		tt := tt

		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, util.Min(tt.x, tt.y))
		})
	}
}
