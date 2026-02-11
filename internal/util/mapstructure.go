package util

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mitchellh/mapstructure"
)

// DecodeWithStringBoolHook decodes input into output using mapstructure,
// allowing only string -> bool coercion for "true"/"false" values.
func DecodeWithStringBoolHook(input, output any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result: output,
		DecodeHook: mapstructure.DecodeHookFuncType(func(from reflect.Type, to reflect.Type, data any) (any, error) {
			if from.Kind() != reflect.String || to.Kind() != reflect.Bool {
				return data, nil
			}

			// TrimSpace + EqualFold for robustness against minor formatting differences.
			strValue := strings.TrimSpace(data.(string))

			switch {
			case strings.EqualFold(strValue, "true"):
				return true, nil
			case strings.EqualFold(strValue, "false"):
				return false, nil
			default:
				return nil, fmt.Errorf("invalid boolean string %q, expected \"true\" or \"false\"", strValue)
			}
		}),
	})
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}
