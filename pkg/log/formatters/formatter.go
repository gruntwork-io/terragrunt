package formatters

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/sirupsen/logrus"
)

type Formatter interface {
	logrus.Formatter
	fmt.Stringer

	Name() string
}

// ParseFormat takes a string and returns the Formatter instance with defined options.
func ParseFormat(str string) (Formatter, error) {
	formatters := []Formatter{
		NewJSONFormatter(),
		NewKeyValueFormatter(),
		NewPrettyFormatter(),
	}

	var (
		formatter Formatter
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

	for _, f := range formatters {
		if strings.EqualFold(f.Name(), name) {
			formatter = f
			break
		}
	}

	if formatter == nil {
		return nil, errors.Errorf("invalid format name: %q", name)
	}

	for name, value := range opts {
		if err := setField(formatter, name, value); err != nil {
			return nil, err
		}
	}

	return formatter, nil
}

func setField(item interface{}, fieldName string, value interface{}) error {
	val := reflect.ValueOf(item).Elem()
	if !val.CanAddr() {
		return errors.Errorf("cannot assign to the item passed, item must be a pointer in order to assign")
	}

	fieldNames := map[string]int{}

	for i := 0; i < val.NumField(); i++ {
		typeField := val.Type().Field(i)
		tag := typeField.Tag

		tagVal, ok := tag.Lookup("opt")
		if !ok {
			continue
		}

		tagName := strings.Split(tagVal, ",")[0]
		fieldNames[tagName] = i
	}

	fieldNum, ok := fieldNames[fieldName]
	if !ok {
		return errors.Errorf("field %s does not exist within the provided item", fieldName)
	}

	fieldVal := val.Field(fieldNum)
	fieldVal.Set(reflect.ValueOf(value))
	return nil
}
