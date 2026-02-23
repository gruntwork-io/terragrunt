package filter

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Filters represents multiple filter queries that are evaluated with union (OR) semantics.
// Multiple filters in Filters are always unioned (as opposed to multiple filters
// within one filter string separated by |, which are intersected).
type Filters []*Filter

// ParseFilterQueries parses multiple filter strings and returns a Filters object.
// Collects all parse errors and returns them as a joined error if any occur.
// Returns an empty Filters if filterStrings is empty.
// Color output for diagnostics is determined by the logger's color settings and terminal detection.
func ParseFilterQueries(l log.Logger, filterStrings []string) (Filters, error) {
	if len(filterStrings) == 0 {
		return Filters{}, nil
	}

	// Determine if we should use color based on logger settings and terminal detection.
	// Error output goes to stderr, so we check if stderr is redirected.
	useColor := !l.Formatter().DisabledColors()

	filters := make([]*Filter, 0, len(filterStrings))

	var diagnostics []string

	for i, filterString := range filterStrings {
		filter, err := Parse(filterString)
		if err != nil {
			var parseErr ParseError
			if errors.As(err, &parseErr) {
				diagnostics = append(diagnostics, FormatDiagnostic(&parseErr, i, useColor))

				continue
			}

			diagnostics = append(diagnostics, fmt.Sprintf("filter %d: %v", i, err))

			continue
		}

		filters = append(filters, filter)
	}

	result := Filters(filters)

	if len(diagnostics) > 0 {
		return result, fmt.Errorf("%s", strings.Join(diagnostics, "\n"))
	}

	return result, nil
}

// HasPositiveFilter returns true if the filters have any positive filters.
func (f Filters) HasPositiveFilter() bool {
	for _, filter := range f {
		if !IsNegated(filter.expr) {
			return true
		}
	}

	return false
}

// RequiresDiscovery returns the first expression that requires discovery of Terragrunt components if any do.
func (f Filters) RequiresDiscovery() (Expression, bool) {
	for _, filter := range f {
		if e, ok := filter.expr.RequiresDiscovery(); ok {
			return e, true
		}
	}

	return nil, false
}

// RequiresParse returns the first expression that requires parsing of Terragrunt HCL configurations if any do.
func (f Filters) RequiresParse() (Expression, bool) {
	for _, filter := range f {
		if e, ok := filter.RequiresParse(); ok {
			return e, true
		}
	}

	return nil, false
}

// DependencyGraphExpressions returns all target expressions from graph expressions that require dependency traversal.
func (f Filters) DependencyGraphExpressions() []Expression {
	targets := make([]Expression, 0, len(f))

	for _, filter := range f {
		targets = append(targets, collectGraphExpressionTargetsWithDependencies(filter.expr)...)
	}

	return targets
}

// DependentGraphExpressions returns all target expressions from graph expressions that require dependent traversal.
func (f Filters) DependentGraphExpressions() []Expression {
	targets := make([]Expression, 0, len(f))

	for _, filter := range f {
		targets = append(targets, collectGraphExpressionTargetsWithDependents(filter.expr)...)
	}

	return targets
}

// UniqueGitFilters returns all unique Git filters that require worktree discovery.
func (f Filters) UniqueGitFilters() GitExpressions {
	var targets GitExpressions

	seen := make(map[string]struct{})

	for _, filter := range f {
		filterWorktreeExpressions := collectWorktreeExpressions(filter.expr)

		for _, filterWorktreeExpression := range filterWorktreeExpressions {
			if _, ok := seen[filterWorktreeExpression.String()]; ok {
				continue
			}

			seen[filterWorktreeExpression.String()] = struct{}{}
			targets = append(targets, filterWorktreeExpression)
		}
	}

	return targets
}

// RestrictToStacks returns a new Filters object with only the filters that are restricted to stacks.
func (f Filters) RestrictToStacks() Filters {
	return slices.Collect(func(yield func(*Filter) bool) {
		for _, filter := range f {
			if filter.expr.IsRestrictedToStacks() && !yield(filter) {
				return
			}
		}
	})
}

// collectGraphExpressionTargetsWithDependencies collects target expressions from GraphExpression nodes that have IncludeDependencies set.
func collectGraphExpressionTargetsWithDependencies(expr Expression) []Expression {
	var targets []Expression

	WalkExpressions(expr, func(e Expression) bool {
		if graphExpr, ok := e.(*GraphExpression); ok && graphExpr.IncludeDependencies {
			targets = append(targets, graphExpr.Target)
		}

		return true
	})

	return targets
}

