package maps

import (
	"fmt"
	"strings"
)

// Join converts the map to a string type by concatenating the key with the value using the given `mapSep` string, and `sliceSep` string between the slice values.
// For example: `Slice(map[int]string{1: "one", 2: "two"}, "-", ", ")` returns `"1-one, 2-two"`
func Join[M ~map[K]V, K comparable, V any](m M, sliceSep, mapSep string) string {
	list := Slice(m, mapSep)

	return strings.Join(list, sliceSep)
}

// Slice converts the map to a string slice by concatenating the key with the value using the given `sep` string.
// For example: `Slice(map[int]string{1: "one", 2: "two"}, "-")` returns `[]string{"1-one", "2-two"}`
func Slice[M ~map[K]V, K comparable, V any](m M, sep string) []string {
	var list []string

	for key, val := range m {
		s := fmt.Sprintf("%v%s%v", key, sep, val)
		list = append(list, s)

	}

	return list
}
