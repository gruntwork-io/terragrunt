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

// IsPureNegation returns true if every operand of the expression is negated, meaning the
// filter can only subtract components and never selects any of its own.
//
// A compound expression such as "!foo | bar" is not a pure negation. The "|" operator
// intersects left to right, so the expression still selects the components matching "bar".
func IsPureNegation(expr Expression) bool {
	switch node := expr.(type) {
	case *PrefixExpression:
		return node.Operator == "!"
	case *InfixExpression:
		return IsPureNegation(node.Left) && IsPureNegation(node.Right)
	default:
		return false
	}
}
