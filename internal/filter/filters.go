package filter

import (
	"errors"
	"fmt"
	"strings"
)

// Filters represents multiple filter queries that are evaluated with union (OR) semantics.
// Multiple filters in Filters are always unioned (as opposed to multiple filters
// within one filter string separated by |, which are intersected).
type Filters []*Filter

// ParseFilterQueries parses multiple filter strings and returns a Filters object.
// Collects all parse errors and returns them as a joined error if any occur.
// Returns an empty Filters if filterStrings is empty.
func ParseFilterQueries(filterStrings []string, workingDir string) (Filters, error) {
	if len(filterStrings) == 0 {
		return Filters{}, nil
	}

	filters := make([]*Filter, 0, len(filterStrings))

	var parseErrors []error

	for i, filterString := range filterStrings {
		filter, err := Parse(filterString, workingDir)
		if err != nil {
			parseErrors = append(parseErrors, fmt.Errorf("filter %d (%q): %w", i, filterString, err))

			continue
		}

		filters = append(filters, filter)
	}

	// Return Filters with successfully parsed filters, plus any errors
	result := Filters(filters)

	if len(parseErrors) > 0 {
		return result, errors.Join(parseErrors...)
	}

	return result, nil
}

// startsWithNegation checks if an expression starts with a negation operator.
func startsWithNegation(expr Expression) bool {
	if prefixExpr, ok := expr.(*PrefixExpression); ok {
		return prefixExpr.Operator == "!"
	}

	return false
}

// ExcludeByDefault returns true if the filters operate in exclude-by-default mode.
// This is true if ANY filter doesn't start with a negation expression.
// When true, discovery should start with an empty set and add matches.
// When false, discovery should start with all units and remove matches.
func (f Filters) ExcludeByDefault() bool {
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
//     results and remove matching units
func (f Filters) Evaluate(units []Unit) ([]Unit, error) {
	if len(f) == 0 {
		return units, nil
	}

	// Separate filters into positive and negative
	var positiveFilters, negativeFilters []*Filter

	for _, filter := range f {
		if startsWithNegation(filter.expr) {
			negativeFilters = append(negativeFilters, filter)
		} else {
			positiveFilters = append(positiveFilters, filter)
		}
	}

	// Phase 1: Union positive filters
	seen := make(map[string]Unit, len(units))

	for _, filter := range positiveFilters {
		result, err := filter.Evaluate(units)
		if err != nil {
			return nil, err
		}

		// Add results to seen map (union)
		for _, unit := range result {
			seen[unit.Path] = unit
		}
	}

	// Convert to slice for phase 2
	combined := make([]Unit, 0, len(seen))
	for _, unit := range seen {
		combined = append(combined, unit)
	}

	// Phase 2: Apply negative filters to remove units
	for _, filter := range negativeFilters {
		result, err := filter.Evaluate(combined)
		if err != nil {
			return nil, err
		}

		combined = result
	}

	return combined, nil
}

func initialComponents(positiveFilters []*Filter, components component.Components) (component.Components, error) {
	if len(positiveFilters) == 0 {
		return components, nil
	}

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

	remaining := make(component.Components, 0, len(seen))
	for _, c := range seen {
		remaining = append(remaining, c)
	}

	return remaining, nil
}

// String returns a JSON array representation of all filter strings.
func (f Filters) String() string {
	filterStrings := make([]string, len(f))
	for i, filter := range f {
		filterStrings[i] = filter.String()
	}

	// Create JSON array manually (simpler than importing encoding/json)
	if len(filterStrings) == 0 {
		return "[]"
	}

	result := "["

	for i, s := range filterStrings {
		if i > 0 {
			result += ", "
		}
		// Escape quotes in filter string
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		result += `"` + escaped + `"`
	}

	result += "]"

	return result
}
