package valuesblock

import (
	"fmt"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

const (
	blockName      = "values"
	valueBlockName = "value"
)

const (
	typeAttr        = "type"
	defaultAttr     = "default"
	descriptionAttr = "description"
)

var (
	valuesSchema = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: blockName}},
	}
	valuesBlockSchema = &hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: valueBlockName, LabelNames: []string{"name"}}},
	}
	valueBlockSchema = &hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: typeAttr}, {Name: defaultAttr}, {Name: descriptionAttr}},
	}
)

// Declarations is a file's values block.
type Declarations struct {
	// Values holds the value blocks in source order.
	Values []Declaration
	// DefRange is the header of the values block.
	DefRange hcl.Range
}

// Declaration is one value block.
type Declaration struct {
	// Default is the default converted to Type, or nil when the value is required.
	Default *cty.Value
	// TypeDefaults holds the optional attribute defaults of Type, or nil when Type has none.
	TypeDefaults *typeexpr.Defaults
	// Type is the type constraint, with any optional attributes. It is [cty.DynamicPseudoType] when the block sets no
	// type.
	Type cty.Type
	// Name is the block label.
	Name string
	// Description is the description, or the empty string when the block sets none.
	Description string
	// DefRange is the header of the value block.
	DefRange hcl.Range
}

// Decode reads the values block of body, ignoring everything else in body. It returns nil when body has no values
// block.
//
// Reports a diagnostic for a second values block, a value name that is not a valid identifier, a name declared twice,
// an unexpected attribute or block, an invalid type constraint, a default or description that is not a constant, a
// default that does not conform to the type, and a description that is not a string.
//
// A value block whose type or default fails to decode accepts any value and resolves to an unknown value when nothing
// sets it.
func Decode(body hcl.Body) (*Declarations, hcl.Diagnostics) {
	content, _, diags := body.PartialContent(valuesSchema)
	if len(content.Blocks) == 0 {
		return nil, diags
	}

	first := content.Blocks[0]

	for _, extra := range content.Blocks[1:] {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Duplicate " + blockName + " block",
			Detail: fmt.Sprintf(
				"Only one %s block is allowed. Another was defined at %s.",
				blockName,
				first.DefRange,
			),
			Subject: extra.DefRange.Ptr(),
		})
	}

	decls := &Declarations{DefRange: first.DefRange}

	inner, innerDiags := first.Body.Content(valuesBlockSchema)
	diags = append(diags, innerDiags...)

	declared := make(map[string]hcl.Range, len(inner.Blocks))

	for _, block := range inner.Blocks {
		decl, declDiags := decodeValue(block)
		diags = append(diags, declDiags...)

		if earlier, ok := declared[decl.Name]; ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Duplicate value declaration",
				Detail:   fmt.Sprintf("The value %q was already declared at %s.", decl.Name, earlier),
				Subject:  block.LabelRanges[0].Ptr(),
			})

			continue
		}

		declared[decl.Name] = block.DefRange
		decls.Values = append(decls.Values, decl)
	}

	return decls, diags
}

// Names returns the declared value names in sorted order.
func (d *Declarations) Names() []string {
	names := make([]string, 0, len(d.Values))

	for i := range d.Values {
		names = append(names, d.Values[i].Name)
	}

	slices.Sort(names)

	return names
}

// Lookup returns the declaration of the value name, and false when the block does not declare it.
func (d *Declarations) Lookup(name string) (*Declaration, bool) {
	i := slices.IndexFunc(d.Values, func(decl Declaration) bool { return decl.Name == name })
	if i < 0 {
		return nil, false
	}

	return &d.Values[i], true
}

// convert applies the optional attribute defaults of d to val and converts the result to the type of d.
func (d *Declaration) convert(val cty.Value) (cty.Value, error) {
	if d.TypeDefaults != nil && !val.IsNull() {
		val = d.TypeDefaults.Apply(val)
	}

	return convert.Convert(val, d.Type)
}

// decodeValue reads one value block. On a type or default error the declaration accepts any value and defaults to an
// unknown value.
func decodeValue(block *hcl.Block) (Declaration, hcl.Diagnostics) {
	decl := Declaration{
		Name:     block.Labels[0],
		Type:     cty.DynamicPseudoType,
		DefRange: block.DefRange,
	}

	var diags hcl.Diagnostics

	if !hclsyntax.ValidIdentifier(decl.Name) {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid value name",
			Detail: fmt.Sprintf(
				"The value name %q is not a valid identifier. A name must start with a letter or underscore and "+
					"may contain only letters, digits, underscores and dashes.",
				decl.Name,
			),
			Subject: block.LabelRanges[0].Ptr(),
		})
	}

	content, contentDiags := block.Body.Content(valueBlockSchema)
	diags = append(diags, contentDiags...)

	if attr, ok := content.Attributes[descriptionAttr]; ok {
		description, descriptionDiags := decodeDescription(decl.Name, attr)
		diags = append(diags, descriptionDiags...)
		decl.Description = description
	}

	typeDiags := decl.decodeType(content.Attributes[typeAttr])
	diags = append(diags, typeDiags...)

	if typeDiags.HasErrors() {
		decl.acceptAnything()

		return decl, diags
	}

	attr, ok := content.Attributes[defaultAttr]
	if !ok {
		return decl, diags
	}

	defaultDiags := decl.decodeDefault(attr)
	diags = append(diags, defaultDiags...)

	if defaultDiags.HasErrors() {
		decl.acceptAnything()
	}

	return decl, diags
}

// decodeType sets the type constraint of d from attr, which is nil when the block sets no type.
func (d *Declaration) decodeType(attr *hcl.Attribute) hcl.Diagnostics {
	if attr == nil {
		return nil
	}

	ty, defaults, diags := typeexpr.TypeConstraintWithDefaults(attr.Expr)
	if diags.HasErrors() {
		return diags
	}

	d.Type = ty
	d.TypeDefaults = defaults

	return diags
}

// decodeDefault sets the default of d from attr.
func (d *Declaration) decodeDefault(attr *hcl.Attribute) hcl.Diagnostics {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return diags
	}

	converted, err := d.convert(val)
	if err != nil {
		return append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Invalid default for value %q", d.Name),
			Detail: fmt.Sprintf(
				"The default does not match the type constraint of the value: %s.",
				formatConversionError(err),
			),
			Subject: attr.Expr.Range().Ptr(),
		})
	}

	d.Default = &converted

	return diags
}

// acceptAnything makes d accept any value and default to an unknown value.
func (d *Declaration) acceptAnything() {
	d.Type = cty.DynamicPseudoType
	d.TypeDefaults = nil
	d.Default = new(cty.DynamicVal)
}

// decodeDescription evaluates the description attribute of the value name.
func decodeDescription(name string, attr *hcl.Attribute) (string, hcl.Diagnostics) {
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() {
		return "", diags
	}

	description, err := convert.Convert(val, cty.String)
	if err != nil || description.IsNull() {
		return "", append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid description",
			Detail:   fmt.Sprintf("The description of value %q must be a string.", name),
			Subject:  attr.Expr.Range().Ptr(),
		})
	}

	return description.AsString(), diags
}
