package preprocess

import (
	"github.com/hashicorp/hcl/v2/hclwrite"
	"strings"
)

func attrValueAsString(attr *hclwrite.Attribute) *string {
	if attr == nil {
		return nil
	}

	asString := string(attr.Expr().BuildTokens(nil).Bytes())

	// Clean up the value by stripping whitespace and wrapping quotes
	asString = strings.TrimSpace(asString)
	asString = strings.TrimPrefix(asString, `"`)
	asString = strings.TrimSuffix(asString, `"`)

	return &asString
}
