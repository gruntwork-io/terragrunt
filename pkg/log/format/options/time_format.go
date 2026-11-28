package options

import (
	"maps"
	"slices"
	"strings"
	"time"
)

// TimeFormatOptionName is the option name.
const TimeFormatOptionName = "format"

const (
	// DateTime names the [time.DateTime] layout, `2006-01-02 15:04:05`.
	DateTime = "date-time"
	// DateOnly names the [time.DateOnly] layout, `2006-01-02`.
	DateOnly = "date-only"
	// TimeOnly names the [time.TimeOnly] layout, `15:04:05`.
	TimeOnly = "time-only"
	// RFC3339 names the [time.RFC3339] layout, `2006-01-02T15:04:05Z07:00`.
	RFC3339 = "rfc3339"
	// RFC3339Nano names the [time.RFC3339Nano] layout,
	// `2006-01-02T15:04:05.999999999Z07:00`.
	RFC3339Nano = "rfc3339-nano"

	// Hour24Zero formats the hour on a 24-hour clock with a leading zero, `00` to `23`.
	Hour24Zero = "H"
	// Hour12Zero formats the hour on a 12-hour clock with a leading zero, `01` to `12`.
	Hour12Zero = "h"
	// Hour12 formats the hour on a 12-hour clock without a leading zero, `1` to `12`.
	Hour12 = "g"
	// MinZero formats the minute with a leading zero, `00` to `59`.
	MinZero = "i"
	// SecZero formats the second with a leading zero, `00` to `59`.
	SecZero = "s"
	// MilliSec formats the milliseconds after a dot, such as `.654`.
	MilliSec = "v"
	// MicroSec formats the microseconds after a dot, such as `.654321`.
	MicroSec = "u"
	// YearFull formats the full year, such as `2024`.
	YearFull = "Y"
	// Year formats the year without its century, such as `24`.
	Year = "y"
	// MonthNumZero formats the month number with a leading zero, `01` to `12`.
	MonthNumZero = "m"
	// MonthNum formats the month number without a leading zero, `1` to `12`.
	MonthNum = "n"
	// MonthText formats the three-letter month name, `Jan` to `Dec`.
	MonthText = "M"
	// DayZero formats the day of the month with a leading zero, `01` to `31`.
	DayZero = "d"
	// Day formats the day of the month without a leading zero, `1` to `31`.
	Day = "j"
	// DayText formats the three-letter weekday name, `Mon` to `Sun`.
	DayText = "D"
	// PMUpper formats the upper-case `AM` or `PM`.
	PMUpper = "A"
	// PMLower formats the lower-case `am` or `pm`.
	PMLower = "a"
	// TZText formats the time zone abbreviation, such as `UTC`.
	TZText = "T"
	// TZNumWithColon formats the UTC offset with a colon, such as `+02:00`.
	TZNumWithColon = "P"
	// TZNum formats the UTC offset without a colon, such as `+0200`.
	TZNum = "O"
)

var (
	timeFormatList = NewTimeFormatValue(map[string]string{
		YearFull:       "2006",
		Year:           "06",
		MonthNumZero:   "01",
		MonthNum:       "1",
		MonthText:      "Jan",
		Day:            "2",
		DayZero:        "02",
		DayText:        "Mon",
		PMUpper:        "PM",
		PMLower:        "pm",
		Hour24Zero:     "15",
		Hour12Zero:     "03",
		Hour12:         "3",
		MinZero:        "04",
		SecZero:        "05",
		MicroSec:       ".000000",
		MilliSec:       ".000",
		TZText:         "MST",
		TZNum:          "-0700",
		TZNumWithColon: "-07:00",
		RFC3339:        time.RFC3339,
		RFC3339Nano:    time.RFC3339Nano,
		DateTime:       time.DateTime,
		DateOnly:       time.DateOnly,
		TimeOnly:       time.TimeOnly,
	})
)

// TimeFormatValue is the value of [TimeFormatOption]: a Go time layout
// translated from a format string of tokens such as [YearFull] and layout names
// such as [RFC3339], for example `Y-m-d H:i:s`.
type TimeFormatValue struct {
	MapValue[string]
}

// NewTimeFormatValue returns a [TimeFormatValue] that translates each key of list
// to the Go layout fragment it maps to.
func NewTimeFormatValue(list map[string]string) *TimeFormatValue {
	return &TimeFormatValue{
		MapValue: NewMapValue(list),
	}
}

// SortedKeys returns the tokens in the order of the layout fragments they map to.
func (val TimeFormatValue) SortedKeys() []string {
	keys := maps.Keys(val.list)

	return slices.SortedFunc(keys, func(a, b string) int {
		return strings.Compare(val.list[a], val.list[b])
	})
}

// Set returns a copy of val whose layout is the format string v translated with
// the tokens [TimeFormat] accepts, not those of val.
func (val TimeFormatValue) Set(v string) *TimeFormatValue {
	val.value = timeFormatList.Value(v)

	return &val
}

// Value translates the format string str to a Go layout, replacing each token
// in the order of [TimeFormatValue.SortedKeys].
func (val TimeFormatValue) Value(str string) string {
	for _, key := range val.SortedKeys() {
		str = strings.ReplaceAll(str, key, val.list[key])
	}

	return str
}

// Parse sets the layout to the format string str translated with the tokens
// [TimeFormat] accepts, not those of val. Text that is not a token passes
// through unchanged.
func (val *TimeFormatValue) Parse(str string) error {
	val.value = timeFormatList.Value(str)

	return nil
}

// TimeFormatOption is the `format` option of the `%time` placeholder. It
// replaces the text with the entry's time in its layout.
type TimeFormatOption struct {
	*CommonOption[string]
}

// Format implements `Option` interface.
func (option *TimeFormatOption) Format(data *Data, _ any) (any, error) {
	return data.Time.Format(option.value.Get()), nil
}

// TimeFormat creates the option to format the entry's time with val, a format
// string of tokens such as [YearFull] and layout names such as [RFC3339].
func TimeFormat(val string) Option {
	return &TimeFormatOption{
		CommonOption: NewCommonOption(TimeFormatOptionName, timeFormatList.Set(val)),
	}
}
