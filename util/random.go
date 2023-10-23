package util

import (
	"bytes"
	"math/rand"
	"time"
)

const (
	mSecond = 1000
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

	lowerBoundMs := lowerBound.Seconds() * mSecond
	upperBoundMs := upperBound.Seconds() * mSecond

	lowerBoundMsInt := int(lowerBoundMs)
	upperBoundMsInt := int(upperBoundMs)

	randTimeInt := random(lowerBoundMsInt, upperBoundMsInt)
	return time.Duration(randTimeInt) * time.Millisecond
}

// Generate a random int between min and max, inclusive
func random(min int, max int) int {
	return rand.Intn(max-min) + min
}

const BASE_62_CHARS = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
const UNIQUE_ID_LENGTH = 6 // Should be good for 62^6 = 56+ billion combinations

// Returns a unique (ish) id we can use to name resources so they don't conflict with each other. Uses base 62 to
// generate a 6 character string that's unlikely to collide with the handful of tests we run in parallel. Based on code
// here: http://stackoverflow.com/a/9543797/483528
func UniqueId() string {
	var out bytes.Buffer

	for i := 0; i < UNIQUE_ID_LENGTH; i++ {
		out.WriteByte(BASE_62_CHARS[rand.Intn(len(BASE_62_CHARS))])
	}

	return out.String()
}
