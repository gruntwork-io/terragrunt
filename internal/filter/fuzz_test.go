package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
)

// FuzzParse tests the main Parse() function with arbitrary input.
// It verifies that Parse never panics regardless of input.
func FuzzParse(f *testing.F) {
	seeds := []string{
		// Simple paths
		"foo",
		"foo/bar",
		"**/*.hcl",
		"./apps",
		"./apps/*",
		"./apps/**/foo",
		"/absolute/path",
		"../foo",
		"./my-app_v2/foo-bar",

		// Attributes
		"name=foo",
		"name=bar",
		"type=unit",
		"key=value",
		"source=github.com/acme/foo/bar",

		// Operators
		"!foo",
		"foo | bar",
		"!name=bar",
		"!./apps/legacy",
		"./apps/* | name=bar",
		"name=foo | !./legacy | ./apps/**",

		// Graph expressions
		"...foo",
		"foo...",
		"...foo...",
		"^foo",
		"...^foo",
		"^foo...",
		"...^foo...",
		"1...foo",
		"foo...1",
		"2...foo...3",
		"1...^foo...2",

		// Braced paths
		"{./apps/*}",
		"{my path/file}",
		"!{./apps/legacy}",
		"{1}",

		// Git refs
		"[HEAD]",
		"[main]",
		"[main...HEAD]",
		"[main...feature]",
		"[abc123...def456]",
		"[v1.0.0...v2.0.0]",
		"[HEAD~1...HEAD]",
		"[feature/name]",

		// Git + graph combinations
		"[main...HEAD]...",
		"...[main...HEAD]",
		"...[main...HEAD]...",
		"...^[main...HEAD]...",

		// Edge cases
		"",
		".",
		"...",
		"{",
		"[",
		"!",
		"|",
		"=",
		"^",
		"}",
		"]",
		"[]",
		"{}",
		".gitignore",
		".terragrunt-cache",
		"@username",
		"foo bar",
		"   \t\n  ",
		"..1",
		"..25",
		"foo..bar",
		"foo..1",
		"..2foo",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		_, _ = filter.Parse(input)
	})
}

// FuzzLexer tests the lexer by tokenizing arbitrary input.
// It verifies that the lexer never panics and always terminates.
func FuzzLexer(f *testing.F) {
	seeds := []string{
		// Single operators
		"!",
		"|",
		"=",
		"{",
		"}",
		"[",
		"]",
		"^",
		"...",

		// Identifiers
		"foo",
		"foo_bar",
		"foo-bar",
		".gitignore",
		".terragrunt-cache",
		"@username",

		// Paths
		"./apps",
		"/absolute/path",
		"./apps/*",
		"./apps/**/foo",
		"../foo",
		"foo/bar",

		// Complex sequences
		"name=foo",
		"!name=bar",
		"./apps/* | name=bar",
		"name=foo | !./legacy | ./apps/**",
		"...foo",
		"foo...",
		"1...foo",
		"foo...1",
		"{./apps/*}",
		"[main...HEAD]",

		// Edge cases
		"",
		"   \t\n  ",
		".",
		"..1",
		"..25",
		"foo..bar",
		"1...1",
		"99999999999999999999999...foo",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		lexer := filter.NewLexer(input)

		// Tokenize until EOF - should never hang or panic
		// Set a reasonable limit to prevent infinite loops in case of bugs
		const maxTokens = 10000
		for range maxTokens {
			tok := lexer.NextToken()
			if tok.Type == filter.EOF {
				break
			}
		}
	})
}

// FuzzParser tests the parser by parsing arbitrary input.
// It verifies that the parser never panics during AST construction.
func FuzzParser(f *testing.F) {
	seeds := []string{
		// Simple expressions
		"foo",
		"name=bar",
		"./apps/*",
		"{./apps/*}",
		"[main]",

		// Prefix expressions
		"!foo",
		"!name=bar",
		"!./apps/legacy",
		"!{./apps/legacy}",
		"![main...HEAD]",

		// Infix expressions
		"foo | bar",
		"foo | bar | baz",
		"./apps/* | name=bar",
		"name=foo | !./legacy | ./apps/**",
		"!foo | bar",
		"foo | !bar",

		// Graph expressions
		"...foo",
		"foo...",
		"...foo...",
		"^foo",
		"...^foo",
		"^foo...",
		"...^foo...",
		"1...foo",
		"foo...1",
		"2...foo...3",
		"1...^foo...2",
		"10...foo...25",
		"999999999...foo",

		// Git expressions
		"[main]",
		"[main...HEAD]",
		"[abc123...def456]",
		"[v1.0.0...v2.0.0]",
		"[HEAD~1...HEAD]",
		"[feature/name]",

		// Combined expressions
		"[main...HEAD]...",
		"...[main...HEAD]",
		"...[main...HEAD]...",
		"[main...HEAD] | ./apps/*",
		"!...foo",
		"...!foo",
		"...foo | bar",
		"foo | bar...",

		// Error cases (parser should handle gracefully)
		"",
		"!",
		"name=",
		"foo |",
		"foo | bar |",
		"|",
		"| foo",
		"[]",
		"[main",
		"[...]",
		"[main...]",
		"]",
		"{}",
		"{",
		"...",
		"^",
		"... |",
		"^ |",
		"1...",
		"1... ",
		"1......2",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		lexer := filter.NewLexer(input)
		parser := filter.NewParser(lexer)

		// ParseExpression should not panic on any input
		// It may return an error for invalid input, which is expected
		_, _ = parser.ParseExpression()
	})
}
