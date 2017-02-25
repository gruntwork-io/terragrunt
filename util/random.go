package util

import (
	"math/rand"
	"time"
)

// Get a random time duration between the lower bound and upper bound. This is useful because some of our automated tests
// wound up flooding the AWS API all at once, leading to a "Subscriber limit exceeded" error.
// TODO: Some of the more exotic test cases fail, but it's not worth catching them given the intended use of this function.
func GetRandomTime(lowerBound, upperBound time.Duration) time.Duration {
	if lowerBound < 0 {
		lowerBound = -1 * lowerBound
	}

	if upperBound < 0 {
		upperBound = -1 * upperBound
	}

	if lowerBound > upperBound {
		return upperBound
	}

	if lowerBound == upperBound {
		return lowerBound
	}

	lowerBoundMs := lowerBound.Seconds() * 1000
	upperBoundMs := upperBound.Seconds() * 1000

	lowerBoundMsInt := int(lowerBoundMs)
	upperBoundMsInt := int(upperBoundMs)

	randTimeInt := random(lowerBoundMsInt, upperBoundMsInt)
	return time.Duration(randTimeInt) * time.Millisecond
}

// Generate a random int between min and max, inclusive
func random(min int, max int) int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max-min) + min
}
