package options

import (
	"fmt"
	"strings"
)

func toString(val any) string {
	switch val := val.(type) {
	case string:
		return val
	case []string:
		return strings.Join(val, " ")
	}

	return fmt.Sprintf("%v", val)
}
