package util

import (
	"cmp"
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

// RemoveDuplicates returns a new slice with duplicates removed.
// Note: This function sorts the result, so original order is not preserved.
func RemoveDuplicates[S ~[]E, E cmp.Ordered](list S) S {
	result := slices.Clone(list)
	slices.Sort(result)

	return slices.Compact(result)
}

// MergeSlices combines multiple slices and removes duplicates.
// Note: This function sorts the result, so original order is not preserved.
func MergeSlices[S ~[]E, E cmp.Ordered](slicesToMerge ...S) S {
	result := slices.Concat(slicesToMerge...)
	if result == nil {
		return S{}
	}

	slices.Sort(result)

	return slices.Compact(result)
}

// RemoveDuplicatesKeepLast returns a new slice with duplicates removed, keeping the last occurrence.
// Unlike RemoveDuplicates, this preserves the relative order of elements.
func RemoveDuplicatesKeepLast[S ~[]E, E comparable](list S) S {
	seen := make(map[E]int, len(list))
	result := make(S, 0, len(list))

	for _, item := range list {
		if idx, exists := seen[item]; exists {
			// Remove the previous occurrence
			result = slices.Delete(result, idx, idx+1)
			// Update indices for items that were shifted
			for k, v := range seen {
				if v > idx {
					seen[k] = v - 1
				}
			}
		}

		seen[item] = len(result)
		result = append(result, item)
	}

	return result
}

// FirstNonEmpty returns the first non-empty/non-zero element from the slice, or the zero value if none found.
func FirstNonEmpty[S ~[]E, E comparable](list S) E {
	var empty E
	for _, item := range list {
		if item != empty {
			return item
		}
	}

	return empty
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
