package cli_test

import (
	"fmt"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/cli"
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

	for i, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("testCase-%d", i), func(t *testing.T) {
			t.Parallel()

			actual := cli.LexicographicLess(testCase.i, testCase.j)
			assert.Equal(t, testCase.expected, actual, testCase)
		})
	}
}
