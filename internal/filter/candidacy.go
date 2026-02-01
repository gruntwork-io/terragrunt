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

// CandidacyInfo contains information about how an expression should be evaluated
// during the phased discovery process.
type CandidacyInfo struct {
	GraphDirection         GraphDirection
	DependencyDepth        int
	DependentDepth         int
	RequiresFilesystemOnly bool
	RequiresParsing        bool
	RequiresGraphDiscovery bool
	IsNegated              bool
	ExcludeTarget          bool
}

// AnalyzeCandidacy analyzes an expression and returns information about how it
// should be evaluated during the phased discovery process.
func AnalyzeCandidacy(expr Expression) CandidacyInfo {
	info := CandidacyInfo{
		RequiresFilesystemOnly: true,
	}

	analyzeExpressionCandidacy(expr, &info)

	return info
}

// analyzeExpressionCandidacy recursively analyzes an expression.
func analyzeExpressionCandidacy(expr Expression, info *CandidacyInfo) {
	switch node := expr.(type) {
	case *PathExpression:
		// Path expressions only require filesystem info
		info.RequiresFilesystemOnly = true

	case *AttributeExpression:
		switch node.Key {
		case AttributeName, AttributeType, AttributeExternal:
			// These can be evaluated with filesystem info
			info.RequiresFilesystemOnly = true
		case AttributeReading, AttributeSource:
			// These require parsing
			info.RequiresParsing = true
			info.RequiresFilesystemOnly = false
		default:
			// Unknown attributes conservatively require parsing
			info.RequiresParsing = true
			info.RequiresFilesystemOnly = false
		}

	case *GraphExpression:
		// Analyze target expression first (for parsing requirements, etc.)
		analyzeExpressionCandidacy(node.Target, info)

		// Graph expressions always require graph discovery and are not filesystem-only
		info.RequiresGraphDiscovery = true
		info.RequiresFilesystemOnly = false
		info.ExcludeTarget = node.ExcludeTarget
		info.DependencyDepth = node.DependencyDepth
		info.DependentDepth = node.DependentDepth

		// Determine graph direction
		switch {
		case node.IncludeDependencies && node.IncludeDependents:
			info.GraphDirection = GraphDirectionBoth
		case node.IncludeDependencies:
			info.GraphDirection = GraphDirectionDependencies
		case node.IncludeDependents:
			info.GraphDirection = GraphDirectionDependents
		}

	case *GitExpression:
		// Git expressions are evaluated during worktree phase
		// They don't require filesystem-only or parsing
		info.RequiresFilesystemOnly = true

	case *PrefixExpression:
		if node.Operator == "!" {
			info.IsNegated = true
		}

		analyzeExpressionCandidacy(node.Right, info)

	case *InfixExpression:
		leftInfo := CandidacyInfo{}
		rightInfo := CandidacyInfo{}

		analyzeExpressionCandidacy(node.Left, &leftInfo)
		analyzeExpressionCandidacy(node.Right, &rightInfo)

		// Merge: if either side requires more, the whole expression does
		info.RequiresFilesystemOnly = leftInfo.RequiresFilesystemOnly && rightInfo.RequiresFilesystemOnly
		info.RequiresParsing = leftInfo.RequiresParsing || rightInfo.RequiresParsing
		info.RequiresGraphDiscovery = leftInfo.RequiresGraphDiscovery || rightInfo.RequiresGraphDiscovery
	}
}

// GetGraphTargets extracts the target expressions from graph expressions.
// Returns nil if the expression contains no graph expressions.
func GetGraphTargets(expr Expression) []Expression {
	var targets []Expression

	WalkExpressions(expr, func(e Expression) bool {
		if graphExpr, ok := e.(*GraphExpression); ok {
			targets = append(targets, graphExpr.Target)
		}

		return true
	})

	return targets
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

// GetGraphExpressions returns all graph expressions within an expression tree.
func GetGraphExpressions(expr Expression) []*GraphExpression {
	var graphExprs []*GraphExpression

	WalkExpressions(expr, func(e Expression) bool {
		if graphExpr, ok := e.(*GraphExpression); ok {
			graphExprs = append(graphExprs, graphExpr)
		}

		return true
	})

	return graphExprs
}
