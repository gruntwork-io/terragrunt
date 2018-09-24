package util

import (
	"fmt"
	"strings"
)

// Return true if the given list contains the given element
func ListContainsElement(list []string, element string) bool {
	for _, item := range list {
		if item == element {
			return true
		}
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

// PrefixListItems prefixes each list element with a given prefix and returns a new list
func PrefixListItems(prefix string, list []string) []string {
	values := make([]string, 0, len(list))
	for _, value := range list {
		values = append(values, prefix+value)
	}
	return values
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
