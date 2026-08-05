package filter

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

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

// graphTraversalParams consolidates parameters for filter graph traversal.
type graphTraversalParams struct {
	resultSet   map[string]component.Component
	visited     map[string]int
	boundary    string
	direction   GraphDirection
	warnOnLimit bool
}

// EvaluationContext carries the discovery settings that graph traversal has to
// honor. The zero value traverses the whole component graph.
type EvaluationContext struct {
	WorkingDir        string
	DiscoveryBoundary string
}

// graphBoundary returns the directory confining traversal for one direction of
// a graph expression, empty when that direction is unbounded. An inline "(dir)"
// operand overrides the discovery boundary, and is resolved against the working
// directory the same way discovery resolves it.
func (c EvaluationContext) graphBoundary(bound GraphBound) string {
	if bound.Boundary == "" {
		return c.DiscoveryBoundary
	}

	if filepath.IsAbs(bound.Boundary) {
		return filepath.Clean(bound.Boundary)
	}

	return filepath.Clean(filepath.Join(c.WorkingDir, bound.Boundary))
}

// outsideBoundary reports whether path falls outside boundary. An empty
// boundary bounds nothing.
func outsideBoundary(boundary, path string) bool {
	if boundary == "" {
		return false
	}

	rel, err := filepath.Rel(boundary, filepath.Clean(path))
	if err != nil {
		return true
	}

	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// Evaluate evaluates an expression against a list of components and returns the filtered components.
// If logger is provided, it will be used for logging warnings during evaluation.
func Evaluate(
	l log.Logger,
	evalCtx EvaluationContext,
	expr Expression,
	components component.Components,
) (component.Components, error) {
	if expr == nil {
		return nil, NewEvaluationError("expression is nil")
	}

	switch node := expr.(type) {
	case *PathExpression:
		return evaluatePathFilter(l, node, components)
	case *AttributeExpression:
		return evaluateAttributeFilter(l, node, components)
	case *PrefixExpression:
		return evaluatePrefixExpression(l, evalCtx, node, components)
	case *InfixExpression:
		return evaluateInfixExpression(l, evalCtx, node, components)
	case *GraphExpression:
		return evaluateGraphExpression(l, evalCtx, node, components)
	case *GitExpression:
		return evaluateGitFilter(node, components)
	default:
		return nil, NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilter evaluates a path filter using glob matching.
func evaluatePathFilter(
	l log.Logger,
	filter *PathExpression,
	components component.Components,
) (component.Components, error) {
	result := make(component.Components, 0, len(components))

	for _, c := range components {
		if matchPath(c, filter) {
			result = append(result, c)

			continue
		}

		traceFilterMiss(l, filter, c)
	}

	return result, nil
}

// evaluateAttributeFilter evaluates an attribute filter.
func evaluateAttributeFilter(
	l log.Logger,
	filter *AttributeExpression,
	components []component.Component,
) ([]component.Component, error) {
	var result []component.Component

	switch filter.Key {
	case AttributeName:
		g := filter.Glob()

		for _, c := range components {
			if g.Match(filepath.Base(c.Path())) {
				result = append(result, c)

				continue
			}

			traceFilterMiss(l, filter, c)
		}

	case AttributeType:
		switch filter.Value {
		case AttributeTypeValueUnit:
			for _, c := range components {
				if _, ok := c.(*component.Unit); ok {
					result = append(result, c)

					continue
				}

				traceFilterMiss(l, filter, c)
			}
		case AttributeTypeValueStack:
			for _, c := range components {
				if _, ok := c.(*component.Stack); ok {
					result = append(result, c)

					continue
				}

				traceFilterMiss(l, filter, c)
			}
		default:
			return nil, NewEvaluationError(
				"invalid type value: " + filter.Value + " (expected 'unit' or 'stack')",
			)
		}
	case AttributeExternal:
		switch filter.Value {
		case AttributeExternalValueTrue:
			for _, c := range components {
				if c.External() {
					result = append(result, c)

					continue
				}

				traceFilterMiss(l, filter, c)
			}
		case AttributeExternalValueFalse:
			for _, c := range components {
				if !c.External() {
					result = append(result, c)

					continue
				}

				traceFilterMiss(l, filter, c)
			}
		default:
			return nil, NewEvaluationError(
				"invalid external value: " + filter.Value + " (expected 'true' or 'false')",
			)
		}
	case AttributeReading:
		g := filter.Glob()

		for _, c := range components {
			// Read paths are OS-native; the glob is '/'-separated, so normalize first.
			if slices.ContainsFunc(c.Reading(), func(reading string) bool {
				return g.Match(filepath.ToSlash(reading))
			}) {
				result = append(result, c)

				continue
			}

			discoveryCtx := c.DiscoveryContext()
			if discoveryCtx == nil || discoveryCtx.WorkingDir == "" {
				traceFilterMiss(l, filter, c)

				continue
			}

			relReading := make([]string, 0, len(c.Reading()))
			for _, reading := range c.Reading() {
				rel, err := filepath.Rel(c.DiscoveryContext().WorkingDir, reading)
				if err != nil {
					return nil, NewEvaluationErrorWithCause(
						fmt.Sprintf(
							"failed to get relative path for component %s reading: %s",
							c.Path(),
							reading,
						),
						err,
					)
				}

				relReading = append(relReading, filepath.ToSlash(rel))
			}

			if slices.ContainsFunc(relReading, g.Match) {
				result = append(result, c)

				continue
			}

			traceFilterMiss(l, filter, c)
		}
	case AttributeSource:
		g := filter.Glob()

		for _, c := range components {
			// terraform.source can carry native or mixed separators on Windows, so
			// normalize before matching the '/'-separated glob.
			if slices.ContainsFunc(c.Sources(), func(source string) bool {
				return g.Match(filepath.ToSlash(source))
			}) {
				result = append(result, c)

				continue
			}

			traceFilterMiss(l, filter, c)
		}
	default:
		return nil, NewEvaluationError("unknown attribute key: " + filter.Key)
	}

	return result, nil
}

// traceFilterMiss emits a trace log when a component does not match a filter.
func traceFilterMiss(l log.Logger, expr Expression, c component.Component) {
	l.Tracef("Filter %s did not match component %s", expr, c.Path())
}

// evaluatePrefixExpression evaluates a prefix expression (negation).
func evaluatePrefixExpression(
	l log.Logger,
	evalCtx EvaluationContext,
	expr *PrefixExpression,
	components component.Components,
) (component.Components, error) {
	if expr.Operator != "!" {
		return nil, NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	toExclude, err := Evaluate(l, evalCtx, expr.Right, components)
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
func evaluateInfixExpression(
	l log.Logger,
	evalCtx EvaluationContext,
	expr *InfixExpression,
	components component.Components,
) (component.Components, error) {
	if expr.Operator != "|" {
		return nil, NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	leftResult, err := Evaluate(l, evalCtx, expr.Left, components)
	if err != nil {
		return nil, err
	}

	rightResult, err := Evaluate(l, evalCtx, expr.Right, leftResult)
	if err != nil {
		return nil, err
	}

	return rightResult, nil
}

// evaluateGraphExpression evaluates a graph expression by traversing dependency/dependent graphs.
func evaluateGraphExpression(
	l log.Logger,
	evalCtx EvaluationContext,
	expr *GraphExpression,
	components component.Components,
) (component.Components, error) {
	targetMatches, err := Evaluate(l, evalCtx, expr.Target, components)
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

	if expr.Dependencies.Include {
		depth := MaxTraversalDepth
		warnOnLimit := true

		if expr.Dependencies.Depth > 0 {
			depth = expr.Dependencies.Depth
			warnOnLimit = false
		}

		params := &graphTraversalParams{
			resultSet:   resultSet,
			visited:     make(map[string]int),
			boundary:    evalCtx.graphBoundary(expr.Dependencies),
			direction:   GraphDirectionDependencies,
			warnOnLimit: warnOnLimit,
		}

		for _, target := range targetMatches {
			traverseGraph(l, target, params, depth)
		}
	}

	if expr.Dependents.Include {
		depth := MaxTraversalDepth
		warnOnLimit := true

		if expr.Dependents.Depth > 0 {
			depth = expr.Dependents.Depth
			warnOnLimit = false
		}

		params := &graphTraversalParams{
			resultSet:   resultSet,
			visited:     make(map[string]int),
			boundary:    evalCtx.graphBoundary(expr.Dependents),
			direction:   GraphDirectionDependents,
			warnOnLimit: warnOnLimit,
		}

		for _, target := range targetMatches {
			traverseGraph(l, target, params, depth)
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
func evaluateGitFilter(
	filter *GitExpression,
	components component.Components,
) (component.Components, error) {
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

// traverseGraph recursively traverses the graph in the specified direction (dependencies or dependents).
// The visited map tracks the maximum remaining depth at which each node was visited, allowing re-traversal
// when a node is reached with more remaining depth (e.g., from a closer target).
// The warnOnLimit flag controls whether to log a warning when depth is exhausted (used for safety limits only).
func traverseGraph(
	l log.Logger,
	c component.Component,
	params *graphTraversalParams,
	remainingDepth int,
) {
	if remainingDepth <= 0 {
		if params.warnOnLimit {
			directionName := params.direction.String()

			l.Warnf(
				"Maximum %s traversal depth (%d) reached for component %s during filtering. "+
					"Some %s may have been excluded from results.",
				directionName,
				MaxTraversalDepth,
				c.Path(),
				directionName,
			)
		}

		return
	}

	path := c.Path()

	if prevDepth, seen := params.visited[path]; seen && prevDepth >= remainingDepth {
		return
	}

	params.visited[path] = remainingDepth

	var relatedComponents []component.Component
	if params.direction == GraphDirectionDependencies {
		relatedComponents = c.Dependencies()
	} else {
		relatedComponents = c.Dependents()
	}

	for _, related := range relatedComponents {
		relatedPath := related.Path()

		// A component reachable only by passing through the boundary is
		// excluded along with the hop that leaves it, so traversal stops here
		// rather than skipping the hop and continuing past it.
		if outsideBoundary(params.boundary, relatedPath) {
			l.Debugf(
				"%s %s is outside discovery boundary %s; skipping",
				params.direction,
				relatedPath,
				params.boundary,
			)

			continue
		}

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

		params.resultSet[relatedPath] = related

		traverseGraph(l, related, params, remainingDepth-1)
	}
}
