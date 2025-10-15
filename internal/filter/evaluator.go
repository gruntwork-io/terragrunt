package filter

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/discoveredconfig"
)

// Evaluate evaluates an expression against a list of configs and returns the filtered configs.
func Evaluate(expr Expression, configs []*discoveredconfig.DiscoveredConfig) ([]*discoveredconfig.DiscoveredConfig, error) {
	if expr == nil {
		return nil, NewEvaluationError("expression is nil")
	}

	return evaluate(expr, configs)
}

// evaluate is the internal recursive evaluation function.
func evaluate(expr Expression, configs []*discoveredconfig.DiscoveredConfig) ([]*discoveredconfig.DiscoveredConfig, error) {
	switch node := expr.(type) {
	case *PathFilter:
		return evaluatePathFilter(node, configs)
	case *AttributeFilter:
		return evaluateAttributeFilter(node, configs)
	case *PrefixExpression:
		return evaluatePrefixExpression(node, configs)
	case *InfixExpression:
		return evaluateInfixExpression(node, configs)
	default:
		return nil, NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilter evaluates a path filter using glob matching.
func evaluatePathFilter(filter *PathFilter, configs []*discoveredconfig.DiscoveredConfig) ([]*discoveredconfig.DiscoveredConfig, error) {
	// Get the compiled glob (compiled once and cached)
	g, err := filter.CompileGlob()
	if err != nil {
		return nil, NewEvaluationErrorWithCause("failed to compile glob pattern: "+filter.Value, err)
	}

	var result []*discoveredconfig.DiscoveredConfig

	for _, cfg := range configs {
		// Normalize the config path for matching
		normalizedPath := filepath.ToSlash(cfg.Path)

		if g.Match(normalizedPath) {
			result = append(result, cfg)
		}
	}

	return result, nil
}

// evaluateAttributeFilter evaluates an attribute filter.
func evaluateAttributeFilter(filter *AttributeFilter, configs []*discoveredconfig.DiscoveredConfig) ([]*discoveredconfig.DiscoveredConfig, error) {
	var result []*discoveredconfig.DiscoveredConfig

	switch filter.Key {
	case "name":
		// Match by config name (derived from directory basename)
		for _, cfg := range configs {
			if filepath.Base(cfg.Path) == filter.Value {
				result = append(result, cfg)
			}
		}
	case "type":
		// Match by config type (unit or stack)
		switch filter.Value {
		case string(discoveredconfig.ConfigTypeUnit):
			for _, cfg := range configs {
				if cfg.Type == discoveredconfig.ConfigTypeUnit {
					result = append(result, cfg)
				}
			}
		case string(discoveredconfig.ConfigTypeStack):
			for _, cfg := range configs {
				if cfg.Type == discoveredconfig.ConfigTypeStack {
					result = append(result, cfg)
				}
			}
		default:
			return nil, NewEvaluationError("invalid type value: " + filter.Value + " (expected 'unit' or 'stack')")
		}
	case "external":
		// Match by external flag
		switch filter.Value {
		case "true":
			for _, cfg := range configs {
				if cfg.External {
					result = append(result, cfg)
				}
			}
		case "false":
			for _, cfg := range configs {
				if !cfg.External {
					result = append(result, cfg)
				}
			}
		default:
			return nil, NewEvaluationError("invalid external value: " + filter.Value + " (expected 'true' or 'false')")
		}
	default:
		return nil, NewEvaluationError("unknown attribute key: " + filter.Key)
	}

	return result, nil
}

// evaluatePrefixExpression evaluates a prefix expression (negation).
func evaluatePrefixExpression(expr *PrefixExpression, configs []*discoveredconfig.DiscoveredConfig) ([]*discoveredconfig.DiscoveredConfig, error) {
	if expr.Operator != "!" {
		return nil, NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	// Evaluate the right side to get configs to exclude
	toExclude, err := evaluate(expr.Right, configs)
	if err != nil {
		return nil, err
	}

	// Create a set of paths to exclude for efficient lookup
	excludeSet := make(map[string]bool, len(toExclude))
	for _, cfg := range toExclude {
		excludeSet[cfg.Path] = true
	}

	// Return all configs NOT in the exclude set
	var result []*discoveredconfig.DiscoveredConfig

	for _, cfg := range configs {
		if !excludeSet[cfg.Path] {
			result = append(result, cfg)
		}
	}

	return result, nil
}

// evaluateInfixExpression evaluates an infix expression (intersection).
func evaluateInfixExpression(expr *InfixExpression, configs []*discoveredconfig.DiscoveredConfig) ([]*discoveredconfig.DiscoveredConfig, error) {
	if expr.Operator != "|" {
		return nil, NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	// Evaluate left side
	leftResult, err := evaluate(expr.Left, configs)
	if err != nil {
		return nil, err
	}

	// Evaluate right side against the left result (refine/narrow)
	// The right filter only evaluates configs that passed the left filter
	rightResult, err := evaluate(expr.Right, leftResult)
	if err != nil {
		return nil, err
	}

	// Return the intersection (configs that passed both filters)
	return rightResult, nil
}
