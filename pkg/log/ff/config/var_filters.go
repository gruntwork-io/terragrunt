package config

import (
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
)

const varFilterSeparator = "|"

const (
	FilterRequired Filter = iota
	FilterUpper
	FilterLower
	FilterTitle
)

var (
	// AllFilters exposes all var filters
	AllFilters = Filters{
		FilterUpper,
		FilterLower,
		FilterTitle,
	}

	varFilterNames = map[Filter]string{
		FilterUpper: "upper",
		FilterLower: "lower",
		FilterTitle: "title",
	}
)

type Filters []Filter

func (filters Filters) Value(val string) string {
	for _, filter := range filters {
		switch filter {
		case FilterUpper:
			val = strings.ToUpper(val)
		case FilterLower:
			val = strings.ToLower(val)
		case FilterTitle:
			val = strings.Title(val)
		}
	}

	return val
}

func (filters Filters) String() string {
	return strings.Join(filters.Names(), ", ")
}

func (filters Filters) Names() []string {
	strs := make([]string, len(filters))

	for i, filter := range filters {
		strs[i] = filter.String()
	}

	return strs
}

// ParseFilters takes a string and returns the var filter constants.
func ParseFilters(names []string) (Filters, error) {
	var filters Filters

	for _, name := range names {
		if name == "" {
			continue
		}
		filter, err := ParseFilter(name)
		if err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}

	return filters, nil
}

type Filter byte

// ParseFilter takes a string and returns the var filter constant.
func ParseFilter(str string) (Filter, error) {
	for filter, name := range varFilterNames {
		if strings.EqualFold(name, str) {
			return filter, nil
		}
	}

	return Filter(0), errors.Errorf("invalid variable filter %q, supported variable filtres: %s", str, AllFilters)
}

// String implements fmt.Stringer.
func (filter Filter) String() string {
	if name, ok := varFilterNames[filter]; ok {
		return name
	}

	return ""
}
