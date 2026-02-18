package filter

import (
	"strconv"
	"strings"
)

// Parser parses a filter query string into an AST.
type Parser struct {
	lexer         *Lexer
	errors        []error
	originalQuery string
	curToken      Token
	peekToken     Token
}

// Operator precedence levels
const (
	_ int = iota
	LOWEST
	INTERSECTION // |
	PREFIX       // !
)

// precedences maps token types to their precedence levels
var precedences = map[TokenType]int{
	PIPE: INTERSECTION,
}

// NewParser creates a new Parser for the given lexer.
func NewParser(lexer *Lexer) *Parser {
	p := &Parser{
		lexer:         lexer,
		errors:        []error{},
		originalQuery: lexer.Input(), // Capture original input for diagnostics
	}

	// Read two tokens to initialize curToken and peekToken
	p.nextToken()
	p.nextToken()

	return p
}

// ParseExpression parses and returns an expression from the input.
func (p *Parser) ParseExpression() (Expression, error) {
	expr := p.parseExpression(LOWEST)

	if expr == nil {
		if len(p.errors) > 0 {
			return nil, p.errors[0]
		}

		return nil, p.createError(ErrorCodeUnknown, "Parse error", "failed to parse expression")
	}

	if p.curToken.Type != EOF {
		return nil, p.createError(ErrorCodeUnexpectedToken, "Unexpected token", "Unexpected '"+p.curToken.Literal+"' after expression")
	}

	return expr, nil
}

// createError creates a ParseError with full context for rich diagnostics.
func (p *Parser) createError(code ErrorCode, title, msg string) error {
	tokenLen := len(p.curToken.Literal)
	if tokenLen == 0 {
		tokenLen = 1 // Minimum length for underline
	}

	return NewParseErrorWithContext(
		title,
		msg,
		p.curToken.Position,
		p.curToken.Position,
		p.originalQuery,
		p.curToken.Literal,
		tokenLen,
		code,
	)
}

// Errors returns any parsing errors that occurred.
func (p *Parser) Errors() []error {
	return p.errors
}

// nextToken advances to the next token.
func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.lexer.NextToken()
}

// parseExpression is the core recursive descent parser.
func (p *Parser) parseExpression(precedence int) Expression {
	// Check for prefix depth (N...foo) or ellipsis (...foo)
	includeDependents := false
	dependentDepth := 0

	// Check for N... (number followed by ellipsis = dependent depth)
	if isPurelyNumeric(p.curToken.Literal) && p.peekToken.Type == ELLIPSIS {
		includeDependents = true
		dependentDepth = parseDepth(p.curToken.Literal)
		p.nextToken() // consume number
		p.nextToken() // consume ellipsis
	} else if p.curToken.Type == ELLIPSIS {
		includeDependents = true

		p.nextToken()
	}

	// Check for caret (^) for exclusion
	excludeTarget := false
	if p.curToken.Type == CARET {
		excludeTarget = true

		p.nextToken()
	}

	var leftExpr Expression

	switch p.curToken.Type {
	case BANG:
		leftExpr = p.parsePrefixExpression()
	case PATH:
		leftExpr = p.parsePathFilter()
	case LBRACE:
		leftExpr = p.parseBracedPath()
	case LBRACKET:
		leftExpr = p.parseGitFilter()
	case IDENT:
		if p.peekToken.Type == EQUAL {
			leftExpr = p.parseAttributeFilter()

			break
		}

		leftExpr = &AttributeExpression{Key: "name", Value: p.curToken.Literal}
		p.nextToken()
	case ILLEGAL:
		p.addErrorWithCode(ErrorCodeIllegalToken, "Illegal token", "Unrecognized character '"+p.curToken.Literal+"'")
		return nil
	case EOF:
		p.addErrorWithCode(ErrorCodeUnexpectedEOF, "Unexpected end of input", "Expression is incomplete")
		return nil
	case PIPE:
		p.addErrorWithCode(ErrorCodeUnexpectedToken, "Unexpected token", "Missing left-hand side of '|' operator")
	case EQUAL, RBRACE, RBRACKET, ELLIPSIS, CARET:
		p.addErrorWithCode(ErrorCodeUnexpectedToken, "Unexpected token", "Unexpected '"+p.curToken.Literal+"'")
		return nil
	default:
		p.addErrorWithCode(ErrorCodeUnexpectedToken, "Unexpected token", "Unexpected '"+p.curToken.Literal+"'")
		return nil
	}

	if leftExpr == nil {
		return nil
	}

	target := leftExpr

	// Check for postfix ellipsis (foo... or foo...N)
	includeDependencies := false
	dependencyDepth := 0

	if p.curToken.Type == ELLIPSIS {
		includeDependencies = true

		p.nextToken()

		// Check for ...N (ellipsis followed by number = dependency depth)
		if isPurelyNumeric(p.curToken.Literal) {
			dependencyDepth = parseDepth(p.curToken.Literal)
			p.nextToken()
		}
	}

	// If we have any graph operators, wrap in GraphExpression
	if includeDependents || includeDependencies || excludeTarget {
		leftExpr = &GraphExpression{
			Target:              target,
			IncludeDependents:   includeDependents,
			IncludeDependencies: includeDependencies,
			ExcludeTarget:       excludeTarget,
			DependentDepth:      dependentDepth,
			DependencyDepth:     dependencyDepth,
		}
	}

	for p.curToken.Type != EOF && precedence < p.curPrecedence() {
		switch p.curToken.Type {
		case PIPE:
			leftExpr = p.parseInfixExpression(leftExpr)
		case ILLEGAL, EOF, IDENT, PATH, BANG, EQUAL, LBRACE, RBRACE, LBRACKET, RBRACKET, ELLIPSIS, CARET:
			return leftExpr
		default:
			return leftExpr
		}
	}

	return leftExpr
}

