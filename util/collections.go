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

// Return the index of the first given element in the given list
func IndexOfElement(list []string, element string) int {
	for index, item := range list {
		if item == element {
			return index
		}
	}

	return -1
}

// Return true if the given list contains the given element
func ListContainsElement(list []string, element string) bool {
	index := IndexOfElement(list, element)

	return index != -1
}

// Returns true if an instance of the list sublist can be found in the given list
func ListContainsSubList(list, sublist []string) bool {
	n := len(sublist)
	switch {
	case n == 0:
		return false
	case n > len(list):
		return false
	case n == 1:
		return IndexOfElement(list, sublist[0]) != -1
	default:
		match := false
		beg := 0
		for !match && beg < len(list) {
			l := list[beg:]
			i := IndexOfElement(l, sublist[0])
			if i == -1 {
				break
			}
			for _, item := range sublist[1:] {
				i++
				if i >= len(l) || item != l[i] {
					match = false
					beg += i
					break
				}
				match = true
			}
		}
		return match
	}
	return false
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
	out := []string{}
	for _, item := range listToClone {
		out = append(out, item)
	}
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
