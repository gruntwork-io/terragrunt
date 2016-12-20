package util

import (
	"time"
	"math/rand"
)

// Get a random amount of time between the lower bound and upper bound. This is useful because some of our automated tests
// wound up flooding the AWS API all at once, leading to a "Subscriber limit exceeded" error.


func GetRandomTime(lowerBound, upperBound time.Duration) time.Duration {
	if lowerBound == upperBound {
		return lowerBound
	}

	var lowerBoundMs, upperBoundMs float64

	if upperBound < 1 * time.Second {
		lowerBoundMs = 1
		upperBoundMs = 1000
	} else {
		lowerBoundMs = lowerBound.Seconds() * 1000
		upperBoundMs = upperBound.Seconds() * 1000
	}

	lowerBoundMsInt := int(lowerBoundMs)
	upperBoundMsInt := int(upperBoundMs)

	randTimeInt := random(lowerBoundMsInt, upperBoundMsInt)
	return time.Duration(randTimeInt) * time.Millisecond
}

// Generate a random int between min and max, inclusive
func random(min int, max int) int {
	rand.Seed(time.Now().UnixNano())
	return rand.Intn(max - min) + min
}