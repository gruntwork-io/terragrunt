package util

import (
	"fmt"

	"github.com/gruntwork-io/terragrunt/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// SetAttributeToBodyValue sets an attribute with the given name on the given body to the given value. The value is
// assumed to be another HCL body. This method will wrap the given body in curly braces it and set it as the value of
// the specified attribute. For example, if you had a body like:
//
// example_body {}
//
// And an attribute value like:
//
// x = "xxx"
// y = 12345
// z = true
//
// Then calling SetAttributeToBodyValue(body, "foo", value) would give:
//
//	example_body {
//	  foo = {
//	    x = "xxx"
//	    y = 12345
//	    z = true
//	  }
//	}
//
// This method exists because, while hclwrite.Body has a SetAttributeValue method which works with primitive cty types
// (e.g., string, int, bool) and static cty map types, there doesn't seem to be any easy way to set an attribute to
// a dynamic / arbitrary type where the underlying fields aren't known until run-time. Therefore, this method is a handy
// way to set attributes to arbitrary HCL values known only at run-time.
func SetAttributeToBodyValue(body *hclwrite.Body, attributeName string, attributeObjectValue *hclwrite.Body) error {
	valueTokens := attributeObjectValue.BuildTokens(nil)

	attributeTokens := []*hclwrite.Token{}
	attributeTokens = append(attributeTokens, openBraceToken)

	// Add a newline if the user's value doesn't already start with a new line
	if !StartsWithNewLine(valueTokens) {
		attributeTokens = append(attributeTokens, newLineToken)
	}

	attributeTokens = append(attributeTokens, valueTokens...)

	// Add a newline if the user's value doesn't already end with a new line
	if !EndsWithNewLine(valueTokens) {
		attributeTokens = append(attributeTokens, newLineToken)
	}

	attributeTokens = append(attributeTokens, closeBraceToken)

	if body.GetAttribute(attributeName) == nil {
		body.AppendNewline()
	}

	body.SetAttributeRaw(attributeName, attributeTokens)

	return nil
}

// StartsWithNewLine returns true if the given tokens start with a new line
func StartsWithNewLine(tokens []*hclwrite.Token) bool {
	return len(tokens) > 0 && tokens[0].Type == hclsyntax.TokenNewline
}

// EndsWithNewLine returns true if the given tokens end with a new line
func EndsWithNewLine(tokens []*hclwrite.Token) bool {
	return len(tokens) > 0 && tokens[len(tokens)-1].Type == hclsyntax.TokenNewline
}

// EndsWithTwoNewLines returns true if the given tokens end with two new lines
func EndsWithTwoNewLines(tokens []*hclwrite.Token) bool {
	return len(tokens) > 1 && tokens[len(tokens)-1].Type == hclsyntax.TokenNewline && tokens[len(tokens)-2].Type == hclsyntax.TokenNewline
}

// SetAttributeRawFromString sets an attribute on the given body with the given name to the given value. The value is
// a string that can contain any arbitrary HCL expression in it. Note that the caller must format this string as a
// proper HCL value: for example, if it's a string, you must wrap it with double-quotes yourself (pass in "xxx", not
// xxx).
//
// This method exists because, while the hclwrite.Body type has a SetAttributeValue method which works with primitive
// cty types (e.g., string, int, bool), I found no easy way to set an attribute to totally arbitrary HCL values from a
// string. This method adds this capability through a somewhat hacky mechanism: we use hclwrite to parse the given
// string, turning it into the proper hclwrite types we need, and then we set those on the given body.
func SetAttributeRawFromString(body *hclwrite.Body, attributeName string, attributeValue string) error {
	parsedExpr, err := parseHclExpressionFromString(attributeValue)
	if err != nil {
		return err
	}

	if body.GetAttribute(attributeName) == nil {
		body.AppendNewline()
	}

	body.SetAttributeRaw(attributeName, parsedExpr.BuildTokens(nil))

	return nil
}

// parseHclExpressionFromString parses the given string as an HCL expression. This method exists because the
// hclwrite.Parse methods are designed to parse HCL files, but not individual expressions. For example, you can use
// the hclwrite.Parse methods to parse:
//
// foo = "bar"
//
// But you could not use those methods, directly, to parse:
//
// "bar"
//
// This method offers a way to parse the latter. We do this with a hacky mechanism: we take the value you pass in and
// turn it into an expression where we set a placeholder variable to your given expression, parse the whole thing with
// hclwrite.Parse, and then read out the value, already parsed for us.
func parseHclExpressionFromString(expression string) (*hclwrite.Expression, error) {
	hclExpression := fmt.Sprintf(`%s = %s`, placeholderAttrName, expression)

	parsed, err := ParseHclString(hclExpression)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	parsedAttributeValue := parsed.Body().GetAttribute(placeholderAttrName)

	// This should never be possible, but handle just in case, and ensure we don't panic below
	if parsedAttributeValue == nil {
		return nil, errors.WithStackTrace(PlaceholderAttributeNotFound(placeholderAttrName))
	}

	return parsedAttributeValue.Expr(), nil
}

// CloneBody clones the given hclwrite.Body. This is useful if you want to make changes to the given HCL without
// affecting the original object. There doesn't seem to be any native way in hclwrite.Body to clone their types, so
// this method is a hacky workaround where we re-parse all the HCL in the given body to get a totally new hclwrite.Body
// object.
func CloneBody(body *hclwrite.Body) (*hclwrite.Body, error) {
	tokens := body.BuildTokens(nil)
	parsed, err := ParseHclBytes(tokens.Bytes())
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	return parsed.Body(), nil
}

// ParseHclBytes parses the given byte slice, which is assumed to contain HCL. This is a convenience wrapper for the
// hclwrite.ParseConfig method that's useful when working in-memory, so there are no file names or starting positions
// to pass to the hclwrite.ParseConfig method.
func ParseHclBytes(hclBytes []byte) (*hclwrite.File, error) {
	file, diag := hclwrite.ParseConfig(hclBytes, placeholderFileName, hcl.InitialPos)
	if diag.HasErrors() {
		return nil, diag
	}
	return file, nil
}

// ParseHclString parses the given string, which is assumed to contain HCL. This is a convenience wrapper for the
// hclwrite.ParseConfig method that's useful when working in-memory, so there are no file names or starting positions
// to pass to the hclwrite.ParseConfig method.
func ParseHclString(hclStr string) (*hclwrite.File, error) {
	return ParseHclBytes([]byte(hclStr))
}

// The hclwrite.Parse methods require a file name. Sometimes, when working with HCL content in-memory, we have no file
// name available, so this is a fake file name that we use as a placeholder.
const placeholderFileName = "__internal__.tf"

// We sometimes need to use a fake attribute name with the hclwrite methods, so this is a fake, internal placeholder.
const placeholderAttrName = "__attr__"

var openBraceToken = &hclwrite.Token{
	Type:  hclsyntax.TokenOBrace,
	Bytes: []byte("{"),
}

var closeBraceToken = &hclwrite.Token{
	Type:  hclsyntax.TokenCBrace,
	Bytes: []byte("}"),
}

var newLineToken = &hclwrite.Token{
	Type:  hclsyntax.TokenNewline,
	Bytes: []byte("\n"),
}

// Custom error types

type PlaceholderAttributeNotFound string

func (err PlaceholderAttributeNotFound) Error() string {
	return fmt.Sprintf("Did not find placeholder attribute '%s' in HCL expression. This is a bug in Terragrunt. Please report it at: https://github.com/gruntwork-io/terragrunt/issues", string(err))
}
