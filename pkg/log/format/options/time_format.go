package options

import (
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/maps"
)

// TimeFormatOptionName is the option name.
const TimeFormatOptionName = "format"

const (
	DateTime    = "date-time"
	DateOnly    = "date-only"
	TimeOnly    = "time-only"
	RFC3339     = "rfc3339"
	RFC3339Nano = "rfc3339-nano"

	Hour24Zero     = "H"
	Hour12Zero     = "h"
	Hour12         = "g"
	MinZero        = "i"
	SecZero        = "s"
	MilliSec       = "v"
	MicroSec       = "u"
	YearFull       = "Y"
	Year           = "y"
	MonthNumZero   = "m"
	MonthNum       = "n"
	MonthText      = "M"
	DayZero        = "d"
	Day            = "j"
	DayText        = "D"
	PMUpper        = "A"
	PMLower        = "a"
	TZText         = "T"
	TZNumWithColon = "P"
	TZNum          = "O"
)

var (
	timeFormatList = NewTimeFormatValue(map[string]string{ //nolint:gochecknoglobals
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

type TimeFormatValue struct {
	MapValue[string]
}

func NewTimeFormatValue(list map[string]string) *TimeFormatValue {
	return &TimeFormatValue{
		MapValue: NewMapValue(list),
	}
}

func (val TimeFormatValue) SortedKeys() []string {
	keys := maps.Keys(val.list)

	sort.Slice(keys, func(i, j int) bool {
		return val.list[keys[i]] < val.list[keys[j]]
	})

	return keys
}

func (val TimeFormatValue) Set(v string) *TimeFormatValue {
	val.value = timeFormatList.Value(v)

	return &val
}

func (val TimeFormatValue) Value(str string) string {
	for _, key := range val.SortedKeys() {
		str = strings.ReplaceAll(str, key, val.list[key])
	}

	return str
}

func (val *TimeFormatValue) Parse(str string) error {
	val.value = timeFormatList.Value(str)

	return nil
}

type TimeFormatOption struct {
	*CommonOption[string]
}

// Format implements `Option` interface.
func (option *TimeFormatOption) Format(data *Data, _ any) (any, error) {
	return data.Time.Format(option.value.Get()), nil
}

func TimeFormat(val string) Option {
	return &TimeFormatOption{
		CommonOption: NewCommonOption(TimeFormatOptionName, timeFormatList.Set(val)),
	}
}
