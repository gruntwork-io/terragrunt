package filter

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	AttributeName     = "name"
	AttributeType     = "type"
	AttributeExternal = "external"
	AttributeReading  = "reading"
	AttributeSource   = "source"

	AttributeTypeValueUnit  = string(component.UnitKind)
	AttributeTypeValueStack = string(component.StackKind)

	AttributeExternalValueTrue  = "true"
	AttributeExternalValueFalse = "false"

	// MaxTraversalDepth is the maximum depth to traverse the graph for both dependencies and dependents.
	MaxTraversalDepth = 1000000
)

// EvaluationContext provides additional context for filter evaluation, such as Git worktree directories.
type EvaluationContext struct {
	// GitWorktrees maps Git references to temporary worktree directory paths.
	// This is used by GitFilter expressions to access different Git references.
	GitWorktrees map[string]string
	// WorkingDir is the base working directory for resolving relative paths.
	WorkingDir string
}

// Evaluate evaluates an expression against a list of components and returns the filtered components.
// If logger is provided, it will be used for logging warnings during evaluation.
func Evaluate(l log.Logger, expr Expression, components component.Components) (component.Components, error) {
	if expr == nil {
		return nil, NewEvaluationError("expression is nil")
	}

	switch node := expr.(type) {
	case *PathExpression:
		return evaluatePathFilter(node, components)
	case *AttributeExpression:
		return evaluateAttributeFilter(node, components)
	case *PrefixExpression:
		return evaluatePrefixExpression(l, node, components)
	case *InfixExpression:
		return evaluateInfixExpression(l, node, components)
	case *GraphExpression:
		return evaluateGraphExpression(l, node, components)
	case *GitExpression:
		return evaluateGitFilter(node, components)
	default:
		return nil, NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilter evaluates a path filter using glob matching.
func evaluatePathFilter(filter *PathExpression, components component.Components) (component.Components, error) {
	result := make(component.Components, 0, len(components))

	for _, c := range components {
		matches, err := matchPath(c, filter)
		if err != nil {
			return nil, NewEvaluationErrorWithCause("failed to match path pattern: "+filter.Value, err)
		}

		if matches {
			result = append(result, c)
		}
	}

	return result, nil
}

// evaluateAttributeFilter evaluates an attribute filter.
func evaluateAttributeFilter(filter *AttributeExpression, components []component.Component) ([]component.Component, error) {
	var result []component.Component

	switch filter.Key {
	case AttributeName:
		g, err := filter.CompileGlob()
		if err != nil {
			return nil, NewEvaluationErrorWithCause("failed to compile glob pattern for name filter: "+filter.Value, err)
		}

		for _, c := range components {
			if g.Match(filepath.Base(c.Path())) {
				result = append(result, c)
			}
		}

	case AttributeType:
		switch filter.Value {
		case AttributeTypeValueUnit:
			for _, c := range components {
				if _, ok := c.(*component.Unit); ok {
					result = append(result, c)
				}
			}
		case AttributeTypeValueStack:
			for _, c := range components {
				if _, ok := c.(*component.Stack); ok {
					result = append(result, c)
				}
			}
		default:
			return nil, NewEvaluationError("invalid type value: " + filter.Value + " (expected 'unit' or 'stack')")
		}
	case AttributeExternal:
		switch filter.Value {
		case AttributeExternalValueTrue:
			for _, c := range components {
				if c.External() {
					result = append(result, c)
				}
			}
		case AttributeExternalValueFalse:
			for _, c := range components {
				if !c.External() {
					result = append(result, c)
				}
			}
		default:
			return nil, NewEvaluationError("invalid external value: " + filter.Value + " (expected 'true' or 'false')")
		}
	case AttributeReading:
		g, err := filter.CompileGlob()
		if err != nil {
			return nil, NewEvaluationErrorWithCause("failed to compile glob pattern for reading filter: "+filter.Value, err)
		}

		for _, c := range components {
			if slices.ContainsFunc(c.Reading(), g.Match) {
				result = append(result, c)

				continue
			}

			discoveryCtx := c.DiscoveryContext()
			if discoveryCtx == nil || discoveryCtx.WorkingDir == "" {
				continue
			}

			relReading := make([]string, 0, len(c.Reading()))
			for _, reading := range c.Reading() {
				rel, err := filepath.Rel(c.DiscoveryContext().WorkingDir, reading)
				if err != nil {
					return nil, NewEvaluationErrorWithCause(fmt.Sprintf("failed to get relative path for component %s reading: %s", c.Path(), reading), err)
				}

				relReading = append(relReading, filepath.ToSlash(rel))
			}

			if slices.ContainsFunc(relReading, g.Match) {
				result = append(result, c)
			}
		}
	case AttributeSource:
		g, err := filter.CompileGlob()
		if err != nil {
			return nil, NewEvaluationErrorWithCause("failed to compile glob pattern for source filter: "+filter.Value, err)
		}

		for _, c := range components {
			if slices.ContainsFunc(c.Sources(), g.Match) {
				result = append(result, c)
			}
		}
	default:
		return nil, NewEvaluationError("unknown attribute key: " + filter.Key)
	}

	return result, nil
}

// evaluatePrefixExpression evaluates a prefix expression (negation).
func evaluatePrefixExpression(l log.Logger, expr *PrefixExpression, components component.Components) (component.Components, error) {
	if expr.Operator != "!" {
		return nil, NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	toExclude, err := Evaluate(l, expr.Right, components)
	if err != nil {
		return nil, err
	}

	if len(toExclude) == 0 {
		return components, nil
	}

	// Build a set of paths to exclude for efficient lookup.
	// We compare by path rather than object identity because graph traversal
	// may return component instances from Dependencies()/Dependents() that are
	// different objects than those in the input list.
	excludePaths := make(map[string]struct{}, len(toExclude))
	for _, c := range toExclude {
		excludePaths[c.Path()] = struct{}{}
	}

	// We don't use slices.DeleteFunc here because we don't want the members of the original components slice to be
	// zeroed.
	results := make(component.Components, 0, len(components)-len(toExclude))

	for _, c := range components {
		if _, excluded := excludePaths[c.Path()]; excluded {
			continue
		}

		results = append(results, c)
	}

	return results, nil
}

// evaluateInfixExpression evaluates an infix expression (intersection).
func evaluateInfixExpression(l log.Logger, expr *InfixExpression, components component.Components) (component.Components, error) {
	if expr.Operator != "|" {
		return nil, NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	leftResult, err := Evaluate(l, expr.Left, components)
	if err != nil {
		return nil, err
	}

	rightResult, err := Evaluate(l, expr.Right, leftResult)
	if err != nil {
		return nil, err
	}

	return rightResult, nil
}

// evaluateGraphExpression evaluates a graph expression by traversing dependency/dependent graphs.
func evaluateGraphExpression(l log.Logger, expr *GraphExpression, components component.Components) (component.Components, error) {
	targetMatches, err := Evaluate(l, expr.Target, components)
	if err != nil {
		return nil, err
	}

	// NOTE: We previously filtered out components with OriginGraphDiscovery here to avoid
	// including components that were only discovered via graph relationships. However, this
	// caused issues with intersection filters like "service... | !^db..." where db is
	// discovered via the first filter and then needs to be used as a target in the second.
	// The discovery phase already handles this logic properly, so we don't need to filter
	// by origin here during filter evaluation.

	if len(targetMatches) == 0 {
		return component.Components{}, nil
	}

	resultSet := make(map[string]component.Component)

	if !expr.ExcludeTarget {
		for _, c := range targetMatches {
			resultSet[c.Path()] = c
		}
	}

	visited := make(map[string]int)

	if expr.IncludeDependencies {
		depth := MaxTraversalDepth
		warnOnLimit := true

		if expr.DependencyDepth > 0 {
			depth = expr.DependencyDepth
			warnOnLimit = false
		}

		for _, target := range targetMatches {
			traverseGraph(l, target, resultSet, visited, graphDirectionDependencies, depth, warnOnLimit)
		}
	}

	visited = make(map[string]int)

	if expr.IncludeDependents {
		depth := MaxTraversalDepth
		warnOnLimit := true

		if expr.DependentDepth > 0 {
			depth = expr.DependentDepth
			warnOnLimit = false
		}

		for _, target := range targetMatches {
			traverseGraph(l, target, resultSet, visited, graphDirectionDependents, depth, warnOnLimit)
		}
	}

	result := make(component.Components, 0, len(resultSet))
	for _, c := range resultSet {
		result = append(result, c)
	}

	return result, nil
}

// evaluateGitFilter evaluates a Git filter expression by comparing components between Git references.
// It returns components that were added, removed, or changed between FromRef and ToRef.
func evaluateGitFilter(filter *GitExpression, components component.Components) (component.Components, error) {
	results := make(component.Components, 0, len(components))

	for _, c := range components {
		discoveryCtx := c.DiscoveryContext()
		if discoveryCtx == nil || discoveryCtx.Ref == "" {
			continue
		}

		if discoveryCtx.Ref == filter.FromRef || discoveryCtx.Ref == filter.ToRef {
			results = append(results, c)
		}
	}

	return results, nil
}

// graphDirection represents the direction of graph traversal.
type graphDirection int

const (
	graphDirectionDependencies graphDirection = iota
	graphDirectionDependents
)

func (d graphDirection) String() string {
	switch d {
	case graphDirectionDependencies:
		return "dependencies"
	case graphDirectionDependents:
		return "dependents"
	}

	return "unknown"
}

// traverseGraph recursively traverses the graph in the specified direction (dependencies or dependents).
// The visited map tracks the maximum remaining depth at which each node was visited, allowing re-traversal
// when a node is reached with more remaining depth (e.g., from a closer target).
// The warnOnLimit flag controls whether to log a warning when depth is exhausted (used for safety limits only).
func traverseGraph(
	l log.Logger,
	c component.Component,
	resultSet map[string]component.Component,
	visited map[string]int,
	direction graphDirection,
	remainingDepth int,
	warnOnLimit bool,
) {
	if remainingDepth <= 0 {
		if l != nil && warnOnLimit {
			directionName := direction.String()

			l.Warnf(
				"Maximum %s traversal depth (%d) reached for component %s during filtering. Some %s may have been excluded from results.",
				directionName,
				MaxTraversalDepth,
				c.Path(),
				directionName,
			)
		}

		return
	}

	path := c.Path()

	if prevDepth, seen := visited[path]; seen && prevDepth >= remainingDepth {
		return
	}

	visited[path] = remainingDepth

	var relatedComponents []component.Component
	if direction == graphDirectionDependencies {
		relatedComponents = c.Dependencies()
	} else {
		relatedComponents = c.Dependents()
	}

	for _, related := range relatedComponents {
		relatedPath := related.Path()

		// It's not clear why this isn't necessary. It might be in the future.
		// Tests pass without it, however, so we'll leave it out for now.
		//
		// Needs more investigation.
		//
		// relatedCtx := related.DiscoveryContext()
		// if relatedCtx != nil {
		// 	origin := relatedCtx.Origin()
		// 	if origin != component.OriginGraphDiscovery {
		// 		l.Debugf(
		// 			"Skipping %s %s in graph expression traversal: component was discovered via %s, not graph discovery",
		// 			direction.String(),
		// 			relatedPath,
		// 			origin,
		// 		)

		// 		continue
		// 	}
		// }

		resultSet[relatedPath] = related

		traverseGraph(l, related, resultSet, visited, direction, remainingDepth-1, warnOnLimit)
	}
}
