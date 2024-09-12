package diagnostic

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

const (
	// Sensitive indicates that this value is marked as sensitive in the context of Terraform.
	Sensitive = valueMark("Sensitive")
)

// valueMarks allow creating strictly typed values for use as cty.Value marks.
type valueMark string

func (m valueMark) GoString() string {
	return "marks." + string(m)
}

// ExpressionValue represents an HCL traversal string and a statement about its value while the expression was evaluated.
type ExpressionValue struct {
	Traversal string `json:"traversal"`
	Statement string `json:"statement"`
}

func DescribeExpressionValues(hclDiag *hcl.Diagnostic) []ExpressionValue {
	var (
		expr = hclDiag.Expression
		ctx  = hclDiag.EvalContext

		vars             = expr.Variables()
		values           = make([]ExpressionValue, 0, len(vars))
		seen             = make(map[string]struct{}, len(vars))
		includeUnknown   = DiagnosticCausedByUnknown(hclDiag)
		includeSensitive = DiagnosticCausedBySensitive(hclDiag)
	)

Traversals:
	for _, traversal := range vars {
		for len(traversal) > 1 {
			val, diags := traversal.TraverseAbs(ctx)
			if diags.HasErrors() {
				traversal = traversal[:len(traversal)-1]
				continue
			}

			traversalStr := traversalStr(traversal)
			if _, exists := seen[traversalStr]; exists {
				continue Traversals
			}
			value := ExpressionValue{
				Traversal: traversalStr,
			}
			switch {

			case val.HasMark(Sensitive):
				if !includeSensitive {
					continue Traversals
				}
				value.Statement = "has a sensitive value"
			case !val.IsKnown():
				if ty := val.Type(); ty != cty.DynamicPseudoType {
					if includeUnknown {
						value.Statement = fmt.Sprintf("is a %s, known only after apply", ty.FriendlyName())
					} else {
						value.Statement = "is a " + ty.FriendlyName()
					}
				} else {
					if !includeUnknown {
						continue Traversals
					}
					value.Statement = "will be known only after apply"
				}
			default:
				value.Statement = "is " + valueStr(val)
			}
			values = append(values, value)
			seen[traversalStr] = struct{}{}
		}
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].Traversal < values[j].Traversal
	})

	return values
}

func traversalStr(traversal hcl.Traversal) string {
	var buf bytes.Buffer

	for _, step := range traversal {
		switch tStep := step.(type) {
		case hcl.TraverseRoot:
			buf.WriteString(tStep.Name)
		case hcl.TraverseAttr:
			buf.WriteByte('.')
			buf.WriteString(tStep.Name)
		case hcl.TraverseIndex:
			buf.WriteByte('[')

			if keyTy := tStep.Key.Type(); keyTy.IsPrimitiveType() {
				buf.WriteString(valueStr(tStep.Key))
			} else {
				// We'll just use a placeholder for more complex values, since otherwise our result could grow ridiculously long.
				buf.WriteString("...")
			}

			buf.WriteByte(']')
		}
	}

	return buf.String()
}

func valueStr(val cty.Value) string {
	if val.HasMark(Sensitive) {
		return "(sensitive value)"
	}

	ty := val.Type()

	switch {
	case val.IsNull():
		return "null"
	case !val.IsKnown():
		return "(not yet known)"
	case ty == cty.Bool:
		if val.True() {
			return "true"
		}

		return "false"
	case ty == cty.Number:
		bf := val.AsBigFloat()
		prec := 10

		return bf.Text('g', prec)
	case ty == cty.String:
		return fmt.Sprintf("%q", val.AsString())
	case ty.IsCollectionType() || ty.IsTupleType():
		l := val.LengthInt()
		switch l {
		case 0:
			return "empty " + ty.FriendlyName()
		case 1:
			return ty.FriendlyName() + " with 1 element"
		default:
			return fmt.Sprintf("%s with %d elements", ty.FriendlyName(), l)
		}
	case ty.IsObjectType():
		atys := ty.AttributeTypes()
		l := len(atys)

		switch l {
		case 0:
			return "object with no attributes"
		case 1:
			var name string
			for k := range atys {
				name = k
			}

			return fmt.Sprintf("object with 1 attribute %q", name)
		default:
			return fmt.Sprintf("object with %d attributes", l)
		}
	default:
		return ty.FriendlyName()
	}
}