// collectGraphExpressionTargetsWithDependents collects target expressions from GraphExpression nodes that have IncludeDependents set.
func collectGraphExpressionTargetsWithDependents(expr Expression) []Expression {
	var targets []Expression

	WalkExpressions(expr, func(e Expression) bool {
		if graphExpr, ok := e.(*GraphExpression); ok && graphExpr.IncludeDependents {
			targets = append(targets, graphExpr.Target)
		}

		return true
	})

	return targets
}

// collectWorktreeExpressions collects worktree expressions from GitExpression nodes.
func collectWorktreeExpressions(expr Expression) []*GitExpression {
	var targets []*GitExpression

	WalkExpressions(expr, func(e Expression) bool {
		if gitExpr, ok := e.(*GitExpression); ok {
			targets = append(targets, gitExpr)
		}

		return true
	})

	return targets
}

// collectGitReferences collects Git references from GitExpression nodes.
func collectGitReferences(expr Expression) []string {
	var refs []string

	WalkExpressions(expr, func(e Expression) bool {
		if gitExpr, ok := e.(*GitExpression); ok {
			refs = append(refs, gitExpr.FromRef, gitExpr.ToRef)
		}

		return true
	})

	return refs
}

// Evaluate applies all filters with union (OR) semantics in two phases:
//  1. Positive filters (non-negated) are evaluated and their results are unioned
//  2. Negative filters (starting with negation) are evaluated against the combined
//     results and remove matching components
//
// If logger is provided, it will be used for logging warnings during evaluation.
func (f Filters) Evaluate(l log.Logger, components component.Components) (component.Components, error) {
	if len(f) == 0 {
		return components, nil
	}

	var (
		positiveFilters = make([]*Filter, 0, len(f))
		negativeFilters = make([]*Filter, 0, len(f))
	)

	for _, filter := range f {
		if IsNegated(filter.expr) {
			negativeFilters = append(negativeFilters, filter)

			continue
		}

		positiveFilters = append(positiveFilters, filter)
	}

	// Phase 1: Get initial set of components, which might need to be filtered further by negative filters
	combined, err := initialComponents(l, positiveFilters, components)
	if err != nil {
		return nil, err
	}

	if len(negativeFilters) == 0 {
		return combined, nil
	}

	// Phase 2: Apply negative filters to find components to remove
	toRemove := make(component.Components, 0, len(combined))

	for _, filter := range negativeFilters {
		removed, err := filter.Negated().Evaluate(l, combined)
		if err != nil {
			return nil, err
		}

		for _, c := range removed {
			if !slices.Contains(toRemove, c) {
				toRemove = append(toRemove, c)
			}
		}
	}

	if len(toRemove) == 0 {
		return combined, nil
	}

	// Phase 3: Remove components from the initial set

	// We don't use slices.DeleteFunc here because we don't want the members of the original components slice to be
	// zeroed.
	results := make(component.Components, 0, len(combined)-len(toRemove))
	for _, c := range combined {
		if slices.Contains(toRemove, c) {
			continue
		}

		results = append(results, c)
	}

	return results, nil
}

// EvaluateOnFiles evaluates the filters on a list of files and returns the filtered result.
// This is useful for the hcl format command, where we want to evaluate filters on files
// rather than directories, like we do with components.
func (f Filters) EvaluateOnFiles(l log.Logger, files []string, workingDir string) (component.Components, error) {
	if e, ok := f.RequiresDiscovery(); ok {
		return nil, FilterQueryRequiresDiscoveryError{Query: e.String()}
	}

	comps := make(component.Components, 0, len(files))
	for _, file := range files {
		unit := component.NewUnit(file)
		if workingDir != "" {
			unit = unit.WithDiscoveryContext(&component.DiscoveryContext{
				WorkingDir: workingDir,
			})
		}

		comps = append(comps, unit)
	}

	if len(f) == 0 {
		return comps, nil
	}

	return f.Evaluate(l, comps)
}

func initialComponents(l log.Logger, positiveFilters []*Filter, components component.Components) (component.Components, error) {
	if len(positiveFilters) == 0 {
		return components, nil
	}

	seen := make(map[string]component.Component, len(components))

	for _, filter := range positiveFilters {
		result, err := filter.Evaluate(l, components)
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
