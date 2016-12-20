package util

import (
	"testing"
	"time"
	"github.com/stretchr/testify/assert"
)

func TestGetRandomTime(t *testing.T) {
	t.Parallel()

	testCases := []struct{
		lowerBound time.Duration
		upperBound time.Duration
	}{
		{ 1 * time.Second, 10 * time.Second },
	}

	for _, testCase := range testCases {
		actual := GetRandomTime(testCase.lowerBound, testCase.upperBound)
		assert.NotEqual(t, testCase.lowerBound, actual, "Randomly computed time %v should not be equal to lower bound time %v.\n", actual, testCase.lowerBound)
		assert.NotEqual(t, testCase.upperBound, actual, "Randomly computed time %v should not be equal to upper bound time %v.\n", actual, testCase.upperBound)

		if actual < testCase.lowerBound {
			t.Fatalf("Randomly computed time %v should not be less than lowerBound %v", actual, testCase.lowerBound)
		}

		if actual > testCase.upperBound {
			t.Fatalf("Randomly computed time %v should not be greater than upperBound %v", actual, testCase.upperBound)
		}
	}
}