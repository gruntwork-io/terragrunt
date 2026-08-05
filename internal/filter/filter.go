package filter

import (
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Filter represents a parsed filter query that can be evaluated against discovered configs.
type Filter struct {
	expr          Expression
	originalQuery string
}

// Parse parses a filter query string and returns a Filter object.
// Returns an error if the query cannot be parsed.
func Parse(filterString string) (*Filter, error) {
	lexer := NewLexer(filterString)
	parser := NewParser(lexer)

	expr, err := parser.ParseExpression()
	if err != nil {
		return nil, err
	}

	return &Filter{
		expr:          expr,
		originalQuery: filterString,
	}, nil
}

// NewFilter creates a new Filter object.
func NewFilter(expr Expression, originalQuery string) *Filter {
	return &Filter{expr: expr, originalQuery: originalQuery}
}

// String returns a string representation of the filter.
func (f *Filter) String() string {
	return f.originalQuery
}

// Evaluate applies the filter to a list of components and returns the filtered result.
// If logger is provided, it will be used for logging warnings during evaluation.
func (f *Filter) Evaluate(
	l log.Logger,
	evalCtx EvaluationContext,
	components component.Components,
) (component.Components, error) {
	return Evaluate(l, evalCtx, f.expr, components)
}

// Expression returns the parsed AST expression.
// This is useful for debugging or advanced use cases.
func (f *Filter) Expression() Expression {
	return f.expr
}

// RequiresParse returns true if the filter requires parsing of Terragrunt HCL configurations.
func (f *Filter) RequiresParse() (Expression, bool) {
	return f.expr.RequiresParse()
}

// HasGraphBoundary reports whether any graph expression in the filter carries
// an inline "(dir)" boundary operand.
func (f *Filter) HasGraphBoundary() bool {
	found := false

	WalkExpressions(f.expr, func(e Expression) bool {
		if g, ok := e.(*GraphExpression); ok && (g.Dependents.Boundary != "" || g.Dependencies.Boundary != "") {
			found = true

			return false
		}

		return true
	})

	return found
}

// GraphBoundaries returns every inline "(dir)" boundary operand in the filter,
// unresolved and in no particular order.
func (f *Filter) GraphBoundaries() []string {
	var boundaries []string

	WalkExpressions(f.expr, func(e Expression) bool {
		g, ok := e.(*GraphExpression)
		if !ok {
			return true
		}

		if g.Dependents.Boundary != "" {
			boundaries = append(boundaries, g.Dependents.Boundary)
		}

		if g.Dependencies.Boundary != "" {
			boundaries = append(boundaries, g.Dependencies.Boundary)
		}

		return true
	})

	return boundaries
}

// HasDependents reports whether any graph expression in the filter traverses
// the dependent direction.
func (f *Filter) HasDependents() bool {
	found := false

	WalkExpressions(f.expr, func(e Expression) bool {
		if g, ok := e.(*GraphExpression); ok && g.Dependents.Include {
			found = true

			return false
		}

		return true
	})

	return found
}

// Apply is a convenience function that parses and evaluates a filter in one step.
// It's equivalent to calling Parse followed by Evaluate.
func Apply(
	l log.Logger,
	evalCtx EvaluationContext,
	filterString string,
	components component.Components,
) (component.Components, error) {
	filter, err := Parse(filterString)
	if err != nil {
		return nil, err
	}

	return filter.Evaluate(l, evalCtx, components)
}
