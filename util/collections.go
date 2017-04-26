package util

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
