package filter

// Filter represents a parsed filter query that can be evaluated against units.
type Filter struct {
	expr          Expression
	originalQuery string
	workingDir    string
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

// Evaluate applies the filter to a list of units and returns the filtered result.
func (f *Filter) Evaluate(allUnits []Unit) ([]Unit, error) {
	return Evaluate(f.expr, allUnits)
}

// String returns the original filter query string.
func (f *Filter) String() string {
	return f.originalQuery
}

// Expression returns the parsed AST expression.
// This is useful for debugging or advanced use cases.
func (f *Filter) Expression() Expression {
	return f.expr
}

// Apply is a convenience function that parses and evaluates a filter in one step.
// It's equivalent to calling Parse followed by Evaluate.
func Apply(filterString string, units []Unit) ([]Unit, error) {
	filter, err := Parse(filterString)
	if err != nil {
		return nil, err
	}

	return filter.Evaluate(units)
}
