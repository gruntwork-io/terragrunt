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
	afterEqual   bool   // True if the last token was EQUAL (for parsing attribute values)
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
		l.afterEqual = true

		return tok
	case '{':
		tok = NewToken(LBRACE, string(l.ch), startPosition)
		l.readChar()
	case '}':
		tok = NewToken(RBRACE, string(l.ch), startPosition)
		l.readChar()
	case '[':
		tok = NewToken(LBRACKET, string(l.ch), startPosition)
		l.readChar()
	case ']':
		tok = NewToken(RBRACKET, string(l.ch), startPosition)
		l.readChar()
	case '^':
		tok = NewToken(CARET, string(l.ch), startPosition)
		l.readChar()
	case 0:
		tok = NewToken(EOF, "", startPosition)
	case '.':
		// Check for ellipsis (three consecutive dots)
		if l.peekChar() == '.' {
			if l.readPosition+1 < len(l.input) && l.input[l.readPosition+1] == '.' {
				l.readChar()
				l.readChar()

				tok = NewToken(ELLIPSIS, "...", startPosition)

				l.readChar()

				return tok
			}
		}

		switch nextCh := l.peekChar(); {
		case nextCh == '/':
			tok = l.readPath(startPosition)
		case isIdentifierChar(nextCh):
			literal := l.readIdentifier()
			tok = NewToken(IDENT, literal, startPosition)

			return tok
		default:
			tok = NewToken(ILLEGAL, string(l.ch), startPosition)
			l.readChar()
		}
	case '/':
		tok = l.readPath(startPosition)
	default:
		if l.afterEqual {
			// After '=', read as attribute value (can contain slashes)
			literal := l.readAttributeValue()
			tok = NewToken(IDENT, literal, startPosition)
			l.afterEqual = false

			return tok
		}

		if isIdentifierChar(l.ch) {
			literal := l.readIdentifier()
			tok = NewToken(IDENT, literal, startPosition)

			return tok
		}

		tok = NewToken(ILLEGAL, string(l.ch), startPosition)
		l.readChar()
	}

	l.afterEqual = false

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
		// Check if we're about to read an ellipsis (...)
		if l.ch == '.' && l.peekChar() == '.' {
			if l.readPosition+1 < len(l.input) && l.input[l.readPosition+1] == '.' {
				break
			}
		}

		l.readChar()
	}

	literal := l.input[position:l.position]

	return strings.TrimSpace(literal)
}

// readAttributeValue reads an attribute value from the input.
// Attribute values can contain slashes, letters, numbers, underscores, hyphens, dots, etc.
// They stop at special operators (|, !, {, }) or end of input.
// Trailing whitespace is trimmed.
func (l *Lexer) readAttributeValue() string {
	position := l.position
	for isAttributeValueChar(l.ch) {
		// Check if we're about to read an ellipsis (...)
		if l.ch == '.' && l.peekChar() == '.' {
			if l.readPosition+1 < len(l.input) && l.input[l.readPosition+1] == '.' {
				break
			}
		}

		l.readChar()
	}

	literal := l.input[position:l.position]

	return strings.TrimSpace(literal)
}

// readPath reads a path from the input.
// Paths can contain any characters except special operators.
// Trailing whitespace is trimmed.
func (l *Lexer) readPath(startPosition int) Token {
	position := l.position
	for isPathChar(l.ch) {
		// Check if we're about to read an ellipsis (...)
		if l.ch == '.' && l.peekChar() == '.' {
			if l.readPosition+1 < len(l.input) && l.input[l.readPosition+1] == '.' {
				break
			}
		}

		l.readChar()
	}

	literal := l.input[position:l.position]

	literal = strings.TrimSpace(literal)

	return NewToken(PATH, literal, startPosition)
}

// isSpecialChar returns true if the character is a special operator or delimiter.
func isSpecialChar(ch byte) bool {
	return ch == '!' || ch == '|' || ch == '=' || ch == '{' || ch == '}' || ch == '[' || ch == ']' || ch == '^' || ch == 0
}

// isPathSeparator returns true if the character is a path separator.
func isPathSeparator(ch byte) bool {
	return ch == '/'
}

// isIdentifierChar returns true if the character can be part of an identifier.
func isIdentifierChar(ch byte) bool {
	return !isSpecialChar(ch) && !isPathSeparator(ch)
}

// isAttributeValueChar returns true if the character can be part of an attribute value.
// Attribute values can contain slashes (unlike regular identifiers).
func isAttributeValueChar(ch byte) bool {
	return !isSpecialChar(ch)
}

// isPathChar returns true if the character can be part of a path.
func isPathChar(ch byte) bool {
	return !isSpecialChar(ch)
}
