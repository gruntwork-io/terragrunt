package formats

import (
	"fmt"
	"strings"
	"time"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const argFieldPrefix = "fields."

var (
	timestampLayoutMap = map[string]string{
		"Y":            "2006",
		"y":            "06",
		"m":            "01",
		"n":            "1",
		"M":            "Jan",
		"j":            "2",
		"d":            "02",
		"D":            "Mon",
		"A":            "PM",
		"a":            "pm",
		"H":            "15",
		"h":            "03",
		"g":            "3",
		"i":            "04",
		"s":            "05",
		"u":            ".000000",
		"v":            ".000",
		"T":            "MST",
		"O":            "-0700",
		"P":            "-07:00",
		"rfc3339":      time.RFC3339,
		"rfc3339-nano": time.RFC3339Nano,
		"date-time":    time.DateTime,
		"date-only":    time.DateOnly,
		"time-only":    time.TimeOnly,
	}

	ArgsFunc = make(map[string]ArgFunc)
)

const (
	ArgOptRequireValue ArgOpt = iota
)

type ArgOpt byte

type ArgFunc func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string

type Arg struct {
	fn   ArgFunc
	opts []ArgOpt
}

type Layout struct {
	format string
	args   []*Arg
}

func NewLayout(format string, args ...*Arg) *Layout {
	return &Layout{
		format: format,
		args:   args,
	}
}

func (layout *Layout) Value(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
	var vals []any

	for _, arg := range layout.args {
		val := arg.fn(baseTime, curTime, level, msg, fields)
		if val == "" && collections.ListContainsElement(arg.opts, ArgOptRequireValue) {
			return ""
		}
		vals = append(vals, val)
	}

	return fmt.Sprintf(layout.format, vals...)
}

func NewArg(name string, opts ...ArgOpt) *Arg {
	arg, _ := GetArgWithError(name, opts...)
	return arg
}

func GetArgWithError(name string, opts ...ArgOpt) (*Arg, error) {
	if strings.HasPrefix(name, argFieldPrefix) {
		key := name[len(argFieldPrefix):]
		fn := func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
			if val, ok := fields[key]; ok {
				return fmt.Sprintf("%s", val)
			}
			return ""
		}
		return &Arg{fn: fn, opts: opts}, nil

	} else if fn, ok := ArgsFunc[name]; ok {
		return &Arg{fn: fn, opts: opts}, nil
	}

	return nil, errors.Errorf("invalid argument %q", name)
}

func ParseLayout(str string) (*Layout, error) {
	var (
		format = str
		args   []*Arg
	)

	if parts := strings.Split(str, "@"); len(parts) > 1 {
		format = parts[0]
		argNames := parts[1:]

		for _, argName := range argNames {
			var opts []ArgOpt

			if argName[0] == '?' {
				argName = argName[1:]
				opts = append(opts, ArgOptRequireValue)
			}

			arg, err := GetArgWithError(argName, opts...)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
		}

		if format == "" {
			format = strings.Repeat("%s", len(args))
		}
	}

	return &Layout{format: format, args: args}, nil
}

func init() {
	for name, layout := range timestampLayoutMap {
		ArgsFunc[name] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
			return curTime.Format(layout)
		}
	}
	ArgsFunc["minits"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return fmt.Sprintf("%04d", time.Since(baseTime)/time.Second)
	}

	ArgsFunc["level"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return level.String()
	}
	ArgsFunc["Level"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return strings.Title(level.String())
	}
	ArgsFunc["LEVEL"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return strings.ToUpper(level.String())
	}
	ArgsFunc["lvl"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return level.MiddleName()
	}
	ArgsFunc["Lvl"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return strings.Title(level.MiddleName())
	}
	ArgsFunc["LVL"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return strings.ToUpper(level.MiddleName())
	}
	ArgsFunc["l"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return level.ShortName()
	}
	ArgsFunc["L"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return strings.ToUpper(level.ShortName())
	}

	ArgsFunc["message"] = func(baseTime, curTime time.Time, level log.Level, msg string, fields log.Fields) string {
		return msg
	}

}
