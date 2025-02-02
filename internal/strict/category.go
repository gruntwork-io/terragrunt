package strict

import (
	"sort"

	"golang.org/x/exp/slices"
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
	if len((categories)[j].Name) == 0 {
		return false
	} else if len((categories)[i].Name) == 0 {
		return true
	}

	return (categories)[i].Name < (categories)[j].Name
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

// Category strict control category. Used to group controls when they are displayed.
type Category struct {
	// Name is the name of the category.
	Name string
	// ShowStatus specifies whether to show or hide `Status`.
	ShowStatus bool
	// AllowedStatuses allows controls to be displayed only with the specified statuses.
	AllowedStatuses Statuses
}

// String implements `fmt.Stringer` interface.
func (category *Category) String() string {
	return category.Name
}
