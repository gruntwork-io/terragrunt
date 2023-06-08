package maps

import (
	"fmt"
	"strings"
)

func Join[M ~map[K]V, K comparable, V any](m M, sliceSep, mapSep string) string {
	list := Slice(m, mapSep)

	return strings.Join(list, sliceSep)
}

func Slice[M ~map[K]V, K comparable, V any](m M, sep string) []string {
	var list []string

	for key, val := range m {
		s := fmt.Sprintf("%v%s%v", key, sep, val)
		list = append(list, s)

	}

	return list
}
