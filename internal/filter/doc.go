// Package filter provides a parser and evaluator for filter query strings used to select Terragrunt units.
//
// # Overview
//
// The filter package implements a three-stage compiler architecture:
//  1. Lexer: Tokenizes the input filter query string
//  2. Parser: Builds an Abstract Syntax Tree (AST) from tokens
//  3. Evaluator: Applies the filter logic to a list of units
//
// This design follows the classic compiler pattern and provides a clean separation of concerns
// between syntax analysis and semantic evaluation.
//
// # Filter Syntax
//
// The filter package supports the following syntax elements:
//
// ## Path Filters
//
// Path filters match units by their file system path. They support glob patterns:
//
//	./apps/frontend         # Exact path match
//	./apps/*                # Single-level wildcard
//	./apps/**/api           # Recursive wildcard
//	/absolute/path          # Absolute path
//
// ## Attribute Filters
//
// Attribute filters match units by their attributes:
//
//	name=my-app             # Match by unit name
//	type=unit               # Match by unit type
//	foo                     # Shorthand for name=foo
//
// ## Negation Operator (!)
//
// The negation operator excludes matching units:
//
//	!name=legacy            # Exclude units named "legacy"
//	!./apps/old             # Exclude units at path ./apps/old
//	!foo                    # Exclude units named "foo"
//
// ## Union Operator (|)
//
// The union operator combines multiple filters with OR semantics.
// The pipe character (|) is the only delimiter between filter expressions.
// Whitespace is optional around operators but is NOT a delimiter itself.
//
//	foo | bar               # Units named foo OR bar
//	foo|bar                 # Same as above (spaces optional)
//	./apps/* | ./libs/*     # Units in apps OR libs directory
//	name=a | name=b | name=c # Units named a OR b OR c
//
// Spaces within unit names and paths are preserved:
//
//	my app                  # Unit named "my app" (with space)
//	./my path/file          # Path with spaces
//
// # Operator Precedence
//
// Operators are evaluated with the following precedence (highest to lowest):
//  1. Prefix operators (!)
//  2. Infix operators (|)
//
// This means !foo | bar is evaluated as (!foo) | bar, not !(foo | bar).
//
// # Usage Examples
//
// ## Basic Usage
//
//	// Parse a filter query
//	filter, err := filter.Parse("./apps/* | !legacy", ".")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Apply the filter to units
//	units := []filter.Unit{
//	    {Name: "app1", Path: "./apps/app1"},
//	    {Name: "legacy", Path: "./apps/legacy"},
//	    {Name: "db", Path: "./libs/db"},
//	}
//	result, err := filter.Evaluate(units)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// ## One-Shot Usage
//
//	// Parse and evaluate in one step
//	result, err := filter.Apply("name=foo | name=bar", units)
//
// # Implementation Details
//
// ## Lexer
//
// The lexer (lexer.go) scans the input string and produces tokens:
//   - IDENT: Identifiers (foo, name, etc.)
//   - PATH: Paths (./apps/*, /absolute, etc.)
//   - BANG: Negation operator (!)
//   - PIPE: Union operator (|)
//   - EQUAL: Assignment operator (=)
//   - EOF: End of input
//
// ## Parser
//
// The parser (parser.go) uses recursive descent parsing with Pratt parsing for operators.
// It produces an AST with the following node types:
//   - PathFilter: Path/glob filter
//   - AttributeFilter: Key-value attribute filter
//   - PrefixExpression: Negation operator
//   - InfixExpression: Union operator
//
// ## Evaluator
//
// The evaluator (evaluator.go) walks the AST and applies the filter logic:
//   - PathFilter: Uses glob matching (github.com/gobwas/glob) with eager compilation
//     and caching via sync.OnceValue for performance
//   - AttributeFilter: Matches attributes by key-value pairs
//   - PrefixExpression: Returns the complement of the right side
//   - InfixExpression: Returns the union (deduplicated) of both sides
//
// Path filters compile their glob pattern once on first evaluation and cache
// the compiled result for reuse in subsequent evaluations, providing significant
// performance improvements when filters are evaluated multiple times.
//
// # Related
//
// This package implements the filter syntax described in RFC #4060:
// https://github.com/gruntwork-io/terragrunt/issues/4060
//
// The syntax is inspired by Turborepo's filter syntax:
// https://turbo.build/repo/docs/reference/run#--filter-string
//
// # Future Enhancements
//
// Future versions may support:
//   - Git-based filtering ([main...HEAD])
//   - Dependency traversal (...name=foo)
//   - Read-based filtering (reads:path/to/file)
//   - AND operator (&)
package filter
