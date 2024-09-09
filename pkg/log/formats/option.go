package formats

import (
	"strings"

	"github.com/gruntwork-io/go-commons/collections"
	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	OptionColor  = "color"
	OptionTime   = "time"
	OptionLevel  = "level"
	OptionMsg    = "msg"
	OptionPrefix = "prefix"
)

type Options []*Option

func (opts Options) Get(name string, level log.Level) *Option {
	var new *Option

	for _, opt := range opts {
		// if levelFound true, `opt` data takes precedence
		levelFound := collections.ListContainsElement(opt.levels, level)

		if opt.name == name && (levelFound || len(opt.levels) == 0) {
			if new == nil {
				new = &Option{
					name:   opt.name,
					enable: opt.enable,
					layout: opt.layout,
					levels: log.Levels{level},
				}
			} else {
				if new.layout == nil || (levelFound && opt.layout != nil) {
					new.layout = opt.layout
				}

				if levelFound {
					new.enable = opt.enable
				}
			}
		}
	}

	return new
}

type Option struct {
	name   string
	enable bool
	layout *Layout
	levels log.Levels
}

func NewOption(name string, enable bool, layout *Layout, levels ...log.Level) *Option {
	if layout == nil {
		layout = NewLayout("%s", NewArg(name))
	}

	return &Option{
		name:   name,
		enable: enable,
		layout: layout,
		levels: levels,
	}
}

func ParseOption(str string) (*Option, error) {
	var (
		name   = str
		enable = true
		layout *Layout
		levels log.Levels
		err    error
	)

	parts := strings.SplitN(name, ":", 2)
	name = parts[0]
	if strings.HasPrefix(name, "no-") {
		name = name[3:]
		enable = false
	}

	if parts := strings.Split(name, "@"); len(parts) > 1 {
		name = parts[0]

		if levels, err = ParseLevels(parts[1:]); err != nil {
			return nil, err
		}
	}

	if name == "" {
		return nil, errors.Errorf("specified empty option name")
	}

	if len(parts) > 1 {
		if layout, err = ParseLayout(parts[1]); err != nil {
			return nil, err
		}
	}

	return &Option{
		name:   name,
		enable: enable,
		layout: layout,
		levels: levels,
	}, nil
}

func ParseLevels(levelNames []string) (log.Levels, error) {
	var levels log.Levels

	for _, levelName := range levelNames {
		if levelName == "" {
			continue
		}

		level, err := log.ParseLevel(levelName)
		if err != nil {
			return nil, err
		}

		levels = append(levels, level)
	}

	return levels, nil
}
