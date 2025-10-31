package filter

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	AttributeName     = "name"
	AttributeType     = "type"
	AttributeExternal = "external"
	AttributeReading  = "reading"

	AttributeTypeValueUnit  = string(component.UnitKind)
	AttributeTypeValueStack = string(component.StackKind)

	AttributeExternalValueTrue  = "true"
	AttributeExternalValueFalse = "false"

	// MaxTraversalDepth is the maximum depth to traverse the graph for both dependencies and dependents.
	MaxTraversalDepth = 1000000
)

// Evaluate evaluates an expression against a list of components and returns the filtered components.
// If logger is provided, it will be used for logging warnings during evaluation.
func Evaluate(l log.Logger, expr Expression, components component.Components) (component.Components, error) {
	if expr == nil {
		return nil, NewEvaluationError("expression is nil")
	}

	return evaluate(l, expr, components)
}

// evaluate is the internal recursive evaluation function.
func evaluate(l log.Logger, expr Expression, components component.Components) (component.Components, error) {
	switch node := expr.(type) {
	case *PathFilter:
		return evaluatePathFilter(node, components)
	case *AttributeFilter:
		return evaluateAttributeFilter(node, components)
	case *PrefixExpression:
		return evaluatePrefixExpression(l, node, components)
	case *InfixExpression:
		return evaluateInfixExpression(l, node, components)
	case *GraphExpression:
		return evaluateGraphExpression(l, node, components)
	default:
		return nil, NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilter evaluates a path filter using glob matching.
func evaluatePathFilter(filter *PathFilter, components component.Components) (component.Components, error) {
	g, err := filter.CompileGlob()
	if err != nil {
		return nil, NewEvaluationErrorWithCause("failed to compile glob pattern: "+filter.Value, err)
	}

	var result component.Components

	for _, component := range components {
		normalizedPath := component.Path()
		if !filepath.IsAbs(normalizedPath) {
			normalizedPath = filepath.Join(filter.WorkingDir, normalizedPath)
		}

		normalizedPath = filepath.ToSlash(normalizedPath)

		if g.Match(normalizedPath) {
			result = append(result, component)
		}
	}

	return result, nil
}

// evaluateAttributeFilter evaluates an attribute filter.
func evaluateAttributeFilter(filter *AttributeFilter, components []component.Component) ([]component.Component, error) {
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
			for _, readFile := range c.Reading() {
				normalizedPath := readFile
				if !filepath.IsAbs(normalizedPath) {
					normalizedPath = filepath.Join(filter.WorkingDir, normalizedPath)
				}

				normalizedPath = filepath.ToSlash(normalizedPath)

				if g.Match(normalizedPath) {
					result = append(result, c)
					break
				}
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

	toExclude, err := evaluate(l, expr.Right, components)
	if err != nil {
		return nil, err
	}

	excludeSet := make(map[string]struct{}, len(toExclude))
	for _, c := range toExclude {
		excludeSet[c.Path()] = struct{}{}
	}

	var result component.Components

	for _, c := range components {
		if _, ok := excludeSet[c.Path()]; !ok {
			result = append(result, c)
		}
	}

	return result, nil
}

// evaluateInfixExpression evaluates an infix expression (intersection).
func evaluateInfixExpression(l log.Logger, expr *InfixExpression, components component.Components) (component.Components, error) {
	if expr.Operator != "|" {
		return nil, NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	leftResult, err := evaluate(l, expr.Left, components)
	if err != nil {
		return nil, err
	}

	rightResult, err := evaluate(l, expr.Right, leftResult)
	if err != nil {
		return nil, err
	}

	return rightResult, nil
}

// evaluateGraphExpression evaluates a graph expression by traversing dependency/dependent graphs.
func evaluateGraphExpression(l log.Logger, expr *GraphExpression, components component.Components) (component.Components, error) {
	targetMatches, err := evaluate(l, expr.Target, components)
	if err != nil {
		return nil, err
	}

	if len(targetMatches) == 0 {
		return component.Components{}, nil
	}

	resultSet := make(map[string]component.Component)

	if !expr.ExcludeTarget {
		for _, c := range targetMatches {
			resultSet[c.Path()] = c
		}
	}

	visited := make(map[string]bool)

	if expr.IncludeDependencies {
		for _, target := range targetMatches {
			traverseDependencies(l, target, resultSet, visited, MaxTraversalDepth)
		}
	}

	visited = make(map[string]bool)

	if expr.IncludeDependents {
		for _, target := range targetMatches {
			traverseDependents(l, target, resultSet, visited, MaxTraversalDepth)
		}
	}

	result := make(component.Components, 0, len(resultSet))
	for _, c := range resultSet {
		result = append(result, c)
	}

	return result, nil
}

// traverseDependencies recursively traverses the dependency graph downward (from a component to its dependencies).
func traverseDependencies(
	l log.Logger,
	c component.Component,
	resultSet map[string]component.Component,
	visited map[string]bool,
	maxDepth int,
) {
	if maxDepth <= 0 {
		if l != nil {
			l.Warnf(
				"Maximum dependency traversal depth (%d) reached for component %s during filtering. Some dependencies may have been excluded from results.",
				MaxTraversalDepth,
				c.Path(),
			)
		}

		return
	}

	path := c.Path()
	if visited[path] {
		return
	}

	visited[path] = true

	for _, dep := range c.Dependencies() {
		depPath := dep.Path()
		resultSet[depPath] = dep

		traverseDependencies(l, dep, resultSet, visited, maxDepth-1)
	}
}

// traverseDependents recursively traverses the dependent graph upward (from a component to its dependents).
func traverseDependents(
	l log.Logger,
	c component.Component,
	resultSet map[string]component.Component,
	visited map[string]bool,
	maxDepth int,
) {
	if maxDepth <= 0 {
		if l != nil {
			l.Warnf(
				"Maximum dependent traversal depth (%d) reached for component %s during filtering. Some dependents may have been excluded from results.",
				MaxTraversalDepth,
				c.Path(),
			)
		}

		return
	}

	path := c.Path()
	if visited[path] {
		return
	}

	visited[path] = true

	for _, dependent := range c.Dependents() {
		depPath := dependent.Path()
		resultSet[depPath] = dependent

		traverseDependents(l, dependent, resultSet, visited, maxDepth-1)
	}
}
