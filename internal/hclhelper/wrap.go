// Package hclhelper providers helpful tools for working with HCL values.
package hclhelper

import (
	"fmt"
	"sort"
	"strings"
)

// WrapMapToSingleLineHcl - This is a workaround to convert a map[string]any to a single line HCL string.
func WrapMapToSingleLineHcl(m map[string]any) string {
	var attributes = make([]string, 0, len(m))
	for key, value := range m {
		attributes = append(attributes, fmt.Sprintf(`%s=%s`, key, formatHclValue(value)))
	}

	sort.Strings(attributes)

	return fmt.Sprintf("{%s}", strings.Join(attributes, ","))
}

// formatHclValue - Wrap single line HCL values in quotes.
func formatHclValue(value any) string {
	switch v := value.(type) {
	case string:
		escapedValue := strings.ReplaceAll(v, `"`, `\"`)
		return fmt.Sprintf(`"%s"`, escapedValue)
	case map[string]any:
		return WrapMapToSingleLineHcl(v)
	default:
		return fmt.Sprintf(`%v`, v)
	}
}