// isPurelyNumeric returns true if the string contains only digits.
func isPurelyNumeric(s string) bool {
	if len(s) == 0 {
		return false
	}

	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}

	return true
}

// parseDepth parses a depth value from a numeric string.
// Returns 0 (unlimited) if parsing fails. Clamps to MaxTraversalDepth for very large values.
func parseDepth(literal string) int {
	depth, err := strconv.Atoi(literal)
	if err != nil || depth < 0 {
		return 0
	}

	if depth > MaxTraversalDepth {
		return MaxTraversalDepth
	}

	return depth
}

// parsePrefixExpression parses a prefix expression (e.g., "!name=foo").
// It collapses consecutive negations: !! becomes positive, !!! becomes negative, etc.
func (p *Parser) parsePrefixExpression() Expression {
	// Count consecutive negation operators
	negationCount := 0
	for p.curToken.Type == BANG {
		negationCount++

		p.nextToken()
	}

	// Parse the inner expression
	inner := p.parseExpression(PREFIX)
	if inner == nil {
		// Clear any errors from parseExpression (like generic EOF error)
		// and add our specific error with the EOF title for consistency
		p.errors = nil
		p.addMissingOperandError("Unexpected end of input", "Missing target expression for '!' operator")

		return nil
	}

	// If even number of negations, they cancel out - return inner expression directly
	if negationCount%2 == 0 {
		return inner
	}

	// Odd number of negations - wrap in single PrefixExpression
	return &PrefixExpression{
		Operator: "!",
		Right:    inner,
	}
}

