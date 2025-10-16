package filter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"golang.org/x/sync/errgroup"
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

	// Use parallel parsing for multiple filters
	if len(filterStrings) >= 2 {
		return parseFilterQueriesParallel(filterStrings)
	}

	// Single filter - use serial parsing
	filter, err := Parse(filterStrings[0])
	if err != nil {
		return Filters{}, fmt.Errorf("filter 0 (%q): %w", filterStrings[0], err)
	}

	return Filters{filter}, nil
}

// parseFilterQueriesParallel parses multiple filter strings concurrently using errgroup.
func parseFilterQueriesParallel(filterStrings []string) (Filters, error) {
	type parseResult struct {
		filter *Filter
		err    error
		index  int
	}

	results := make([]parseResult, len(filterStrings))
	var mu sync.Mutex

	g, _ := errgroup.WithContext(context.Background())

	for i, filterString := range filterStrings {
		i, filterString := i, filterString // capture for goroutine
		g.Go(func() error {
			filter, err := Parse(filterString)

			mu.Lock()
			results[i] = parseResult{filter: filter, err: err, index: i}
			mu.Unlock()

			// Don't return error here - we want to collect all results
			return nil
		})
	}

	// Wait for all parsing to complete
	g.Wait()

	// Collect all errors for comprehensive reporting
	var parseErrors []error
	filters := make([]*Filter, 0, len(filterStrings))

	for _, result := range results {
		if result.err != nil {
			parseErrors = append(parseErrors, fmt.Errorf("filter %d (%q): %w", result.index, filterStrings[result.index], result.err))
		} else if result.filter != nil {
			filters = append(filters, result.filter)
		}
	}

	result := Filters(filters)

	if len(parseErrors) > 0 {
		return result, errors.Join(parseErrors...)
	}

	return result, nil
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
	var mu sync.Mutex

	// Use parallel evaluation if we have multiple positive filters
	if len(positiveFilters) >= MinFiltersForParallelUnion {
		g, _ := errgroup.WithContext(context.Background())

		for _, filter := range positiveFilters {
			filter := filter // capture for goroutine
			g.Go(func() error {
				result, err := filter.Evaluate(components)
				if err != nil {
					return err
				}

				mu.Lock()
				for _, c := range result {
					seen[c.Path] = c
				}
				mu.Unlock()
				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return nil, err
		}
	} else {
		// Serial evaluation for small numbers of filters
		for _, filter := range positiveFilters {
			result, err := filter.Evaluate(components)
			if err != nil {
				return nil, err
			}

			for _, c := range result {
				seen[c.Path] = c
			}
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
