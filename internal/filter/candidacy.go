package filter

// GraphDirection represents the direction of graph traversal.
type GraphDirection int

const (
	// GraphDirectionNone indicates no graph traversal.
	GraphDirectionNone GraphDirection = iota
	// GraphDirectionDependencies indicates traversing dependencies (downstream).
	GraphDirectionDependencies
	// GraphDirectionDependents indicates traversing dependents (upstream).
	GraphDirectionDependents
	// GraphDirectionBoth indicates traversing both directions.
	GraphDirectionBoth
)

// String returns a string representation of the GraphDirection.
func (d GraphDirection) String() string {
	switch d {
	case GraphDirectionNone:
		return "none"
	case GraphDirectionDependencies:
		return "dependencies"
	case GraphDirectionDependents:
		return "dependents"
	case GraphDirectionBoth:
		return "both"
	default:
		return "unknown"
	}
}

// IsNegated returns true if the expression starts with a negation operator.
func IsNegated(expr Expression) bool {
	switch node := expr.(type) {
	case *PrefixExpression:
		return node.Operator == "!"
	case *InfixExpression:
		return IsNegated(node.Left)
	default:
		return false
	}
}
