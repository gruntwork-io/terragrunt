package options_test

import (
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/log/format/options"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeFormatOptionFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		format   string
		expected string
	}{
		{
			name:     "date_and_time_placeholders",
			format:   "Y-m-d H:i:s",
			expected: "2024-03-05 14:07:09",
		},
		{
			name:     "milliseconds",
			format:   "H:i:sv",
			expected: "14:07:09.123",
		},
		{
			name:     "microseconds",
			format:   "s u",
			expected: "09 .123456",
		},
		{
			name:     "twelve_hour_clock",
			format:   "g h A a",
			expected: "2 02 PM pm",
		},
		{
			name:     "short_year_and_unpadded_numbers",
			format:   "y n j",
			expected: "24 3 5",
		},
		{
			name:     "day_name",
			format:   "D",
			expected: "Tue",
		},
		{
			name:     "time_zone",
			format:   "T O P",
			expected: "UTC +0000 +00:00",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opt := options.TimeFormat(tc.format)
			assert.Equal(t, options.TimeFormatOptionName, opt.Name())

			out, err := opt.Format(newTimeData(t), "ignored")
			require.NoError(t, err)
			assert.Equal(t, tc.expected, out)
		})
	}
}

func TestTimeFormatOptionParseValue(t *testing.T) {
	t.Parallel()

	opt := options.TimeFormat("H")

	require.NoError(t, opt.ParseValue("Y/m/d"))

	out, err := opt.Format(newTimeData(t), "ignored")
	require.NoError(t, err)
	assert.Equal(t, "2024/03/05", out)
}

func TestTimeFormatValue(t *testing.T) {
	t.Parallel()

	val := options.NewTimeFormatValue(map[string]string{
		"b": "2",
		"a": "1",
		"c": "3",
	})

	assert.Equal(t, []string{"a", "b", "c"}, val.SortedKeys())
	assert.Equal(t, "1-2-3 x", val.Value("a-b-c x"))
}

func newTimeData(t *testing.T) *options.Data {
	t.Helper()

	return &options.Data{
		Entry: &log.Entry{
			Entry: &logrus.Entry{Time: time.Date(2024, time.March, 5, 14, 7, 9, 123456789, time.UTC)},
		},
	}
}
