package filter

import "strings"

// Parser parses a filter query string into an AST.
type Parser struct {
	lexer     *Lexer
	errors    []error
	curToken  Token
	peekToken Token
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
func NewParser(lexer *Lexer, workingDir string) *Parser {
	p := &Parser{
		lexer:      lexer,
		errors:     []error{},
		workingDir: workingDir,
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

		return nil, NewParseError("failed to parse expression", p.curToken.Position)
	}

	// Verify we've consumed all input
	if p.curToken.Type != EOF {
		return nil, NewParseError("unexpected token after expression: "+p.curToken.Literal, p.curToken.Position)
	}

	return expr, nil
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
	// Parse prefix expression
	var leftExpr Expression

	switch p.curToken.Type {
	case BANG:
		leftExpr = p.parsePrefixExpression()
	case PATH:
		leftExpr = p.parsePathFilter()
	case LBRACE:
		leftExpr = p.parseBracedPath()
	case IDENT:
		// Check if this is an attribute filter (name=value) or just a name
		if p.peekToken.Type == EQUAL {
			leftExpr = p.parseAttributeFilter()
		} else {
			// Treat as a name filter (shorthand for name=value)
			leftExpr = &AttributeFilter{Key: "name", Value: p.curToken.Literal}
			p.nextToken()
		}
	case ILLEGAL:
		p.addError("illegal token: " + p.curToken.Literal)
		return nil
	case EOF:
		p.addError("unexpected end of input")
		return nil
	case PIPE, EQUAL, RBRACE:
		p.addError("unexpected token: " + p.curToken.Literal)
		return nil
	default:
		p.addError("unexpected token: " + p.curToken.Literal)
		return nil
	}

	if leftExpr == nil {
		return nil
	}

	// Parse infix expressions
	for p.curToken.Type != EOF && precedence < p.curPrecedence() {
		switch p.curToken.Type {
		case PIPE:
			leftExpr = p.parseInfixExpression(leftExpr)
		case ILLEGAL, EOF, IDENT, PATH, BANG, EQUAL, LBRACE, RBRACE:
			return leftExpr
		default:
			return leftExpr
		}
	}

	return leftExpr
}

// parsePrefixExpression parses a prefix expression (e.g., "!name=foo").
func (p *Parser) parsePrefixExpression() Expression {
	expression := &PrefixExpression{
		Operator: p.curToken.Literal,
	}

	p.nextToken()

	expression.Right = p.parseExpression(PREFIX)

	if expression.Right == nil {
		p.addError("expected expression after " + expression.Operator)
		return nil
	}

	return expression
}

// parseInfixExpression parses an infix expression (e.g., "name=foo, name=bar").
func (p *Parser) parseInfixExpression(left Expression) Expression {
	expression := &InfixExpression{
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	if expression.Right == nil {
		p.addError("expected expression after " + expression.Operator)
		return nil
	}

	return expression
}

// parsePathFilter parses a path filter (e.g., "./apps/*").
func (p *Parser) parsePathFilter() Expression {
	expr := NewPathFilter(p.curToken.Literal, p.workingDir)
	p.nextToken()

	return expr
}

// parseBracedPath parses a braced path filter (e.g., "{./apps/*}" or "{my path}").
func (p *Parser) parseBracedPath() Expression {
	// We're currently at LBRACE, move to the content
	p.nextToken()

	if p.curToken.Type == RBRACE {
		p.addError("empty braced path expression")
		return nil
	}

	// Read everything until RBRACE as the path
	var pathParts []string
	for p.curToken.Type != RBRACE && p.curToken.Type != EOF {
		pathParts = append(pathParts, p.curToken.Literal)
		p.nextToken()
	}

	if p.curToken.Type != RBRACE {
		p.addError("expected '}' to close braced path")
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
		p.addError("expected identifier or path after '='")
		return nil
	}

	value := p.curToken.Literal
	p.nextToken()

	return &AttributeFilter{
		Key:   key,
		Value: value,
	}
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
	p.errors = append(p.errors, NewParseError(msg, p.curToken.Position))
}
