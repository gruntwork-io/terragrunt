package rcfile

import (
	"fmt"
	"strconv"
)

// decimalBase is the base used to render whole numbers.
const decimalBase = 10

// flagValues renders a declared default the way the value would be typed on the command
// line. A list becomes one value per element, which is how a flag that accepts multiple
// values, such as a slice or a map flag, is given more than once on the command line.
func flagValues(val any) ([]string, error) {
	if list, ok := val.([]any); ok {
		return listValues(list)
	}

	str, err := scalarValue(val)
	if err != nil {
		return nil, err
	}

	return []string{str}, nil
}

// listValues renders each element of a declared list.
func listValues(list []any) ([]string, error) {
	values := make([]string, 0, len(list))

	for _, item := range list {
		str, err := scalarValue(item)
		if err != nil {
			return nil, err
		}

		values = append(values, str)
	}

	return values, nil
}

// scalarValue renders a single JSON or YAML scalar. JSON decodes every number as a float,
// YAML keeps whole numbers as integers, so both shapes are handled.
func scalarValue(val any) (string, error) {
	switch v := val.(type) {
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, decimalBase), nil
	case uint64:
		return strconv.FormatUint(v, decimalBase), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case nil:
		return "", ErrMissingFlagDefault
	default:
		return "", fmt.Errorf("%w: %T", ErrUnsupportedFlagDefault, val)
	}
}
