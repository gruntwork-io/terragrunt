package filter

import (
	"strings"
	"unicode"
)

// Lexer tokenizes a filter query string.
type Lexer struct {
	input        string // The input string being tokenized
	position     int    // Current position in input (points to current char)
	readPosition int    // Current reading position in input (after current char)
	ch           byte   // Current char under examination
}

// NewLexer creates a new Lexer for the given input string.
func NewLexer(input string) *Lexer {
	l := &Lexer{input: input}
	l.readChar() // Initialize by reading the first character

	return l
}

// NextToken reads and returns the next token from the input.
func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	var tok Token

	startPosition := l.position

	switch l.ch {
	case '!':
		tok = NewToken(BANG, string(l.ch), startPosition)
		l.readChar()
	case '|':
		tok = NewToken(PIPE, string(l.ch), startPosition)
		l.readChar()
	case '=':
		tok = NewToken(EQUAL, string(l.ch), startPosition)
		l.readChar()
	case '{':
		tok = NewToken(LBRACE, string(l.ch), startPosition)
		l.readChar()
	case '}':
		tok = NewToken(RBRACE, string(l.ch), startPosition)
		l.readChar()
	case 0:
		tok = NewToken(EOF, "", startPosition)
	case '.':
		// Check if this is the start of a path (./) or a hidden file
		switch nextCh := l.peekChar(); {
		case nextCh == '/':
			tok = l.readPath(startPosition)
		case isIdentifierChar(nextCh):
			// Hidden file like .gitignore, .hidden
			literal := l.readIdentifier()
			tok = NewToken(IDENT, literal, startPosition)

			return tok
		default:
			// Single dot alone is not valid in our syntax
			tok = NewToken(ILLEGAL, string(l.ch), startPosition)
			l.readChar()
		}
	case '/':
		// Absolute path
		tok = l.readPath(startPosition)
	default:
		if isIdentifierStart(l.ch) {
			literal := l.readIdentifier()
			tok = NewToken(IDENT, literal, startPosition)

			return tok
		}

		tok = NewToken(ILLEGAL, string(l.ch), startPosition)
		l.readChar()
	}

	return tok
}

// readChar advances the lexer's position and updates the current character.
func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0 // ASCII code for "NUL", signifies end of input
		l.position = l.readPosition
		l.readPosition++

		return
	}

	l.ch = l.input[l.readPosition]
	l.position = l.readPosition
	l.readPosition++
}

// peekChar returns the next character without advancing the position.
func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}

	return l.input[l.readPosition]
}

// skipWhitespace skips over whitespace characters.
func (l *Lexer) skipWhitespace() {
	for l.ch != 0 && unicode.IsSpace(rune(l.ch)) {
		l.readChar()
	}
}

// readIdentifier reads an identifier from the input.
// Identifiers can contain letters, numbers, underscores, hyphens, dots, and other non-special chars.
// This includes hidden files starting with a dot like .gitignore
// Trailing whitespace is trimmed.
func (l *Lexer) readIdentifier() string {
	position := l.position
	for isIdentifierChar(l.ch) {
		l.readChar()
	}

	literal := l.input[position:l.position]
	// Trim trailing whitespace
	return strings.TrimRight(literal, " \t\n\r")
}

// readPath reads a path from the input.
// Paths can contain any characters except special operators.
// Trailing whitespace is trimmed.
func (l *Lexer) readPath(startPosition int) Token {
	position := l.position
	for isPathChar(l.ch) {
		l.readChar()
	}

	literal := l.input[position:l.position]
	// Trim trailing whitespace
	literal = strings.TrimSpace(literal)

	return NewToken(PATH, literal, startPosition)
}

// isSpecialChar returns true if the character is a special operator or delimiter.
// Note: spaces are NOT special - they can be part of identifiers and paths.
// Only the operators and delimiters themselves act as separators.
func isSpecialChar(ch byte) bool {
	return ch == '!' || ch == '|' || ch == '=' || ch == '{' || ch == '}' || ch == 0
}

// isPathSeparator returns true if the character is a path separator.
func isPathSeparator(ch byte) bool {
	return ch == '/'
}

// isIdentifierStart returns true if the character can start an identifier.
// Identifiers can start with letters, underscores, or dots (for hidden files).
func isIdentifierStart(ch byte) bool {
	return !isSpecialChar(ch) && !isPathSeparator(ch)
}

// isIdentifierChar returns true if the character can be part of an identifier.
// Uses negative logic: anything that's not a special character or path separator is valid.
func isIdentifierChar(ch byte) bool {
	return !isSpecialChar(ch) && !isPathSeparator(ch)
}

// isPathChar returns true if the character can be part of a path.
// Paths can contain anything except special operators and whitespace.
func isPathChar(ch byte) bool {
	return !isSpecialChar(ch)
}
