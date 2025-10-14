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
	PIPE  // union operator (|)
	EQUAL // attribute assignment (=)
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
	default:
		return "UNKNOWN"
	}
}

// Token represents a lexical token with its type, literal value, and position.
type Token struct {
	Type     TokenType // The type of the token
	Literal  string    // The literal value of the token
	Position int       // The position in the input string where the token starts
}

// NewToken creates a new token with the given type, literal, and position.
func NewToken(tokenType TokenType, literal string, position int) Token {
	return Token{
		Type:     tokenType,
		Literal:  literal,
		Position: position,
	}
}
