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
	return removeDuplicatesFromList(list, true)
}

// Returns a copy of the given list with all duplicates removed (keeping the last encountereds)
func RemoveDuplicatesFromListKeepLast(list []string) []string {
	return removeDuplicatesFromList(list, false)
}

func removeDuplicatesFromList(list []string, fromStart bool) []string {
	out := make([]*string, len(list))
	present := make(map[string]bool)

	for i := range list {
		if !fromStart { // We change the index to start from the end
			i = len(list) - i - 1
		}

		if _, ok := present[list[i]]; !ok {
			out[i] = &list[i]
			present[list[i]] = true // Indicates that element is already in the list
		}
	}
	return removeNil(out)
}

// Remove the nil element from the array
func removeNil(list []*string) (out []string) {
	out = make([]string, 0, len(list))
	for _, element := range list {
		if element != nil {
			out = append(out, *element)
		}
	}
	return
}
