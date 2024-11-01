package config

import (
	"sort"
	"strings"

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

func (opts Options) SortByValue() Options {
	sort.Slice(opts, func(i, j int) bool {
		return opts[i].value <= opts[j].value
	})

	return opts
}

func (opts Options) FindByName(name string) Options {
	var foundOpts Options

	for _, opt := range opts {
		if opt.name == name || opt.name == "" || opt.value == name {
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
				value:       opt.value,
				enable:      opt.enable,
				layout:      opt.layout,
				levels:      opt.levels,
				randomColor: opt.randomColor,
			}
		} else {
			if opt.layout != nil {
				new.layout = opt.layout
			}
			if opt.value != "" {
				new.value = opt.value
			}

			new.enable = opt.enable
			new.levels = opt.levels
			new.randomColor = opt.randomColor
		}
	}

	return new
}

func (opts Options) MergeByName() Options {
	var news Options

	for _, opt := range opts {
		isNew := true
		for i, new := range news {
			if opt.name == new.name {
				news[i] = opt
				isNew = false
				break
			}
		}
		if isNew {
			news = append(news, opt)
		}
	}

	return news
}

func (opts Options) MergeIntoOneWithPriorityByLevels(levels ...log.Level) *Option {
	return append(opts.FindWithoutLevels(), opts.FindByLevels(levels...)...).MergeIntoOne()
}

type Option struct {
	name   string
	value  string
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

func (opt *Option) Value(entry *Entry) string {
	if opt.layout == nil {
		return ""
	}

	return opt.layout.Value(opt, entry)
}

func NewOption(name string, enable bool, layout *Layout, levels ...log.Level) *Option {
	if layout == nil {
		layout = NewLayout("%s", NewVar(name))
	}

	var value string

	if parts := strings.SplitN(name, "=", 2); len(parts) > 1 {
		name = parts[0]
		value = parts[1]
	}

	return &Option{
		name:        name,
		value:       value,
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
		value  string
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

	if parts := strings.SplitN(name, "=", 2); len(parts) > 1 {
		name = parts[0]
		value = parts[1]
	}

	if len(parts) > 1 {
		if layout, err = ParseLayout(parts[1]); err != nil {
			return nil, err
		}
	}

	return &Option{
		name:        name,
		value:       value,
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
