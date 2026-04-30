package filter

// WalkExpressions traverses the expression tree depth-first, calling fn for each node.
// The traversal continues to child nodes only if fn returns true.
// For GraphExpression nodes, traversal continues into the Target expression.
// For PrefixExpression nodes, traversal continues into the Right expression.
// For InfixExpression nodes, traversal continues into both Left and Right expressions.
func WalkExpressions(expr Expression, fn func(Expression) bool) {
	if expr == nil {
		return
	}

	if !fn(expr) {
		return
	}

	switch node := expr.(type) {
	case *GraphExpression:
		WalkExpressions(node.Target, fn)
	case *PrefixExpression:
		WalkExpressions(node.Right, fn)
	case *InfixExpression:
		WalkExpressions(node.Left, fn)
		WalkExpressions(node.Right, fn)
	}
}
