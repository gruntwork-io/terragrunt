package filter

import (
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Filter represents a parsed filter query that can be evaluated against discovered configs.
type Filter struct {
	expr          Expression
	originalQuery string
	workingDir    string
}

// String returns a string representation of the filter.
func (f *Filter) String() string {
	return f.originalQuery
}

// Parse parses a filter query string and returns a Filter object.
// Returns an error if the query cannot be parsed.
func Parse(filterString, workingDir string) (*Filter, error) {
	lexer := NewLexer(filterString)
	parser := NewParser(lexer, workingDir)

	expr, err := parser.ParseExpression()
	if err != nil {
		return nil, err
	}

	return &Filter{
		expr:          expr,
		originalQuery: filterString,
		workingDir:    workingDir,
	}, nil
}

// Evaluate applies the filter to a list of components and returns the filtered result.
// If logger is provided, it will be used for logging warnings during evaluation.
func (f *Filter) Evaluate(l log.Logger, components component.Components) (component.Components, error) {
	return Evaluate(l, f.expr, components)
}

// Expression returns the parsed AST expression.
// This is useful for debugging or advanced use cases.
func (f *Filter) Expression() Expression {
	return f.expr
}

// Apply is a convenience function that parses and evaluates a filter in one step.
// It's equivalent to calling Parse followed by Evaluate.
func Apply(l log.Logger, filterString, workingDir string, components component.Components) (component.Components, error) {
	filter, err := Parse(filterString, workingDir)
	if err != nil {
		return nil, err
	}

	return filter.Evaluate(l, components)
}
