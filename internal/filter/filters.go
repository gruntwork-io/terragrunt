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

// RequiresHCLParsing returns the first expression that requires parsing Terragrunt HCL configurations if any do.
func (f Filters) RequiresHCLParsing() (Expression, bool) {
	for _, filter := range f {
		if e, ok := filter.expr.RequiresHCLParsing(); ok {
			return e, true
		}
	}

	return nil, false
}

// Evaluate applies all filters with union (OR) semantics in two phases:
//  1. Positive filters (non-negated) are evaluated and their results are unioned
//  2. Negative filters (starting with negation) are evaluated against the combined
//     results and remove matching components
func (f Filters) Evaluate(components component.Components) (component.Components, error) {
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

	// Phase 1: Get initial set of components, which might need to be filtered further by negative filters
	combined, err := initialComponents(positiveFilters, components)
	if err != nil {
		return nil, err
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

// EvaluateOnFiles evaluates the filters on a list of files and returns the filtered result.
// This is useful for the hcl format command, where we want to evaluate filters on files
// rather than directories, like we do with components.
func (f Filters) EvaluateOnFiles(files []string) (component.Components, error) {
	if e, ok := f.RequiresHCLParsing(); ok {
		return nil, FilterQueryRequiresHCLParsingError{Query: e.String()}
	}

	comps := make(component.Components, 0, len(files))
	for _, file := range files {
		comps = append(comps, component.NewUnit(file))
	}

	if len(f) == 0 {
		return comps, nil
	}

	return f.Evaluate(comps)
}

func initialComponents(positiveFilters []*Filter, components component.Components) (component.Components, error) {
	if len(positiveFilters) == 0 {
		return components, nil
	}

	seen := make(map[string]component.Component, len(components))

	for _, filter := range positiveFilters {
		result, err := filter.Evaluate(components)
		if err != nil {
			return nil, err
		}

		for _, c := range result {
			seen[c.Path()] = c
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
