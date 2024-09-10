package preset

import (
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mgutz/ansi"
)

const (
	ArgNameLevel      = "level"
	ArgNameLevelShort = "level-short"
	ArgNameLevelTiny  = "level-tiny"
	ArgNameMessage    = "message"

	ArgNameHour24Zero     = "H"
	ArgNameHour12Zero     = "h"
	ArgNameHour12         = "g"
	ArgNameMinZero        = "i"
	ArgNameSecZero        = "s"
	ArgNameMilliSec       = "v"
	ArgNameMicroSec       = "u"
	ArgNameYearFull       = "Y"
	ArgNameYear           = "y"
	ArgNameMonthNumZero   = "m"
	ArgNameMonthNum       = "n"
	ArgNameMonthText      = "M"
	ArgNameDayZero        = "d"
	ArgNameDay            = "j"
	ArgNameDayText        = "D"
	ArgNamePMUpper        = "A"
	ArgNamePMLower        = "a"
	ArgNameTZText         = "T"
	ArgNameTZNumWithColon = "P"
	ArgNameTZNum          = "O"

	ArgNameDateTime      = "date-time"
	ArgNameDateOnly      = "date-only"
	ArgNameTimeOnly      = "time-only"
	ArgNameRFC3339       = "rfc3339"
	ArgNameRFC3339Nano   = "rfc3339-nano"
	ArgNameSinceStartSec = "since-start-sec"

	ArgNameColorRed    = "color-red"
	ArgNameColorWhite  = "color-white"
	ArgNameColorYellow = "color-yellow"
	ArgNameColorGreen  = "color-green"
	ArgNameColorBlueH  = "color-blue+h"
	ArgNameColorCyan   = "color-cyan"
	ArgNameColorBlackH = "color-black+h"
	ArgNameColorReset  = "color-reset"
	ArgNameColorRandom = "color-random"
)

const (
	doPostTaskResetColor PostTask = iota
	doPostTaskRandomColor
)

const (
	randomColorMaskFmt = "$$random-color$$"
)

var (
	resetColor = ansi.ColorCode("reset")

	colorsMap = map[string]string{
		ArgNameColorRed:    ansi.ColorCode("red"),
		ArgNameColorWhite:  ansi.ColorCode("white"),
		ArgNameColorYellow: ansi.ColorCode("yellow"),
		ArgNameColorGreen:  ansi.ColorCode("green"),
		ArgNameColorBlueH:  ansi.ColorCode("blue+h"),
		ArgNameColorCyan:   ansi.ColorCode("cyan"),
		ArgNameColorBlackH: ansi.ColorCode("black+h"),
		ArgNameColorReset:  resetColor,
	}

	timestampsMap = map[string]string{
		ArgNameYearFull:       "2006",
		ArgNameYear:           "06",
		ArgNameMonthNumZero:   "01",
		ArgNameMonthNum:       "1",
		ArgNameMonthText:      "Jan",
		ArgNameDay:            "2",
		ArgNameDayZero:        "02",
		ArgNameDayText:        "Mon",
		ArgNamePMUpper:        "PM",
		ArgNamePMLower:        "pm",
		ArgNameHour24Zero:     "15",
		ArgNameHour12Zero:     "03",
		ArgNameHour12:         "3",
		ArgNameMinZero:        "04",
		ArgNameSecZero:        "05",
		ArgNameMicroSec:       ".000000",
		ArgNameMilliSec:       ".000",
		ArgNameTZText:         "MST",
		ArgNameTZNum:          "-0700",
		ArgNameTZNumWithColon: "-07:00",
		ArgNameRFC3339:        time.RFC3339,
		ArgNameRFC3339Nano:    time.RFC3339Nano,
		ArgNameDateTime:       time.DateTime,
		ArgNameDateOnly:       time.DateOnly,
		ArgNameTimeOnly:       time.TimeOnly,
	}

	ArgsFunc = make(map[string]ArgFunc)
)

func init() {
	for name, fmt := range timestampsMap {
		ArgsFunc[name] = func(opt *Option, entry *Entry) string {
			return entry.curTime.Format(fmt)
		}
	}

	for name, colorCode := range colorsMap {
		ArgsFunc[name] = func(opt *Option, entry *Entry) string {
			if entry.disableColor {
				return ""
			}

			if name != ArgNameColorReset {
				opt.layout.doPost[doPostTaskResetColor] = func(val string) string {
					return val + resetColor
				}
			}

			return colorCode
		}
	}

	ArgsFunc[ArgNameColorRandom] = func(opt *Option, entry *Entry) string {
		if entry.disableColor {
			return ""
		}

		opt.layout.doPost[doPostTaskRandomColor] = func(val string) string {
			colorCode := opt.randomColor.ColorCode(val)
			val = strings.ReplaceAll(val, randomColorMaskFmt, colorCode)
			return val + resetColor
		}

		return randomColorMaskFmt
	}

	ArgsFunc[ArgNameSinceStartSec] = func(opt *Option, entry *Entry) string {
		return fmt.Sprintf("%04d", time.Since(entry.baseTime)/time.Second)
	}

	ArgsFunc[ArgNameLevel] = func(opt *Option, entry *Entry) string {
		return entry.level.String()
	}
	ArgsFunc[ArgNameLevelShort] = func(opt *Option, entry *Entry) string {
		return entry.level.ShortName()
	}
	ArgsFunc[ArgNameLevelTiny] = func(opt *Option, entry *Entry) string {
		return entry.level.TinyName()
	}

	ArgsFunc[ArgNameMessage] = func(opt *Option, entry *Entry) string {
		return entry.message
	}
}

type PostFunc func(val string) string

type PostTask byte

type Entry struct {
	baseTime     time.Time
	curTime      time.Time
	level        log.Level
	message      string
	fields       log.Fields
	disableColor bool
}

func NewEntry(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields, disableColor bool) *Entry {
	return &Entry{
		baseTime:     baseTime,
		curTime:      curTime,
		level:        level,
		message:      msg,
		fields:       fields,
		disableColor: disableColor,
	}
}

type ArgFunc func(opt *Option, entry *Entry) string

type Args []*Arg

type Arg struct {
	fn   ArgFunc
	opts ArgOpts
}

func NewArg(name string, opts ...ArgOpt) *Arg {
	if fn, ok := ArgsFunc[name]; ok {
		return &Arg{fn: fn, opts: opts}
	}

	fn := func(opt *Option, entry *Entry) string {
		if val, ok := entry.fields[name]; ok {
			return fmt.Sprintf("%s", val)
		}
		return ""
	}
	return &Arg{fn: fn, opts: opts}
}
