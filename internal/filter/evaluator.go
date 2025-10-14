package filter

import (
	"path/filepath"
)

// Evaluate evaluates an expression against a list of units and returns the filtered units.
func Evaluate(expr Expression, units []Unit) ([]Unit, error) {
	if expr == nil {
		return nil, NewEvaluationError("expression is nil")
	}

	return evaluate(expr, units)
}

// evaluate is the internal recursive evaluation function.
func evaluate(expr Expression, units []Unit) ([]Unit, error) {
	switch node := expr.(type) {
	case *PathFilter:
		return evaluatePathFilter(node, units)
	case *AttributeFilter:
		return evaluateAttributeFilter(node, units)
	case *PrefixExpression:
		return evaluatePrefixExpression(node, units)
	case *InfixExpression:
		return evaluateInfixExpression(node, units)
	default:
		return nil, NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilter evaluates a path filter using glob matching.
func evaluatePathFilter(filter *PathFilter, units []Unit) ([]Unit, error) {
	// Get the compiled glob (compiled once and cached)
	g, err := filter.CompileGlob()
	if err != nil {
		return nil, NewEvaluationErrorWithCause("failed to compile glob pattern: "+filter.Value, err)
	}

	var result []Unit
	for _, unit := range units {
		// Normalize the unit path for matching
		normalizedPath := filepath.ToSlash(unit.Path)

		if g.Match(normalizedPath) {
			result = append(result, unit)
		}
	}

	return result, nil
}

// evaluateAttributeFilter evaluates an attribute filter.
func evaluateAttributeFilter(filter *AttributeFilter, units []Unit) ([]Unit, error) {
	var result []Unit

	switch filter.Key {
	case "name":
		// Match by unit name
		for _, unit := range units {
			if unit.Name == filter.Value {
				result = append(result, unit)
			}
		}
	case "type":
		// For future extensibility - currently all are "unit"
		// In the future this could distinguish between units and stacks
		if filter.Value == "unit" {
			// All units match "type=unit" for now
			result = append(result, units...)
		}
	default:
		return nil, NewEvaluationError("unknown attribute key: " + filter.Key)
	}

	return result, nil
}

// evaluatePrefixExpression evaluates a prefix expression (negation).
func evaluatePrefixExpression(expr *PrefixExpression, units []Unit) ([]Unit, error) {
	if expr.Operator != "!" {
		return nil, NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	// Evaluate the right side to get units to exclude
	toExclude, err := evaluate(expr.Right, units)
	if err != nil {
		return nil, err
	}

	// Create a set of paths to exclude for efficient lookup
	excludeSet := make(map[string]bool, len(toExclude))
	for _, unit := range toExclude {
		excludeSet[unit.Path] = true
	}

	// Return all units NOT in the exclude set
	var result []Unit
	for _, unit := range units {
		if !excludeSet[unit.Path] {
			result = append(result, unit)
		}
	}

	return result, nil
}

// evaluateInfixExpression evaluates an infix expression (union).
func evaluateInfixExpression(expr *InfixExpression, units []Unit) ([]Unit, error) {
	if expr.Operator != "|" {
		return nil, NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	// Evaluate left side
	leftResult, err := evaluate(expr.Left, units)
	if err != nil {
		return nil, err
	}

	// Evaluate right side
	rightResult, err := evaluate(expr.Right, units)
	if err != nil {
		return nil, err
	}

	// Return the union (deduplicated)
	return unionUnits(leftResult, rightResult), nil
}

// unionUnits returns the union of two unit slices, removing duplicates based on path.
func unionUnits(left, right []Unit) []Unit {
	// Use a map to track unique paths
	seen := make(map[string]bool)
	var result []Unit

	// Add all units from left
	for _, unit := range left {
		if !seen[unit.Path] {
			seen[unit.Path] = true
			result = append(result, unit)
		}
	}

	// Add units from right that aren't already in the result
	for _, unit := range right {
		if !seen[unit.Path] {
			seen[unit.Path] = true
			result = append(result, unit)
		}
	}

	return result
}
