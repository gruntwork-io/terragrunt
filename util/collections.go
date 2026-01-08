package util

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

func MatchesAny(regExps []string, s string) bool {
	for _, item := range regExps {
		if matched, _ := regexp.MatchString(item, s); matched {
			return true
		}
	}

	return false
}

// ListContainsSublist returns true if an instance of the sublist can be found in the given list
func ListContainsSublist[S ~[]E, E comparable](list, sublist S) bool {
	// A list cannot contain an empty sublist
	if len(sublist) == 0 {
		return false
	}

	if len(sublist) > len(list) {
		return false
	}

	for i := 0; len(list[i:]) >= len(sublist); i++ {
		if slices.Equal(list[i:i+len(sublist)], sublist) {
			return true
		}
	}

	return false
}

// ListHasPrefix returns true if list starts with the given prefix list
func ListHasPrefix[S ~[]E, E comparable](list, prefix S) bool {
	if len(prefix) == 0 {
		return false
	}

	if len(prefix) > len(list) {
		return false
	}

	return slices.Equal(list[:len(prefix)], prefix)
}

// RemoveElementFromList returns a copy of the given list with all instances of the given element removed.
func RemoveElementFromList[S ~[]E, E comparable](list S, element E) S {
	var out S

	for _, item := range list {
		if item != element {
			out = append(out, item)
		}
	}

	return out
}

// RemoveSublistFromList returns a copy of the given list with all instances of the given sublist removed
func RemoveSublistFromList[S ~[]E, E comparable](list, sublist S) S {
	var out = list
	for _, item := range sublist {
		out = RemoveElementFromList(out, item)
	}

	return out
}

// RemoveDuplicatesFromList returns a copy of the given list with all duplicates removed (keeping the first encountereds)
func RemoveDuplicatesFromList[S ~[]E, E comparable](list S) S {
	return removeDuplicatesFromList(list, false)
}

// RemoveDuplicatesFromListKeepLast returns a copy of the given list with all duplicates removed (keeping the last encountereds)
func RemoveDuplicatesFromListKeepLast[S ~[]E, E comparable](list S) S {
	return removeDuplicatesFromList(list, true)
}

func removeDuplicatesFromList[S ~[]E, E comparable](list S, keepLast bool) S {
	out := make(S, 0, len(list))
	present := make(map[E]bool)

	for _, value := range list {
		if _, ok := present[value]; ok {
			if keepLast {
				out = RemoveElementFromList(out, value)
			} else {
				continue
			}
		}

		out = append(out, value)
		present[value] = true
	}

	return out
}

// CommaSeparatedStrings returns an HCL compliant formatted list of strings (each string within double quote)
func CommaSeparatedStrings(list []string) string {
	values := make([]string, 0, len(list))
	for _, value := range list {
		values = append(values, fmt.Sprintf(`"%s"`, value))
	}

	return strings.Join(values, ", ")
}

// RemoveEmptyElements returns a copy of the given list without empty elements.
func RemoveEmptyElements[S ~[]E, E comparable](list S) S {
	var (
		out   S
		empty E
	)

	for _, item := range list {
		if item != empty {
			out = append(out, item)
		}
	}

	return out
}

// GetElement returns the element with the specified `index` from the given `list`.
// if `index` is -1, the last element is returned.
func GetElement[S ~[]E, E comparable](list S, index int) E {
	lenList := len(list)

	if lenList > 0 && lenList > index {
		if index == -1 {
			return (list)[lenList-1]
		}

		return (list)[index]
	}

	var empty E

	return empty
}

// FirstElement returns the first element from the given `list`.
func FirstElement[S ~[]E, E comparable](list S) E {
	return GetElement(list, 0)
}

// SecondElement returns the second element from the given `list`.
func SecondElement[S ~[]E, E comparable](list S) E {
	return GetElement(list, 1)
}

// LastElement returns the last element from the given `list`.
func LastElement[S ~[]E, E comparable](list S) E {
	return GetElement(list, -1)
}

// SplitUrls slices s into all substrings separated by sep and returns a slice of
// the substrings between those separators.
// Taking into account that the `=` sign can also be used as a git tag, e.g. `git@github.com/test.git?ref=feature`
func SplitUrls(s, sep string) []string {
	masks := map[string]string{
		"?ref=": "<ref-place-holder>",
	}

	// mask
	for src, mask := range masks {
		s = strings.ReplaceAll(s, src, mask)
	}

	urls := strings.Split(s, sep)

	// unmask
	for i := range urls {
		for src, mask := range masks {
			urls[i] = strings.ReplaceAll(urls[i], mask, src)
		}
	}

	return urls
}

// SplitComma splits the given string by comma and returns a slice of the substrings.
func SplitComma(s, sep string) []string {
	return strings.Split(s, sep)
}

// MergeStringSlices combines two string slices removing duplicates
func MergeStringSlices(a, b []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(a)+len(b))

	for _, s := range append(a, b...) {
		if _, exists := seen[s]; !exists {
			seen[s] = struct{}{}

			result = append(result, s)
		}
	}

	return result
}
