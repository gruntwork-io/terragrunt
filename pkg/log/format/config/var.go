package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/mgutz/ansi"
)

const (
	VarLevel      = "level"
	VarLevelShort = "level-short"
	VarLevelTiny  = "level-tiny"
	VarMessage    = "message"

	VarHour24Zero     = "H"
	VarHour12Zero     = "h"
	VarHour12         = "g"
	VarMinZero        = "i"
	VarSecZero        = "s"
	VarMilliSec       = "v"
	VarMicroSec       = "u"
	VarYearFull       = "Y"
	VarYear           = "y"
	VarMonthNumZero   = "m"
	VarMonthNum       = "n"
	VarMonthText      = "M"
	VarDayZero        = "d"
	VarDay            = "j"
	VarDayText        = "D"
	VarPMUpper        = "A"
	VarPMLower        = "a"
	VarTZText         = "T"
	VarTZNumWithColon = "P"
	VarTZNum          = "O"

	VarDateTime      = "date-time"
	VarDateOnly      = "date-only"
	VarTimeOnly      = "time-only"
	VarRFC3339       = "rfc3339"
	VarRFC3339Nano   = "rfc3339-nano"
	VarSinceStartSec = "since-start-sec"

	VarColorRed    = "color-red"
	VarColorWhite  = "color-white"
	VarColorYellow = "color-yellow"
	VarColorGreen  = "color-green"
	VarColorBlueH  = "color-blue+h"
	VarColorCyan   = "color-cyan"
	VarColorBlackH = "color-black+h"
	VarColorReset  = "color-reset"
	VarColorRandom = "color-random"
)

const (
	doPostTaskResetColor PostTask = iota
	doPostTaskRandomColor
)

const (
	randomColorMask = "$$random-color$$"
)

const varSeparator = "@"

var (
	resetColor = ansi.ColorCode("reset")

	colorsMap = map[string]string{
		VarColorRed:    ansi.ColorCode("red"),
		VarColorWhite:  ansi.ColorCode("white"),
		VarColorYellow: ansi.ColorCode("yellow"),
		VarColorGreen:  ansi.ColorCode("green"),
		VarColorBlueH:  ansi.ColorCode("blue+h"),
		VarColorCyan:   ansi.ColorCode("cyan"),
		VarColorBlackH: ansi.ColorCode("black+h"),
		VarColorReset:  resetColor,
	}

	timestampsMap = map[string]string{
		VarYearFull:       "2006",
		VarYear:           "06",
		VarMonthNumZero:   "01",
		VarMonthNum:       "1",
		VarMonthText:      "Jan",
		VarDay:            "2",
		VarDayZero:        "02",
		VarDayText:        "Mon",
		VarPMUpper:        "PM",
		VarPMLower:        "pm",
		VarHour24Zero:     "15",
		VarHour12Zero:     "03",
		VarHour12:         "3",
		VarMinZero:        "04",
		VarSecZero:        "05",
		VarMicroSec:       ".000000",
		VarMilliSec:       ".000",
		VarTZText:         "MST",
		VarTZNum:          "-0700",
		VarTZNumWithColon: "-07:00",
		VarRFC3339:        time.RFC3339,
		VarRFC3339Nano:    time.RFC3339Nano,
		VarDateTime:       time.DateTime,
		VarDateOnly:       time.DateOnly,
		VarTimeOnly:       time.TimeOnly,
	}

	ArgsFunc = make(map[string]VariableFunc)
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

			if name != VarColorReset {
				opt.layout.doPost[doPostTaskResetColor] = func(val string) string {
					return val + resetColor
				}
			}

			return colorCode
		}
	}

	ArgsFunc[VarColorRandom] = func(opt *Option, entry *Entry) string {
		if entry.disableColor {
			return ""
		}

		opt.layout.doPost[doPostTaskRandomColor] = func(val string) string {
			colorCode := opt.randomColor.ColorCode(val)
			val = strings.ReplaceAll(val, randomColorMask, colorCode)
			return val + resetColor
		}

		return randomColorMask
	}

	ArgsFunc[VarSinceStartSec] = func(opt *Option, entry *Entry) string {
		return fmt.Sprintf("%04d", time.Since(entry.baseTime)/time.Second)
	}

	ArgsFunc[VarLevel] = func(opt *Option, entry *Entry) string {
		return entry.level.String()
	}
	ArgsFunc[VarLevelShort] = func(opt *Option, entry *Entry) string {
		return entry.level.ShortName()
	}
	ArgsFunc[VarLevelTiny] = func(opt *Option, entry *Entry) string {
		return entry.level.TinyName()
	}

	ArgsFunc[VarMessage] = func(opt *Option, entry *Entry) string {
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

type VariableFunc func(opt *Option, entry *Entry) string

type Vars []*Var

type Var struct {
	fn      VariableFunc
	filters Filters
}

func (variable *Var) Value(opt *Option, entry *Entry) string {
	val := variable.fn(opt, entry)
	val = variable.filters.Value(val)
	return val
}

func NewVar(name string, filters ...Filter) *Var {
	if fn, ok := ArgsFunc[name]; ok {
		return &Var{fn: fn, filters: filters}
	}

	fn := func(opt *Option, entry *Entry) string {
		if val, ok := entry.fields[name]; ok {
			return fmt.Sprintf("%s", val)
		}
		return ""
	}
	return &Var{fn: fn, filters: filters}
}

func ParseVar(str string) (*Var, error) {
	var (
		name    = str
		filters Filters
		err     error
	)

	if parts := strings.Split(name, varFilterSeparator); len(parts) > 1 {
		name = parts[0]

		filters, err = ParseFilters(parts[1:])
		if err != nil {
			return nil, err
		}
	}

	return NewVar(name, filters...), nil
}
