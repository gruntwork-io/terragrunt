package filter

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
)

const (
	AttributeName     = "name"
	AttributeType     = "type"
	AttributeExternal = "external"
	AttributeReading  = "reading"

	AttributeTypeValueUnit  = string(component.Unit)
	AttributeTypeValueStack = string(component.Stack)

	AttributeExternalValueTrue  = "true"
	AttributeExternalValueFalse = "false"
)

// Evaluate evaluates an expression against a list of components and returns the filtered components.
func Evaluate(expr Expression, components component.Components) (component.Components, error) {
	if expr == nil {
		return nil, NewEvaluationError("expression is nil")
	}

	return evaluate(expr, components)
}

// evaluate is the internal recursive evaluation function.
func evaluate(expr Expression, components component.Components) (component.Components, error) {
	switch node := expr.(type) {
	case *PathFilter:
		return evaluatePathFilter(node, components)
	case *AttributeFilter:
		return evaluateAttributeFilter(node, components)
	case *PrefixExpression:
		return evaluatePrefixExpression(node, components)
	case *InfixExpression:
		return evaluateInfixExpression(node, components)
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
		normalizedPath := component.Path
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
func evaluateAttributeFilter(filter *AttributeFilter, components []*component.Component) ([]*component.Component, error) {
	var result []*component.Component

	switch filter.Key {
	case AttributeName:
		if strings.ContainsAny(filter.Value, "*?[]") {
			g, err := filter.CompileGlob()
			if err != nil {
				return nil, NewEvaluationErrorWithCause("failed to compile glob pattern for name filter: "+filter.Value, err)
			}

			for _, c := range components {
				if g.Match(filepath.Base(c.Path)) {
					result = append(result, c)
				}
			}

			break
		}

		for _, c := range components {
			if filepath.Base(c.Path) == filter.Value {
				result = append(result, c)
			}
		}

	case AttributeType:
		switch filter.Value {
		case AttributeTypeValueUnit:
			for _, c := range components {
				if c.Kind == component.Unit {
					result = append(result, c)
				}
			}
		case AttributeTypeValueStack:
			for _, c := range components {
				if c.Kind == component.Stack {
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
				if c.External {
					result = append(result, c)
				}
			}
		case AttributeExternalValueFalse:
			for _, c := range components {
				if !c.External {
					result = append(result, c)
				}
			}
		default:
			return nil, NewEvaluationError("invalid external value: " + filter.Value + " (expected 'true' or 'false')")
		}
	case AttributeReading:
		if strings.ContainsAny(filter.Value, "*?[]") {
			g, err := filter.CompileGlob()
			if err != nil {
				return nil, NewEvaluationErrorWithCause("failed to compile glob pattern for reading filter: "+filter.Value, err)
			}

			for _, c := range components {
				if slices.ContainsFunc(c.Reading, g.Match) {
					result = append(result, c)
				}
			}

			break
		}

		for _, c := range components {
			if slices.Contains(c.Reading, filter.Value) {
				result = append(result, c)
			}
		}
	default:
		return nil, NewEvaluationError("unknown attribute key: " + filter.Key)
	}

	return result, nil
}

// evaluatePrefixExpression evaluates a prefix expression (negation).
func evaluatePrefixExpression(expr *PrefixExpression, components component.Components) (component.Components, error) {
	if expr.Operator != "!" {
		return nil, NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	toExclude, err := evaluate(expr.Right, components)
	if err != nil {
		return nil, err
	}

	excludeSet := make(map[string]struct{}, len(toExclude))
	for _, c := range toExclude {
		excludeSet[c.Path] = struct{}{}
	}

	var result component.Components

	for _, c := range components {
		if _, ok := excludeSet[c.Path]; !ok {
			result = append(result, c)
		}
	}

	return result, nil
}

// evaluateInfixExpression evaluates an infix expression (intersection).
func evaluateInfixExpression(expr *InfixExpression, components component.Components) (component.Components, error) {
	if expr.Operator != "|" {
		return nil, NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	leftResult, err := evaluate(expr.Left, components)
	if err != nil {
		return nil, err
	}

	rightResult, err := evaluate(expr.Right, leftResult)
	if err != nil {
		return nil, err
	}

	return rightResult, nil
}
