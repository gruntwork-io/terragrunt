package options

import (
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/maps"
)

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
	timeFormatValueMap = TimeFormatValueMap{
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
	}
)

type TimeFormatValueMap map[string]string

func (valMap TimeFormatValueMap) SortedKeys() []string {
	keys := maps.Keys(valMap)

	sort.Slice(keys, func(i, j int) bool {
		return timeFormatValueMap[keys[i]] < timeFormatValueMap[keys[j]]
	})

	return keys
}

func (valMap TimeFormatValueMap) Value(str string) string {
	for _, key := range valMap.SortedKeys() {
		str = strings.ReplaceAll(str, key, timeFormatValueMap[key])
	}

	return str
}

type TimeFormatValue string

type timeFormat struct {
	*CommonOption[string]
}

func (option *timeFormat) SetValue(str string) error {
	option.value = timeFormatValueMap.Value(str)

	return nil
}

func (option *timeFormat) Evaluate(data *Data, str string) string {
	return data.Time.Format(option.Value())
}

func TimeFormat(str string) Option {
	return &timeFormat{
		CommonOption: NewCommonOption[string](TimeFormatOptionName, timeFormatValueMap.Value(str), nil),
	}
}
