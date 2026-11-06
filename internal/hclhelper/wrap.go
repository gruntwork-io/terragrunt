// Package hclhelper providers helpful tools for working with HCL values.
package hclhelper

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// WrapMapToSingleLineHcl - This is a workaround to convert a map[string]any to a single line HCL string.
// Keys that are not valid HCL identifiers are quoted. Entries with a nil value are left out, as a null
// attribute in a backend configuration is the same as an unset one.
func WrapMapToSingleLineHcl(m map[string]any) string {
	var attributes = make([]string, 0, len(m))
	for key, value := range m {
		if value == nil {
			continue
		}

		attributes = append(
			attributes,
			fmt.Sprintf(`%s=%s`, formatHclKey(key), FormatValueToSingleLineHcl(value)),
		)
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
	case nil:
		return "null"
	case string:
		return quoteHclString(v)
	case map[string]any:
		return WrapMapToSingleLineHcl(v)
	case []any:
		return WrapListToSingleLineHcl(v)
	default:
		return fmt.Sprintf(`%v`, v)
	}
}

// formatHclKey returns key as an object key, quoting it unless it is a valid HCL identifier.
func formatHclKey(key string) string {
	if hclsyntax.ValidIdentifier(key) {
		return key
	}

	return quoteHclString(key)
}

// quoteHclString returns s as an HCL string literal, escaping template sequences such as "${" so
// that they are read back literally.
func quoteHclString(s string) string {
	return string(hclwrite.TokensForValue(cty.StringVal(s)).Bytes())
}
