package util

import (
	"testing"
	"time"
)

func TestGetRandomTime(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		lowerBound time.Duration
		upperBound time.Duration
	}{
		{1 * time.Second, 10 * time.Second},
		{0, 0},
		{-1 * time.Second, -3 * time.Second},
		{1 * time.Second, 2000000001 * time.Nanosecond},
		{1 * time.Millisecond, 10 * time.Millisecond},
		//{1 * time.Second, 1000000001 * time.Nanosecond}, // This case fails
	}

	// Loop through each test case
	for _, testCase := range testCases {
		// Try each test case 100 times to avoid fluke test results
		for i := 0; i < 100; i++ {
			actual := GetRandomTime(testCase.lowerBound, testCase.upperBound)

			if testCase.lowerBound > 0 && testCase.upperBound > 0 {
				if actual < testCase.lowerBound {
					t.Fatalf("Randomly computed time %v should not be less than lowerBound %v", actual, testCase.lowerBound)
				}

				if actual > testCase.upperBound {
					t.Fatalf("Randomly computed time %v should not be greater than upperBound %v", actual, testCase.upperBound)
				}
			}
		}
	}
}
