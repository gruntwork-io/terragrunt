package cli

import "sort"

// Category represents a command category used to group commands when displaying them.
type Category struct {
	// Name is the name of the category.
	Name string
	// Order is a number indicating the order in the category list.
	Order uint
}

// String implements `fmt.Stringer` interface.
func (category *Category) String() string {
	return category.Name
}

// Categories is a slice of `Category`.
type Categories []*Category

// Len implements `sort.Interface` interface.
func (categories Categories) Len() int {
	return len(categories)
}

// Less implements `sort.Interface` interface.
func (categories Categories) Less(i, j int) bool {
	if categories[i].Order == categories[j].Order {
		return categories[i].Name < categories[j].Name
	}

	return categories[i].Order < categories[j].Order
}

// Swap implements `sort.Interface` interface.
func (categories Categories) Swap(i, j int) {
	categories[i], categories[j] = categories[j], categories[i]
}

// Sort returns `categories` in sorted order.
func (categories Categories) Sort() Categories {
	sort.Sort(categories)

	return categories
}
