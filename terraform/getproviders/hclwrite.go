package getproviders

import (
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// getAttributeValueAsString returns a value of Attribute as string. There is no way to get value as string directly, so we parses tokens of Attribute and build string representation.
func getAttributeValueAsUnquotedString(attr *hclwrite.Attribute) string {
	// find TokenEqual
	expr := attr.Expr()
	exprTokens := expr.BuildTokens(nil)

	// TokenIdent records SpaceBefore, but we should ignore it here.
	quotedValue := strings.TrimSpace(string(exprTokens.Bytes()))

	// unquote
	value := strings.Trim(quotedValue, "\"")

	return value
}

// tokensForListPerLine builds a hclwrite.Tokens for a given hashes, but breaks the line for each element.
func tokensForListPerLine(hashes []Hash) hclwrite.Tokens {
	// The original TokensForValue implementation does not break line by line for hashes, so we build a token sequence by ourselves.
	tokens := append(hclwrite.Tokens{},
		&hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte{'['}},
		&hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte{'\n'}})

	for _, hash := range hashes {
		ts := hclwrite.TokensForValue(cty.StringVal(hash.String()))
		tokens = append(tokens, ts...)
		tokens = append(tokens,
			&hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte{','}},
			&hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte{'\n'}})
	}

	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte{']'}})
	return tokens
}
