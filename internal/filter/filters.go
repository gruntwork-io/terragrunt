package filter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/component"
)

// Filters represents multiple filter queries that are evaluated with union (OR) semantics.
// Multiple filters in Filters are always unioned (as opposed to multiple filters
// within one filter string separated by |, which are intersected).
type Filters []*Filter

// ParseFilterQueries parses multiple filter strings and returns a Filters object.
// Collects all parse errors and returns them as a joined error if any occur.
// Returns an empty Filters if filterStrings is empty.
func ParseFilterQueries(filterStrings []string) (Filters, error) {
	if len(filterStrings) == 0 {
		return Filters{}, nil
	}

	filters := make([]*Filter, 0, len(filterStrings))

	var parseErrors []error

	for i, filterString := range filterStrings {
		filter, err := Parse(filterString)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Errorf("filter %d (%q): %w", i, filterString, err))

			continue
		}

		filters = append(filters, filter)
	}

	result := Filters(filters)

	if len(parseErrors) > 0 {
		return result, errors.Join(parseErrors...)
	}

	return result, nil
}

// HasPositiveFilter returns true if the filters have any positive filters.
func (f Filters) HasPositiveFilter() bool {
	for _, filter := range f {
		if !startsWithNegation(filter.expr) {
			return true
		}
	}

	return false
}

// Evaluate applies all filters with union (OR) semantics in two phases:
//  1. Positive filters (non-negated) are evaluated and their results are unioned
//  2. Negative filters (starting with negation) are evaluated against the combined
//     results and remove matching components
func (f Filters) Evaluate(components []*component.Component) ([]*component.Component, error) {
	if len(f) == 0 {
		return components, nil
	}

	var (
		positiveFilters = make([]*Filter, 0, len(f))
		negativeFilters = make([]*Filter, 0, len(f))
	)

	for _, filter := range f {
		if startsWithNegation(filter.expr) {
			negativeFilters = append(negativeFilters, filter)

			continue
		}

		positiveFilters = append(positiveFilters, filter)
	}

	// Phase 1: Union positive filters
	seen := make(map[string]*component.Component, len(components))

	for _, filter := range positiveFilters {
		result, err := filter.Evaluate(components)
		if err != nil {
			return nil, err
		}

		for _, c := range result {
			seen[c.Path] = c
		}
	}

	// Convert to slice for phase 2
	combined := make([]*component.Component, 0, len(seen))
	for _, c := range seen {
		combined = append(combined, c)
	}

	// Phase 2: Apply negative filters to remove components
	for _, filter := range negativeFilters {
		result, err := filter.Evaluate(combined)
		if err != nil {
			return nil, err
		}

		combined = result
	}

	return combined, nil
}

// String returns a JSON array representation of all filter strings.
func (f Filters) String() string {
	filterStrings := make([]string, len(f))
	for i, filter := range f {
		filterStrings[i] = filter.String()
	}

	jsonBytes, err := json.Marshal(filterStrings)
	if err != nil {
		return "[]"
	}

	return string(jsonBytes)
}

// startsWithNegation checks if an expression starts with a negation operator.
func startsWithNegation(expr Expression) bool {
	if prefixExpr, ok := expr.(*PrefixExpression); ok {
		return prefixExpr.Operator == "!"
	}

	return false
}
