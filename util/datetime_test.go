package util_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("ParseTimestamp(%#v)", tc.arg), func(t *testing.T) {
			t.Parallel()

			actual, err := util.ParseTimestamp(tc.arg)
			if tc.err != "" {
				require.EqualError(t, err, tc.err)
			} else {
				require.NoError(t, err)
			}

			assert.Equal(t, tc.value, actual)
		})
	}
}
