// Package hclhelper providers helpful tools for working with HCL values.
package hclhelper

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// WrapMapToSingleLineHcl - This is a workaround to convert a map[string]any to a single line HCL string.
func WrapMapToSingleLineHcl(m map[string]any) string {
	var attributes = make([]string, 0, len(m))
	for key, value := range m {
		attributes = append(attributes, fmt.Sprintf(`%s=%s`, key, FormatValueToSingleLineHcl(value)))
	}

	sort.Strings(attributes)

	return fmt.Sprintf("{%s}", strings.Join(attributes, ","))
}

// WrapListToSingleLineHcl converts a slice to a single-line HCL list expression.
func WrapListToSingleLineHcl(values []any) string {
	var items = make([]string, 0, len(values))
	for _, item := range values {
		items = append(items, FormatValueToSingleLineHcl(item))
	}

	return fmt.Sprintf("[%s]", strings.Join(items, ","))
}

// FormatValueToSingleLineHcl converts a Go value to a single-line HCL expression.
func FormatValueToSingleLineHcl(value any) string {
	switch v := value.(type) {
	case string:
		return strconv.Quote(v)
	case map[string]any:
		return WrapMapToSingleLineHcl(v)
	case []any:
		return WrapListToSingleLineHcl(v)
	default:
		return fmt.Sprintf(`%v`, v)
	}
}
