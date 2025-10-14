package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLexer_SingleTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []Token
	}{
		{
			name:  "bang operator",
			input: "!",
			expected: []Token{
				{Type: BANG, Literal: "!", Position: 0},
				{Type: EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "pipe operator",
			input: "|",
			expected: []Token{
				{Type: PIPE, Literal: "|", Position: 0},
				{Type: EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "equal operator",
			input: "=",
			expected: []Token{
				{Type: EQUAL, Literal: "=", Position: 0},
				{Type: EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "simple identifier",
			input: "foo",
			expected: []Token{
				{Type: IDENT, Literal: "foo", Position: 0},
				{Type: EOF, Literal: "", Position: 3},
			},
		},
		{
			name:  "identifier with underscore",
			input: "foo_bar",
			expected: []Token{
				{Type: IDENT, Literal: "foo_bar", Position: 0},
				{Type: EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "identifier with hyphen",
			input: "foo-bar",
			expected: []Token{
				{Type: IDENT, Literal: "foo-bar", Position: 0},
				{Type: EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "hidden file",
			input: ".gitignore",
			expected: []Token{
				{Type: IDENT, Literal: ".gitignore", Position: 0},
				{Type: EOF, Literal: "", Position: 10},
			},
		},
		{
			name:  "hidden file with underscore",
			input: ".terragrunt-cache",
			expected: []Token{
				{Type: IDENT, Literal: ".terragrunt-cache", Position: 0},
				{Type: EOF, Literal: "", Position: 17},
			},
		},
		{
			name:  "relative path",
			input: "./apps",
			expected: []Token{
				{Type: PATH, Literal: "./apps", Position: 0},
				{Type: EOF, Literal: "", Position: 6},
			},
		},
		{
			name:  "absolute path",
			input: "/absolute/path",
			expected: []Token{
				{Type: PATH, Literal: "/absolute/path", Position: 0},
				{Type: EOF, Literal: "", Position: 14},
			},
		},
		{
			name:  "glob path with single wildcard",
			input: "./apps/*",
			expected: []Token{
				{Type: PATH, Literal: "./apps/*", Position: 0},
				{Type: EOF, Literal: "", Position: 8},
			},
		},
		{
			name:  "glob path with recursive wildcard",
			input: "./apps/**/foo",
			expected: []Token{
				{Type: PATH, Literal: "./apps/**/foo", Position: 0},
				{Type: EOF, Literal: "", Position: 13},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			for i, expected := range tt.expected {
				tok := lexer.NextToken()
				assert.Equal(t, expected.Type, tok.Type, "token %d type mismatch", i)
				assert.Equal(t, expected.Literal, tok.Literal, "token %d literal mismatch", i)
				assert.Equal(t, expected.Position, tok.Position, "token %d position mismatch", i)
			}
		})
	}
}

func TestLexer_ComplexQueries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []Token
	}{
		{
			name:  "attribute filter",
			input: "name=foo",
			expected: []Token{
				{Type: IDENT, Literal: "name", Position: 0},
				{Type: EQUAL, Literal: "=", Position: 4},
				{Type: IDENT, Literal: "foo", Position: 5},
				{Type: EOF, Literal: "", Position: 8},
			},
		},
		{
			name:  "negated attribute filter",
			input: "!name=bar",
			expected: []Token{
				{Type: BANG, Literal: "!", Position: 0},
				{Type: IDENT, Literal: "name", Position: 1},
				{Type: EQUAL, Literal: "=", Position: 5},
				{Type: IDENT, Literal: "bar", Position: 6},
				{Type: EOF, Literal: "", Position: 9},
			},
		},
		{
			name:  "negated path filter",
			input: "!./apps/legacy",
			expected: []Token{
				{Type: BANG, Literal: "!", Position: 0},
				{Type: PATH, Literal: "./apps/legacy", Position: 1},
				{Type: EOF, Literal: "", Position: 14},
			},
		},
		{
			name:  "union of two filters",
			input: "./apps/* | name=bar",
			expected: []Token{
				{Type: PATH, Literal: "./apps/*", Position: 0},
				{Type: PIPE, Literal: "|", Position: 9},
				{Type: IDENT, Literal: "name", Position: 11},
				{Type: EQUAL, Literal: "=", Position: 15},
				{Type: IDENT, Literal: "bar", Position: 16},
				{Type: EOF, Literal: "", Position: 19},
			},
		},
		{
			name:  "complex query with whitespace",
			input: "name=foo | !./legacy | ./apps/**",
			expected: []Token{
				{Type: IDENT, Literal: "name", Position: 0},
				{Type: EQUAL, Literal: "=", Position: 4},
				{Type: IDENT, Literal: "foo", Position: 5},
				{Type: PIPE, Literal: "|", Position: 9},
				{Type: BANG, Literal: "!", Position: 11},
				{Type: PATH, Literal: "./legacy", Position: 12},
				{Type: PIPE, Literal: "|", Position: 21},
				{Type: PATH, Literal: "./apps/**", Position: 23},
				{Type: EOF, Literal: "", Position: 32},
			},
		},
		{
			name:  "hidden file with operator",
			input: ".env | .gitignore",
			expected: []Token{
				{Type: IDENT, Literal: ".env", Position: 0},
				{Type: PIPE, Literal: "|", Position: 5},
				{Type: IDENT, Literal: ".gitignore", Position: 7},
				{Type: EOF, Literal: "", Position: 17},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			for i, expected := range tt.expected {
				tok := lexer.NextToken()
				assert.Equal(t, expected.Type, tok.Type, "token %d type mismatch", i)
				assert.Equal(t, expected.Literal, tok.Literal, "token %d literal mismatch", i)
				assert.Equal(t, expected.Position, tok.Position, "token %d position mismatch", i)
			}
		})
	}
}

func TestLexer_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []Token
	}{
		{
			name:  "empty input",
			input: "",
			expected: []Token{
				{Type: EOF, Literal: "", Position: 0},
			},
		},
		{
			name:  "only whitespace",
			input: "   \t\n  ",
			expected: []Token{
				{Type: EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "single dot (invalid)",
			input: ".",
			expected: []Token{
				{Type: ILLEGAL, Literal: ".", Position: 0},
				{Type: EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "special character now allowed",
			input: "@username",
			expected: []Token{
				{Type: IDENT, Literal: "@username", Position: 0},
				{Type: EOF, Literal: "", Position: 9},
			},
		},
		{
			name:  "path with dashes and underscores",
			input: "./my-app_v2/foo-bar",
			expected: []Token{
				{Type: PATH, Literal: "./my-app_v2/foo-bar", Position: 0},
				{Type: EOF, Literal: "", Position: 19},
			},
		},
		{
			name:  "tab in identifier",
			input: "foo\tbar",
			expected: []Token{
				{Type: IDENT, Literal: "foo\tbar", Position: 0},
				{Type: EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "special characters in path",
			input: "./app+test",
			expected: []Token{
				{Type: PATH, Literal: "./app+test", Position: 0},
				{Type: EOF, Literal: "", Position: 10},
			},
		},
		{
			name:  "spaces in identifier",
			input: "foo bar",
			expected: []Token{
				{Type: IDENT, Literal: "foo bar", Position: 0},
				{Type: EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "spaces in path",
			input: "./my path/to file",
			expected: []Token{
				{Type: PATH, Literal: "./my path/to file", Position: 0},
				{Type: EOF, Literal: "", Position: 17},
			},
		},
		{
			name:  "spaces with pipe separator",
			input: "foo bar | baz qux",
			expected: []Token{
				{Type: IDENT, Literal: "foo bar", Position: 0},
				{Type: PIPE, Literal: "|", Position: 8},
				{Type: IDENT, Literal: "baz qux", Position: 10},
				{Type: EOF, Literal: "", Position: 17},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := NewLexer(tt.input)
			for i, expected := range tt.expected {
				tok := lexer.NextToken()
				assert.Equal(t, expected.Type, tok.Type, "token %d type mismatch", i)
				assert.Equal(t, expected.Literal, tok.Literal, "token %d literal mismatch", i)
				assert.Equal(t, expected.Position, tok.Position, "token %d position mismatch", i)
			}
		})
	}
}

func TestTokenType_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tokenType TokenType
		expected  string
	}{
		{ILLEGAL, "ILLEGAL"},
		{EOF, "EOF"},
		{IDENT, "IDENT"},
		{PATH, "PATH"},
		{BANG, "!"},
		{PIPE, "|"},
		{EQUAL, "="},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.tokenType.String())
		})
	}
}
