package formats

import (
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	OptionTime = "time"

	OptionLevel      = "level"
	OptionLevelLong  = "@long"
	OptionLevelShort = "@short"

	OptionPrefix        = "prefix"
	OptionPrefixAbsPath = "@abs-path"
	OptionPrefixRelPath = "@rel-path"

	OptionColor = "color"
)

type Options []*Option

func (opts Options) Find(name string, level log.Level) *Option {
	for _, opt := range opts {
		if opt.name == name && (len(opt.levels) == 0 || collections.ListContainsElement(opt.levels, level)) {
			return opt
		}
	}

	return nil
}

type Option struct {
	name   string
	value  string
	enable bool
	levels log.Levels
}

func NewOption(name string, enable bool, value string, levels ...log.Level) *Option {
	return &Option{
		name:   name,
		value:  value,
		enable: enable,
		levels: levels,
	}
}

func ParseOption(str string) (*Option, error) {
	var (
		name   = str
		value  string
		enable bool = true
		levels log.Levels
	)

	if parts := strings.SplitN(name, ":", 2); len(parts) > 1 {
		name = parts[0]
		value = parts[1]
	}

	if parts := strings.Split(name, "@"); len(parts) > 1 {
		name = parts[0]
		levelStrs := parts[1:]

		for _, levelStr := range levelStrs {
			if levelStr == "" {
				continue
			}

			level, err := log.ParseLevel(levelStr)
			if err != nil {
				return nil, err
			}

			levels = append(levels, level)
		}
	}

	if strings.HasPrefix(name, "no-") {
		name = name[3:]
		enable = false
	}

	return NewOption(string(name), enable, value, levels...), nil
}
