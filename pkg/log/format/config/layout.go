package config

import (
	"fmt"
	"strings"
)

type Layout struct {
	format string
	vars   Vars
	doPost map[PostTask]PostFunc
}

func NewLayout(format string, vars ...*Var) *Layout {
	return &Layout{
		format: format,
		vars:   vars,
		doPost: make(map[PostTask]PostFunc),
	}
}

func (layout *Layout) Value(opt *Option, entry *Entry) string {
	var vals []any

	for _, variable := range layout.vars {
		val := variable.Value(opt, entry)
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
		vars   []*Var
	)

	if parts := strings.Split(str, varSeparator); len(parts) > 1 {
		format = parts[0]
		varNames := parts[1:]

		for _, name := range varNames {
			variable, err := ParseVar(name)
			if err != nil {
				return nil, err
			}
			vars = append(vars, variable)
		}

		if format == "" {
			format = strings.Repeat("%s", len(vars))
		}
	}

	return &Layout{format: format, vars: vars}, nil
}
