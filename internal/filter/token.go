package filter

// TokenType represents the type of a token.
type TokenType int

const (
	// ILLEGAL represents an unknown token
	ILLEGAL TokenType = iota

	// EOF represents the end of the input
	EOF

	// IDENT represents an identifier (e.g., "foo", "name")
	IDENT

	// PATH represents a path (starts with ./ or /)
	PATH

	// Operators
	BANG  // negation operator (!)
	PIPE  // intersection operator (|)
	EQUAL // attribute assignment (=)

	// Delimiters
	LBRACE   // left brace ({)
	RBRACE   // right brace (})
	LBRACKET // left bracket ([)
	RBRACKET // right bracket (])

	// Graph operators
	ELLIPSIS // ellipsis operator (...)
	CARET    // caret operator (^)
)

// String returns a string representation of the token type for debugging.
func (t TokenType) String() string {
	switch t {
	case ILLEGAL:
		return "ILLEGAL"
	case EOF:
		return "EOF"
	case IDENT:
		return "IDENT"
	case PATH:
		return "PATH"
	case BANG:
		return "!"
	case PIPE:
		return "|"
	case EQUAL:
		return "="
	case LBRACE:
		return "{"
	case RBRACE:
		return "}"
	case LBRACKET:
		return "["
	case RBRACKET:
		return "]"
	case ELLIPSIS:
		return "..."
	case CARET:
		return "^"
	default:
		return "UNKNOWN"
	}
}

// Token represents a lexical token with its type, literal value, and position.
type Token struct {
	Literal  string
	Type     TokenType
	Position int
}

// NewToken creates a new token with the given type, literal, and position.
func NewToken(tokenType TokenType, literal string, position int) Token {
	return Token{
		Type:     tokenType,
		Literal:  literal,
		Position: position,
	}
}
