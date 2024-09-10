package preset

import (
	"fmt"
	"strings"
)

type Layout struct {
	format string
	args   Args
	doPost map[PostTask]PostFunc
}

func NewLayout(format string, args ...*Arg) *Layout {
	return &Layout{
		format: format,
		args:   args,
		doPost: make(map[PostTask]PostFunc),
	}
}

func (layout *Layout) Clone() *Layout {
	return &Layout{
		format: layout.format,
		args:   layout.args,
		doPost: layout.doPost,
	}
}

func (layout *Layout) Value(opt *Option, entry *Entry) string {
	var vals []any

	for _, arg := range layout.args {

		val := arg.fn(opt, entry)
		for _, opt := range arg.opts {
			switch opt {
			case ArgOptRequired:
				if val == "" {
					return ""
				}
			case ArgOptUpper:
				val = strings.ToUpper(val)
			case ArgOptLower:
				val = strings.ToLower(val)
			case ArgOptTitle:
				val = strings.Title(val)
			}
		}
		vals = append(vals, val)
	}

	val := fmt.Sprintf(layout.format, vals...)

	for _, fn := range layout.doPost {
		val = fn(val)
	}

	return val
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
			var (
				opts ArgOpts
				err  error
			)

			if parts := strings.Split(argName, "|"); len(parts) > 1 {
				argName = parts[0]

				opts, err = ParseArgOpts(parts[1:])
				if err != nil {
					return nil, err
				}
			}

			arg := NewArg(argName, opts...)
			args = append(args, arg)
		}

		if format == "" {
			format = strings.Repeat("%s", len(args))
		}
	}

	return &Layout{format: format, args: args}, nil
}
