package util

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseTimestamp(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		arg   string
		value time.Time
		err   string
	}{
		{"2017-11-22T00:00:00Z", time.Date(2017, time.Month(11), 22, 0, 0, 0, 0, time.UTC), ""},
		{"2017-11-22T01:00:00+01:00", time.Date(2017, time.Month(11), 22, 1, 0, 0, 0, time.FixedZone("", 3600)), ""},
		{"bloop", time.Time{}, `not a valid RFC3339 timestamp: cannot use "bloop" as year`},
		{"2017-11-22 00:00:00Z", time.Time{}, `not a valid RFC3339 timestamp: missing required time introducer 'T'`},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(fmt.Sprintf("ParseTimestamp(%#v)", testCase.arg), func(t *testing.T) {
			t.Parallel()

			actual, err := ParseTimestamp(testCase.arg)
			if testCase.err != "" {
				assert.EqualError(t, err, testCase.err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, testCase.value, actual)
		})
	}
}
