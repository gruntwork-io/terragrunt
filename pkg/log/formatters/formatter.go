package formatters

import (
	"reflect"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"golang.org/x/exp/maps"
)

const tagName = "opt"

// ParseFormat takes a string and returns a Formatter instance with defined options.
func ParseFormat(str string) (log.Formatter, error) {
	formatters := []log.Formatter{
		NewJSONFormatter(),
		NewKeyValueFormatter(),
		NewPrettyFormatter(),
	}

	var (
		formatter log.Formatter
		name      string
		opts      = make(map[string]any)
	)

	parts := strings.Split(str, ",")
	for i, part := range parts {
		if i == 0 {
			name = part
			continue
		}

		opts[part] = true
	}

	formatterNames := make([]string, len(formatters))
	for i, f := range formatters {
		if strings.EqualFold(f.Name(), name) {
			formatter = f
		}
		formatterNames[i] = f.Name()
	}

	if formatter == nil {
		return nil, errors.Errorf("invalid format %q, supported formats: %s", name, strings.Join(formatterNames, ", "))
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
