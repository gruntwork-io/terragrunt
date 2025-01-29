package strict

import (
	"sort"

	"golang.org/x/exp/slices"
)

type Categories []*Category

func (categories Categories) FilterByNames(names ...string) Categories {
	var filtered Categories

	for _, category := range categories {
		if slices.Contains(names, category.Name) {
			filtered = append(filtered, category)
		}
	}

	return filtered
}

func (categories Categories) Find(name string) *Category {
	for _, category := range categories {
		if category.Name == name {
			return category
		}
	}

	return nil
}

func (categories Categories) Len() int {
	return len(categories)
}

func (categories Categories) Less(i, j int) bool {
	if len((categories)[j].Name) == 0 {
		return false
	} else if len((categories)[i].Name) == 0 {
		return true
	}

	return (categories)[i].Name < (categories)[j].Name
}

func (categories Categories) Swap(i, j int) {
	(categories)[i], (categories)[j] = (categories)[j], (categories)[i]
}

func (categories Categories) Sort() Categories {
	sort.Sort(categories)

	return categories
}

type Category struct {
	// Name is the name of the category..
	Name            string
	ShowStatus      bool
	AllowedStatuses Statuses
}

func (category *Category) String() string {
	return category.Name
}
