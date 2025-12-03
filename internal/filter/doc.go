// Package filter provides a parser and evaluator for filter query strings used to select Terragrunt components.
//
// # Overview
//
// The filter package implements a three-stage compiler architecture:
//  1. Lexer: Tokenizes the input filter query string
//  2. Parser: Builds an Abstract Syntax Tree (AST) from tokens
//  3. Evaluator: Applies the filter logic to discovered Terragrunt components
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
// Path filters match components by their file system path. They support glob patterns:
//
//	./apps/frontend         # Exact path match
//	./apps/*                # Single-level wildcard
//	./apps/**/api           # Recursive wildcard
//	/absolute/path          # Absolute path
//
// ## Attribute Filters
//
// Attribute filters match components by their attributes:
//
//	name=my-app             # Match by config name (directory basename)
//	type=unit               # Match components of type "unit"
//	type=stack              # Match components of type "stack"
//	external=true           # Match external dependencies
//	external=false          # Match internal dependencies (not external)
//	foo                     # Shorthand for name=foo
//
// ## Negation Operator (!)
//
// The negation operator excludes matching components:
//
//	!name=legacy            # Exclude components named "legacy"
//	!./apps/old             # Exclude components at path ./apps/old
//	!foo                    # Exclude components named "foo"
//	!external=true          # Exclude external dependencies
//
// ## Intersection Operator (|)
//
// The intersection operator refines/narrows results by applying filters from left to right.
// Each filter in the chain further restricts the results from the previous filter.
// The pipe character (|) is the only delimiter between filter expressions.
// Whitespace is optional around operators but is NOT a delimiter itself.
//
//	./apps/* | name=web         # Components in ./apps/* AND named "web"
//	./apps/*|name=web           # Same as above (spaces optional)
//	./foo* | !./foobar*         # Components in ./foo* AND NOT in ./foobar*
//	type=unit | !external=true  # Internal components only
//
// Spaces within component names and paths are preserved:
//
//	my app                  # Component named "my app" (with space)
//	./my path/file          # Path with spaces
//
// ## Braced Path Syntax ({})
//
// Use braces to explicitly mark a path expression. This is useful when:
// - The path doesn't start with ./ or /
// - You want to be explicit that something is a path, not an identifier
//
//	{./apps/*}              # Explicitly a path
//	{my path/file}          # Path without ./ prefix
//	{apps}                  # Treat "apps" as a path, not a name filter
//
// # Operator Precedence
//
// Operators are evaluated with the following precedence (highest to lowest):
//  1. Prefix operators (!)
//  2. Infix operators (| - intersection/refinement, left-to-right)
//
// This means !foo | bar is evaluated as (!foo) | bar, not !(foo | bar).
// The intersection operator applies filters left-to-right, each filter
// refining/narrowing the results from the previous filter.
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
//	// Apply the filter to discovered components
//	// (typically obtained from discovery.Discover())
//	components := []*component.Component{
//	    {Path: "./apps/app1", Kind: component.Unit},
//	    {Path: "./apps/legacy", Kind: component.Unit},
//	    {Path: "./libs/db", Kind: component.Unit},
//	}
//	result, err := filter.Evaluate(components)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// ## Multiple Filters (Union)
//
// Multiple filter queries can be combined using the Filters type, which applies
// union (OR) semantics. This is different from using | within a single filter,
// which applies intersection (AND) semantics.
//
//	// Parse multiple filter queries
//	filters, err := filter.ParseFilterQueries([]string{
//	    "./apps/*",      // Select all apps
//	    "name=db",       // OR select db
//	}, ".")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	result, err := filters.Evaluate(components)
//	// Returns: all components in ./apps/* OR components named "db"
//
// Multiple filters are evaluated in two phases:
//  1. Positive filters (non-negated) are evaluated and their results are unioned
//  2. Negative filters (starting with !) are applied to remove matching components
//
// The ExcludeByDefault() method signals whether filters operate in exclude-by-default
// mode. This is true if ANY filter doesn't start with a negation expression:
//
//	filters.ExcludeByDefault() // true if any filter is positive
//
// When true, discovery should start with an empty set and add matches.
// When false (all filters are negated), discovery should start with all components
// and remove matches.
//
// ## One-Shot Usage
//
//	// Parse and evaluate in one step
//	result, err := filter.Apply("./apps/* | name=web", ".", components)
//
// # Implementation Details
//
// ## Lexer
//
// The lexer (lexer.go) scans the input string and produces tokens:
//   - IDENT: Identifiers (foo, name, etc.)
//   - PATH: Paths (./apps/*, /absolute, etc.)
//   - BANG: Negation operator (!)
//   - PIPE: Intersection operator (|)
//   - EQUAL: Assignment operator (=)
//   - LBRACE: Left brace ({)
//   - RBRACE: Right brace (})
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
//     and caching via sync.Once for performance
//   - AttributeFilter: Matches attributes by key-value pairs:
//   - name: Matches filepath.Base(component.Path)
//   - type: Matches component.Kind (unit or stack)
//   - external: Matches component.External (true or false)
//   - PrefixExpression: Returns the complement of the right side
//   - InfixExpression: Returns the intersection by applying right filter to left results
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
// Future versions will support:
//   - Git-based filtering ([main...HEAD])
//   - Dependency traversal (name=foo...)
//   - Dependents traversal (...name=foo)
//   - Read-based filtering (reads=path/to/file)
package filter