// parseInfixExpression parses an infix expression (e.g., "./apps/* | name=bar").
func (p *Parser) parseInfixExpression(left Expression) Expression {
	expression := &InfixExpression{
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	if expression.Right == nil {
		// Clear any errors from parseExpression (like generic EOF error)
		// and add our specific error with the EOF title for consistency
		p.errors = nil
		p.addMissingOperandError("Unexpected end of input", "Missing right-hand side of '|' operator")

		return nil
	}

	return expression
}

// parsePathFilter parses a path filter (e.g., "./apps/*").
func (p *Parser) parsePathFilter() Expression {
	expr := NewPathFilter(p.curToken.Literal)
	p.nextToken()

	return expr
}

// parseBracedPath parses a braced path filter (e.g., "{./apps/*}" or "{my path}").
func (p *Parser) parseBracedPath() Expression {
	// Capture opening brace position for error reporting
	openBracePos := p.curToken.Position

	// We're currently at LBRACE, move to the content
	p.nextToken()

	if p.curToken.Type == RBRACE {
		p.addErrorWithCode(ErrorCodeEmptyExpression, "Empty path expression", "Braced path expression cannot be empty")
		return nil
	}

	// Read everything until RBRACE as the path
	var pathParts []string
	for p.curToken.Type != RBRACE && p.curToken.Type != EOF {
		pathParts = append(pathParts, p.curToken.Literal)
		p.nextToken()
	}

	if p.curToken.Type != RBRACE {
		p.addErrorAtPosition(ErrorCodeMissingClosingBrace, "Unclosed path expression", "This braced path expression is missing a closing '}'", openBracePos)
		return nil
	}

	// Move past RBRACE
	p.nextToken()

	// Join all parts to form the complete path
	pathValue := strings.Join(pathParts, "")

	return NewPathFilter(pathValue)
}

// parseAttributeFilter parses an attribute filter (e.g., "name=foo").
func (p *Parser) parseAttributeFilter() Expression {
	key := p.curToken.Literal

	if !p.expectPeek(EQUAL) {
		return nil
	}

	p.nextToken()

	if p.curToken.Type != IDENT && p.curToken.Type != PATH {
		p.addErrorWithCode(ErrorCodeUnexpectedToken, "Attribute expression missing value", "Attribute expressions require a value after '='")
		return nil
	}

	value := p.curToken.Literal
	p.nextToken()

	return &AttributeExpression{
		Key:   key,
		Value: value,
	}
}

// parseGitFilter parses a Git filter expression (e.g., "[main...HEAD]" or "[main]").
func (p *Parser) parseGitFilter() Expression {
	// Capture opening bracket position for error reporting
	openBracketPos := p.curToken.Position

	// We're currently at LBRACKET, move to the content
	p.nextToken()

	if p.curToken.Type == RBRACKET {
		p.addErrorWithCode(ErrorCodeEmptyGitFilter, "Empty Git filter", "Git filter expression cannot be empty")
		return nil
	}

	// Read the first reference (can be IDENT or PATH-like)
	var fromRefParts []string
	for p.curToken.Type != RBRACKET && p.curToken.Type != ELLIPSIS && p.curToken.Type != EOF {
		fromRefParts = append(fromRefParts, p.curToken.Literal)
		p.nextToken()
	}

	if len(fromRefParts) == 0 {
		p.addErrorWithCode(ErrorCodeMissingGitRef, "Missing Git reference", "Expected Git reference in filter")
		return nil
	}

	fromRef := strings.Join(fromRefParts, "")

	// Check if there's an ellipsis and second reference
	if p.curToken.Type == ELLIPSIS {
		// Move past ellipsis
		p.nextToken()

		// Read the second reference
		var toRefParts []string
		for p.curToken.Type != RBRACKET && p.curToken.Type != EOF {
			toRefParts = append(toRefParts, p.curToken.Literal)
			p.nextToken()
		}

		if len(toRefParts) == 0 {
			p.addErrorWithCode(ErrorCodeMissingGitRef, "Missing Git reference", "Expected second Git reference after '...'")
			return nil
		}

		toRef := strings.Join(toRefParts, "")

		if p.curToken.Type != RBRACKET {
			p.addErrorAtPosition(ErrorCodeMissingClosingBracket, "Unclosed Git filter expression", "This Git-based expression is missing a closing ']'", openBracketPos)
			return nil
		}

		// Move past RBRACKET
		p.nextToken()

		return NewGitExpression(fromRef, toRef)
	}

	// Single reference case
	if p.curToken.Type != RBRACKET {
		p.addErrorAtPosition(ErrorCodeMissingClosingBracket, "Unclosed Git filter expression", "This Git-based expression is missing a closing ']'", openBracketPos)
		return nil
	}

	// Move past RBRACKET
	p.nextToken()

	return NewGitExpression(fromRef, "HEAD")
}

// expectPeek checks if the next token is of the expected type and advances if so.
func (p *Parser) expectPeek(t TokenType) bool {
	if p.peekToken.Type == t {
		p.nextToken()
		return true
	}

	p.addError("expected next token to be " + t.String() + ", got " + p.peekToken.Type.String())

	return false
}

// curPrecedence returns the precedence of the current token.
func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}

	return LOWEST
}

// addError adds an error to the parser's error list.
func (p *Parser) addError(msg string) {
	p.addErrorWithCode(ErrorCodeUnknown, "Parse error", msg)
}

// addErrorWithCode adds an error with a specific error code for hint lookup.
func (p *Parser) addErrorWithCode(code ErrorCode, title, msg string) {
	tokenLen := len(p.curToken.Literal)
	if tokenLen == 0 {
		tokenLen = 1 // Minimum length for underline
	}

	err := NewParseErrorWithContext(
		title,
		msg,
		p.curToken.Position,
		p.curToken.Position,
		p.originalQuery,
		p.curToken.Literal,
		tokenLen,
		code,
	)
	p.errors = append(p.errors, err)
}

// addMissingOperandError adds a MissingOperand error with a custom title.
// This is used when a more specific error replaces a generic EOF error.
func (p *Parser) addMissingOperandError(title, msg string) {
	tokenLen := len(p.curToken.Literal)
	if tokenLen == 0 {
		tokenLen = 1 // Minimum length for underline
	}

	err := NewParseErrorWithContext(
		title,
		msg,
		p.curToken.Position,
		p.curToken.Position,
		p.originalQuery,
		p.curToken.Literal,
		tokenLen,
		ErrorCodeMissingOperand,
	)
	p.errors = append(p.errors, err)
}

// addErrorAtPosition adds an error with a specific error code and custom error position for caret placement.
func (p *Parser) addErrorAtPosition(code ErrorCode, title, msg string, errorPosition int) {
	tokenLen := 1 // Single character underline for bracket errors

	err := NewParseErrorWithContext(
		title,
		msg,
		p.curToken.Position,
		errorPosition,
		p.originalQuery,
		p.curToken.Literal,
		tokenLen,
		code,
	)
	p.errors = append(p.errors, err)
}
