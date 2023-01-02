package util

import (
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestSetAttributeToBodyValue(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                    string
		bodyHcl                 string
		attributeObjectValueHcl string
		expectedBodyHcl         string
	}{
		{"Empty block, set single attribute", emptyBlock, singleAttribute, emptyBlockWithSingleAttribute},
		{"Empty block, set multiple attributes", emptyBlock, multipleAttributes, emptyBlockWithMultipleAttributes},
		{"Empty block, set multiple attributes with nested blocks", emptyBlock, multipleAttributesWithNestedBlocks, emptyBlockWithMultipleAttributesWithNestedBlocks},
		{"Non empty block, set multiple attributes with nested blocks", nonEmptyBlock, multipleAttributesWithNestedBlocks, nonEmptyBlockWithMultipleAttributesWithNestedBlocks},
		{"Non empty block, set attribute that was already set previously", nonEmptyBlockWithExistingAttribute, multipleAttributes, emptyBlockWithMultipleAttributes},
	}

	for _, testCase := range testCases {
		// capture range variable to avoid it changing across for loop runs during goroutine transitions.
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			body := parseHclString(t, testCase.bodyHcl)

			targetBlock := body.Body().FirstMatchingBlock("target_block", nil)
			require.NotNil(t, targetBlock)

			attributeValue := parseHclString(t, testCase.attributeObjectValueHcl)

			setErr := SetAttributeToBodyValue(targetBlock.Body(), "target_attribute", attributeValue.Body())
			require.NoError(t, setErr)

			actualHcl := string(hclwrite.Format(body.Bytes()))
			require.Equal(t, normalizeHcl(testCase.expectedBodyHcl), actualHcl)
		})
	}
}

func TestSetAttributeRawFromString(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		bodyHcl           string
		attributeValueHcl string
		expectedBodyHcl   string
	}{
		{"Empty block, set to string expression", emptyBlock, stringExpression, emptyBlockWithStringExpression},
		{"Empty block, set to string expression with interpolation", emptyBlock, stringExpressionWithInterpolation, emptyBlockWithStringExpressionWithInterpolation},
		{"Empty block, set to numeric expression", emptyBlock, numericExpression, emptyBlockWithNumericExpression},
		{"Non empty block, set attribute that was already set previously to string expression", emptyBlock, stringExpression, nonEmptyBlockWithExistingAttributeWithStringExpression},
	}

	for _, testCase := range testCases {
		// capture range variable to avoid it changing across for loop runs during goroutine transitions.
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			body := parseHclString(t, testCase.bodyHcl)

			targetBlock := body.Body().FirstMatchingBlock("target_block", nil)
			require.NotNil(t, targetBlock)

			setErr := SetAttributeRawFromString(targetBlock.Body(), "target_attribute", testCase.attributeValueHcl)
			require.NoError(t, setErr)

			actualHcl := string(hclwrite.Format(body.Bytes()))
			require.Equal(t, normalizeHcl(testCase.expectedBodyHcl), actualHcl)
		})
	}
}

func TestCloneBody(t *testing.T) {
	t.Parallel()

	body := parseHclString(t, emptyBlock)

	clone, err := CloneBody(body.Body())
	require.NoError(t, err)

	clone.SetAttributeValue("foo", cty.StringVal("bar"))

	attrOnClone := clone.GetAttribute("foo")
	require.NotNil(t, attrOnClone)
	require.Equal(t, `"bar"`, string(attrOnClone.Expr().BuildTokens(nil).Bytes()))

	attrOnOriginal := body.Body().GetAttribute("foo")
	require.Nil(t, attrOnOriginal)
}

func parseHclString(t *testing.T, hclStr string) *hclwrite.File {
	file, err := ParseHclString(normalizeHcl(hclStr))
	require.NoError(t, err)

	return file
}

// We include whitespace in the test strings for readability, but for testing, to do comparisons, we need to strip
// all leading / trailing whitespace.
func normalizeHcl(hclStr string) string {
	return strings.TrimSpace(hclStr)
}

const emptyBlock = `
target_block {}`

const singleAttribute = `x = "xxx"`

const emptyBlockWithSingleAttribute = `
target_block {
  target_attribute = {
    x = "xxx"
  }
}
`

const multipleAttributes = `
x = "xxx"
y = 12345
z = true
`

const emptyBlockWithMultipleAttributes = `
target_block {
  target_attribute = {
    x = "xxx"
    y = 12345
    z = true
  }
}
`

const multipleAttributesWithNestedBlocks = `
x = "xxx"
y {
  y1 = [1, 2, 3]
  y2 = {
    abc = "def"
  }
}
y {
  y1 = [4, 5, 6]
  y2 = {
    abc = "ghi"
  }
}
z = {
  a = "b"
  c = "d"
  e = "f"
}
`

const emptyBlockWithMultipleAttributesWithNestedBlocks = `
target_block {
  target_attribute = {
    x = "xxx"
    y {
      y1 = [1, 2, 3]
      y2 = {
        abc = "def"
      }
    }
    y {
      y1 = [4, 5, 6]
      y2 = {
        abc = "ghi"
      }
    }
    z = {
      a = "b"
      c = "d"
      e = "f"
    }
  }
}
`

const nonEmptyBlock = `
target_block {
  x = "should not change"
}

other_block {
  should = "not change"
}

other_block {
  should = "not change"
}
`

const nonEmptyBlockWithMultipleAttributesWithNestedBlocks = `
target_block {
  x = "should not change"

  target_attribute = {
    x = "xxx"
    y {
      y1 = [1, 2, 3]
      y2 = {
        abc = "def"
      }
    }
    y {
      y1 = [4, 5, 6]
      y2 = {
        abc = "ghi"
      }
    }
    z = {
      a = "b"
      c = "d"
      e = "f"
    }
  }
}

other_block {
  should = "not change"
}

other_block {
  should = "not change"
}
`

const nonEmptyBlockWithExistingAttribute = `
target_block {
  target_attribute = "should be overwritten"
}
`

const stringExpression = `"foo"`

const emptyBlockWithStringExpression = `
target_block {
  target_attribute = "foo"
}
`

const stringExpressionWithInterpolation = `"${var.foo}-${var.bar}-${some_function_call()}"`

const emptyBlockWithStringExpressionWithInterpolation = `
target_block {
  target_attribute = "${var.foo}-${var.bar}-${some_function_call()}"
}
`

const numericExpression = `12345`

const emptyBlockWithNumericExpression = `
target_block {
  target_attribute = 12345
}
`

const nonEmptyBlockWithExistingAttributeWithStringExpression = `
target_block {
  target_attribute = "foo"
}
`
