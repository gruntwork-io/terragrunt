package preset

import (
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

type Options []*Option

func (opts Options) Names() []string {
	strs := make([]string, len(opts))

	for i, opt := range opts {
		strs[i] = opt.name
	}

	return strs
}

func (opts Options) FindByName(name string) Options {
	var foundOpts Options

	for _, opt := range opts {
		if opt.name == name {
			foundOpts = append(foundOpts, opt)
		}
	}

	return foundOpts
}

func (opts Options) FindByLevels(levels ...log.Level) Options {
	var foundOpts Options

	for _, opt := range opts {
		for _, level := range levels {
			if opt.levels.Contains(level) {
				foundOpts = append(foundOpts, opt)
			}
		}
	}

	return foundOpts
}

func (opts Options) FindWithoutLevels() Options {
	var foundOpts Options

	for _, opt := range opts {
		if len(opt.levels) == 0 {
			foundOpts = append(foundOpts, opt)
		}
	}

	return foundOpts
}

func (opts Options) FilterByNamePrefixes(mustContain bool, prefixes ...string) Options {
	var filteredOpts Options

	for _, opt := range opts {
		for _, prefix := range prefixes {
			if strings.HasPrefix(opt.name, prefix) == mustContain {
				filteredOpts = append(filteredOpts, opt)
			}
		}
	}

	return filteredOpts
}

func (opts Options) MergeIntoOne() *Option {
	var new *Option

	for _, opt := range opts {
		if new == nil {
			new = &Option{
				name:        opt.name,
				enable:      opt.enable,
				layout:      opt.layout,
				levels:      opt.levels,
				randomColor: opt.randomColor,
			}
		} else {
			if opt.layout != nil {
				new.layout = opt.layout.Clone()
			}

			new.enable = opt.enable
			new.levels = opt.levels
			new.randomColor = opt.randomColor
		}
	}

	return new
}

func (opts Options) MergeIntoOneWithPriorityByLevels(levels ...log.Level) *Option {
	return append(opts.FindWithoutLevels(), opts.FindByLevels(levels...)...).MergeIntoOne()
}

type Option struct {
	name   string
	enable bool
	layout *Layout
	levels log.Levels

	randomColor *RandomColor
}

func (opt *Option) Enable() bool {
	return opt.enable
}

func (opt *Option) Name() string {
	return opt.name
}

func (opt *Option) Levels() log.Levels {
	return opt.levels
}

func (opt *Option) Layout() *Layout {
	return opt.layout
}

func (opt *Option) Value(entry *Entry) (string, bool) {
	if opt.layout == nil {
		return "", true
	}

	return opt.layout.Value(opt, entry), opt.enable
}

func NewOption(name string, enable bool, layout *Layout, levels ...log.Level) *Option {
	if layout == nil {
		layout = NewLayout("%s", NewArg(name))
	}

	return &Option{
		name:        name,
		enable:      enable,
		layout:      layout,
		levels:      levels,
		randomColor: NewRandomColor(),
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
		name:        name,
		enable:      enable,
		layout:      layout,
		levels:      levels,
		randomColor: NewRandomColor(),
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
