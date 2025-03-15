package strict

import (
	"slices"
	"sort"
)

// Categories is multiple of DeprecatedFlag Category.
type Categories []*Category

// FilterByNames filters `categories` by the given `names`.
func (categories Categories) FilterByNames(names ...string) Categories {
	var filtered Categories

	for _, category := range categories {
		if slices.Contains(names, category.Name) {
			filtered = append(filtered, category)
		}
	}

	return filtered
}

// FilterNotHidden filters `categories` by the `Hidden:false` field.
func (categories Categories) FilterNotHidden() Categories {
	var filtered Categories

	for _, category := range categories {
		if !category.Hidden {
			filtered = append(filtered, category)
		}
	}

	return filtered
}

// Find search category by given `name`, returns nil if not found.
func (categories Categories) Find(name string) *Category {
	for _, category := range categories {
		if category.Name == name {
			return category
		}
	}

	return nil
}

// Len implements `sort.Interface` interface.
func (categories Categories) Len() int {
	return len(categories)
}

// Less implements `sort.Interface` interface.
func (categories Categories) Less(i, j int) bool {
	// Handle empty names: empty names should come last
	if categories[i].Name == "" {
		return false
	}

	if categories[j].Name == "" {
		return true
	}
	// Normal lexicographical comparison
	return categories[i].Name < categories[j].Name
}

// Swap implements `sort.Interface` interface.
func (categories Categories) Swap(i, j int) {
	(categories)[i], (categories)[j] = (categories)[j], (categories)[i]
}

// Sort returns `categories` in sorted order by `Name`.
func (categories Categories) Sort() Categories {
	sort.Sort(categories)

	return categories
}

// Category represents a strict control category. Used to group controls when they are displayed.
type Category struct {
	// Name is the name of the category.
	Name string
	// Hidden specifies whether controls belonging to this category should be displayed.
	Hidden bool
}

// String implements `fmt.Stringer` interface.
func (category *Category) String() string {
	return category.Name
}
