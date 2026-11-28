package hclhelper

import (
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// TokensForValue returns the tokens of [hclwrite.TokensForValue] for value, with every object and map key named "for"
// quoted. hclwrite leaves identifier keys bare, and a bare "for" as the first key of an object parses as a for
// expression.
//
// A quoted key keeps the spacing of the bare one. Writing the tokens through an [hclwrite.File] or [hclwrite.Format]
// realigns its line.
//
// Panics on a value [hclwrite.TokensForValue] panics on, e.g. one that holds an unknown value.
func TokensForValue(value cty.Value) hclwrite.Tokens {
	tokens := hclwrite.TokensForValue(value)
	quoted := make(hclwrite.Tokens, 0, len(tokens))

	for _, token := range tokens {
		// true, false, null, and bare object keys are the only identifiers among the tokens of a value.
		if token.Type != hclsyntax.TokenIdent || string(token.Bytes) != "for" {
			quoted = append(quoted, token)

			continue
		}

		quoted = append(
			quoted,
			&hclwrite.Token{Type: hclsyntax.TokenOQuote, Bytes: []byte{'"'}, SpacesBefore: token.SpacesBefore},
			&hclwrite.Token{Type: hclsyntax.TokenQuotedLit, Bytes: token.Bytes},
			&hclwrite.Token{Type: hclsyntax.TokenCQuote, Bytes: []byte{'"'}},
		)
	}

	return quoted
}
