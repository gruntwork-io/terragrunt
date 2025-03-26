package util_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/util"
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
		// {1 * time.Second, 1000000001 * time.Nanosecond}, // This case fails
	}

	// Loop through each test case
	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()

			// Try each test case 100 times to avoid fluke test results
			for j := range 100 {
				t.Run(strconv.Itoa(j), func(t *testing.T) {
					t.Parallel()

					actual := util.GetRandomTime(tc.lowerBound, tc.upperBound)

					if tc.lowerBound > 0 && tc.upperBound > 0 {
						if actual < tc.lowerBound {
							t.Fatalf("Randomly computed time %v should not be less than lowerBound %v", actual, tc.lowerBound)
						}

						if actual > tc.upperBound {
							t.Fatalf("Randomly computed time %v should not be greater than upperBound %v", actual, tc.upperBound)
						}
					}
				})
			}
		})
	}
}
