package clihelper_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/clihelper"
	"github.com/stretchr/testify/assert"
)

func TestLexicographicLess(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		i, j     string
		expected bool
	}{
		{"ab", "cb", true},
		{"ab", "ac", true},
		{"bf", "bc", false},
		{"bb", "bbbb", true},
		{"bbbb", "c", true},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := clihelper.LexicographicLess(tc.i, tc.j)
			assert.Equal(t, tc.expected, actual, tc)
		})
	}
}
