package formatters

import (
	"reflect"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/exp/maps"
)

const tagName = "opt"

type Formatters []log.Formatter

func (formatters Formatters) Names() []string {
	strs := make([]string, len(formatters))

	for i, formatter := range formatters {
		strs[i] = formatter.Name()
	}

	return strs
}

func (formatters Formatters) String() string {
	return strings.Join(formatters.Names(), ", ")
}

func AllFormatters() Formatters {
	return []log.Formatter{
		NewPrettyFormatter(),
		NewKeyValueFormatter(),
		NewJSONFormatter(),
	}
}

// ParseFormat takes a string and returns a Formatter instance with defined options.
func ParseFormat(str string) (log.Formatter, error) {
	var (
		allFormatters = AllFormatters()
		opts          = make(map[string]any)
		formatter     log.Formatter
	)

	formatters := make(map[string]log.Formatter, len(allFormatters))
	for _, f := range allFormatters {
		formatters[f.Name()] = f
	}

	parts := strings.Split(str, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.ToLower(part)

		if f, ok := formatters[part]; ok {
			formatter = f
			continue
		}

		opts[part] = true
	}

	if formatter == nil {
		return nil, errors.Errorf("invalid format, supported formats: %s", strings.Join(maps.Keys(formatters), ", "))
	}

	for name, value := range opts {
		if err := setOpt(formatter, name, value); err != nil {
			return nil, err
		}
	}

	return formatter, nil
}

func setOpt(formatter log.Formatter, optName string, value interface{}) error {
	val := reflect.ValueOf(formatter).Elem()
	if !val.CanAddr() {
		return errors.Errorf("cannot assign to the item passed, item must be a pointer in order to assign")
	}

	optNames := map[string]int{}

	for i := 0; i < val.NumField(); i++ {
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		tagVal, ok := tag.Lookup(tagName)
		if !ok {
			continue
		}

		tagName := strings.Split(tagVal, ",")[0]
		optNames[tagName] = i
	}

	fieldNum, ok := optNames[optName]
	if !ok {
		return errors.Errorf("invalid option %q for the format %q, supprted options: %s", optName, formatter, strings.Join(maps.Keys(optNames), ", "))
	}

	fieldVal := val.Field(fieldNum)
	fieldVal.Set(reflect.ValueOf(value))
	return nil
}
