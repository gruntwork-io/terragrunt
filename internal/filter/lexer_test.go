package filter_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
)

func TestLexer_SingleTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected []filter.Token
	}{
		{
			name:  "bang operator",
			input: "!",
			expected: []filter.Token{
				{Type: filter.BANG, Literal: "!", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "pipe operator",
			input: "|",
			expected: []filter.Token{
				{Type: filter.PIPE, Literal: "|", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "left brace",
			input: "{",
			expected: []filter.Token{
				{Type: filter.LBRACE, Literal: "{", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "right brace",
			input: "}",
			expected: []filter.Token{
				{Type: filter.RBRACE, Literal: "}", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "equal operator",
			input: "=",
			expected: []filter.Token{
				{Type: filter.EQUAL, Literal: "=", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "simple identifier",
			input: "foo",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "foo", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 3},
			},
		},
		{
			name:  "identifier with underscore",
			input: "foo_bar",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "foo_bar", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "identifier with hyphen",
			input: "foo-bar",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "foo-bar", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "hidden file",
			input: ".gitignore",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: ".gitignore", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 10},
			},
		},
		{
			name:  "hidden file with underscore",
			input: ".terragrunt-cache",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: ".terragrunt-cache", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 17},
			},
		},
		{
			name:  "relative path",
			input: "./apps",
			expected: []filter.Token{
				{Type: filter.PATH, Literal: "./apps", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 6},
			},
		},
		{
			name:  "absolute path",
			input: "/absolute/path",
			expected: []filter.Token{
				{Type: filter.PATH, Literal: "/absolute/path", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 14},
			},
		},
		{
			name:  "glob path with single wildcard",
			input: "./apps/*",
			expected: []filter.Token{
				{Type: filter.PATH, Literal: "./apps/*", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 8},
			},
		},
		{
			name:  "glob path with recursive wildcard",
			input: "./apps/**/foo",
			expected: []filter.Token{
				{Type: filter.PATH, Literal: "./apps/**/foo", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 13},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
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
		expected []filter.Token
	}{
		{
			name:  "attribute filter",
			input: "name=foo",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "name", Position: 0},
				{Type: filter.EQUAL, Literal: "=", Position: 4},
				{Type: filter.IDENT, Literal: "foo", Position: 5},
				{Type: filter.EOF, Literal: "", Position: 8},
			},
		},
		{
			name:  "negated attribute filter",
			input: "!name=bar",
			expected: []filter.Token{
				{Type: filter.BANG, Literal: "!", Position: 0},
				{Type: filter.IDENT, Literal: "name", Position: 1},
				{Type: filter.EQUAL, Literal: "=", Position: 5},
				{Type: filter.IDENT, Literal: "bar", Position: 6},
				{Type: filter.EOF, Literal: "", Position: 9},
			},
		},
		{
			name:  "negated path filter",
			input: "!./apps/legacy",
			expected: []filter.Token{
				{Type: filter.BANG, Literal: "!", Position: 0},
				{Type: filter.PATH, Literal: "./apps/legacy", Position: 1},
				{Type: filter.EOF, Literal: "", Position: 14},
			},
		},
		{
			name:  "union of two filters",
			input: "./apps/* | name=bar",
			expected: []filter.Token{
				{Type: filter.PATH, Literal: "./apps/*", Position: 0},
				{Type: filter.PIPE, Literal: "|", Position: 9},
				{Type: filter.IDENT, Literal: "name", Position: 11},
				{Type: filter.EQUAL, Literal: "=", Position: 15},
				{Type: filter.IDENT, Literal: "bar", Position: 16},
				{Type: filter.EOF, Literal: "", Position: 19},
			},
		},
		{
			name:  "complex query with whitespace",
			input: "name=foo | !./legacy | ./apps/**",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "name", Position: 0},
				{Type: filter.EQUAL, Literal: "=", Position: 4},
				{Type: filter.IDENT, Literal: "foo", Position: 5},
				{Type: filter.PIPE, Literal: "|", Position: 9},
				{Type: filter.BANG, Literal: "!", Position: 11},
				{Type: filter.PATH, Literal: "./legacy", Position: 12},
				{Type: filter.PIPE, Literal: "|", Position: 21},
				{Type: filter.PATH, Literal: "./apps/**", Position: 23},
				{Type: filter.EOF, Literal: "", Position: 32},
			},
		},
		{
			name:  "hidden file with operator",
			input: ".env | .gitignore",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: ".env", Position: 0},
				{Type: filter.PIPE, Literal: "|", Position: 5},
				{Type: filter.IDENT, Literal: ".gitignore", Position: 7},
				{Type: filter.EOF, Literal: "", Position: 17},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
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
		expected []filter.Token
	}{
		{
			name:  "empty input",
			input: "",
			expected: []filter.Token{
				{Type: filter.EOF, Literal: "", Position: 0},
			},
		},
		{
			name:  "only whitespace",
			input: "   \t\n  ",
			expected: []filter.Token{
				{Type: filter.EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "single dot (invalid)",
			input: ".",
			expected: []filter.Token{
				{Type: filter.ILLEGAL, Literal: ".", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 1},
			},
		},
		{
			name:  "special character now allowed",
			input: "@username",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "@username", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 9},
			},
		},
		{
			name:  "path with dashes and underscores",
			input: "./my-app_v2/foo-bar",
			expected: []filter.Token{
				{Type: filter.PATH, Literal: "./my-app_v2/foo-bar", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 19},
			},
		},
		{
			name:  "tab in identifier",
			input: "foo\tbar",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "foo\tbar", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "special characters in path",
			input: "./app+test",
			expected: []filter.Token{
				{Type: filter.PATH, Literal: "./app+test", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 10},
			},
		},
		{
			name:  "spaces in identifier",
			input: "foo bar",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "foo bar", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 7},
			},
		},
		{
			name:  "spaces in path",
			input: "./my path/to file",
			expected: []filter.Token{
				{Type: filter.PATH, Literal: "./my path/to file", Position: 0},
				{Type: filter.EOF, Literal: "", Position: 17},
			},
		},
		{
			name:  "spaces with pipe separator",
			input: "foo bar | baz qux",
			expected: []filter.Token{
				{Type: filter.IDENT, Literal: "foo bar", Position: 0},
				{Type: filter.PIPE, Literal: "|", Position: 8},
				{Type: filter.IDENT, Literal: "baz qux", Position: 10},
				{Type: filter.EOF, Literal: "", Position: 17},
			},
		},
		{
			name:  "braced path",
			input: "{./apps/*}",
			expected: []filter.Token{
				{Type: filter.LBRACE, Literal: "{", Position: 0},
				{Type: filter.PATH, Literal: "./apps/*", Position: 1},
				{Type: filter.RBRACE, Literal: "}", Position: 9},
				{Type: filter.EOF, Literal: "", Position: 10},
			},
		},
		{
			name:  "braced path with spaces",
			input: "{my path/file}",
			expected: []filter.Token{
				{Type: filter.LBRACE, Literal: "{", Position: 0},
				{Type: filter.IDENT, Literal: "my path", Position: 1},
				{Type: filter.PATH, Literal: "/file", Position: 8},
				{Type: filter.RBRACE, Literal: "}", Position: 13},
				{Type: filter.EOF, Literal: "", Position: 14},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lexer := filter.NewLexer(tt.input)
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
		expected  string
		tokenType filter.TokenType
	}{
		{"ILLEGAL", filter.ILLEGAL},
		{"EOF", filter.EOF},
		{"IDENT", filter.IDENT},
		{"PATH", filter.PATH},
		{"!", filter.BANG},
		{"|", filter.PIPE},
		{"=", filter.EQUAL},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.expected, tt.tokenType.String())
		})
	}
}
