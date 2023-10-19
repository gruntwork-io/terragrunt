package util

import (
	"fmt"
	"regexp"
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

// ListEquals returns true if the two lists are equal
func ListEquals(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Return true if the given list contains the given element
func ListContainsElement(list []string, element string) bool {
	for _, item := range list {
		if item == element {
			return true
		}
	}

	return false
}

// ListContainsSublist returns true if an instance of the sublist can be found in the given list
func ListContainsSublist(list, sublist []string) bool {
	// A list cannot contain an empty sublist
	if len(sublist) == 0 {
		return false
	}
	if len(sublist) > len(list) {
		return false
	}
	for i := 0; len(list[i:]) >= len(sublist); i++ {
		if ListEquals(list[i:i+len(sublist)], sublist) {
			return true
		}
	}
	return false
}

// ListHasPrefix returns true if list starts with the given prefix list
func ListHasPrefix(list, prefix []string) bool {
	if len(prefix) == 0 {
		return false
	}
	if len(prefix) > len(list) {
		return false
	}
	return ListEquals(list[:len(prefix)], prefix)
}

// Return a copy of the given list with all instances of the given element removed
func RemoveElementFromList(list []string, element string) []string {
	out := []string{}
	for _, item := range list {
		if item != element {
			out = append(out, item)
		}
	}
	return out
}

// Returns a copy of the given list with all duplicates removed (keeping the first encountereds)
func RemoveDuplicatesFromList(list []string) []string {
	return removeDuplicatesFromList(list, false)
}

// Returns a copy of the given list with all duplicates removed (keeping the last encountereds)
func RemoveDuplicatesFromListKeepLast(list []string) []string {
	return removeDuplicatesFromList(list, true)
}

func removeDuplicatesFromList(list []string, keepLast bool) []string {
	out := make([]string, 0, len(list))
	present := make(map[string]bool)

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

// Make a copy of the given list of strings
func CloneStringList(listToClone []string) []string {
	var out []string
	out = append(out, listToClone...)
	return out
}

// Make a copy of the given map of strings
func CloneStringMap(mapToClone map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range mapToClone {
		out[key] = value
	}
	return out
}

// A convenience method that returns the first item (0th index) in the given list or an empty string if this is an
// empty list
func FirstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// A convenience method that returns the second item (1st index) in the given list or an empty string if this is a
// list that has less than 2 items in it
func SecondArg(args []string) string {
	if len(args) > 1 {
		return args[1]
	}
	return ""
}

// A convenience method that returns the last item in the given list or an empty string if this is an empty list
func LastArg(args []string) string {
	if len(args) > 0 {
		return args[len(args)-1]
	}
	return ""
}

// StringListInsert will insert the given string in to the provided string list at the specified index and return the
// new list of strings. To insert the element, we append the item to the tail of the string and then prepend the
// existing items.
func StringListInsert(list []string, element string, index int) []string {
	tail := append([]string{element}, list[index:]...)
	return append(list[:index], tail...)
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
