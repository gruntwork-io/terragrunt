package valuesblock

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/upstream/tfdiags"
)

// Kind is the kind of component that declares values.
type Kind string

const (
	// KindUnit is a unit, declared in a terragrunt.hcl file.
	KindUnit Kind = "unit"
	// KindStack is a stack, declared in a terragrunt.stack.hcl file.
	KindStack Kind = "stack"
)

// Component names the declaring component in diagnostics.
type Component struct {
	// Kind is the kind of the component.
	Kind Kind
	// Address is the address of the unit or stack block that supplies the values, such as app["a"], or the empty
	// string when the values come from a terragrunt.values.hcl file.
	Address string
	// Location is where the component is, such as its config path or its source.
	Location string
}

// Resolve returns the values of the component c given s: an object with one attribute per declared value.
//
// Reports a diagnostic for a key that d does not declare, a value that does not convert to its type, a required value
// that s does not set, a null for a required value that is not nullable, and a supply that is not an object or map.
// Each value that fails becomes an unknown value of its type. One diagnostic names every required value that s does
// not set. Every diagnostic has a subject within the ranges s was built from.
//
// A value that s does not set takes its default. A null that s sets stays null for a nullable value, and takes the
// default otherwise.
func (d *Declarations) Resolve(s *Supplied, c Component) (cty.Value, hcl.Diagnostics) {
	supplied, marks := s.value.Unmark()

	if !supplied.IsKnown() {
		return cty.UnknownVal(d.resolvedType()).WithMarks(marks), nil
	}

	if !supplied.IsNull() && !supplied.Type().IsObjectType() && !supplied.Type().IsMapType() {
		return cty.UnknownVal(d.resolvedType()).WithMarks(marks), hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid values",
			Detail: fmt.Sprintf(
				"The values of the %s at %s must be an object, but the given value has type %s.",
				c.Kind,
				c.Location,
				supplied.Type().FriendlyName(),
			),
			Subject: s.rng.Ptr(),
		}}
	}

	var given map[string]cty.Value
	if !supplied.IsNull() {
		given = supplied.AsValueMap()
	}

	diags := d.undeclared(given, s, c)
	out := make(map[string]cty.Value, len(d.Values))

	var missing []*Declaration

	for i := range d.Values {
		decl := &d.Values[i]
		val, set := given[decl.Name]
		dropped := set && val.IsNull() && !decl.Nullable

		switch {
		case dropped && decl.Default == nil:
			diags = append(diags, decl.nullDiagnostic(s, c))
			out[decl.Name] = decl.placeholder()
		case set && !dropped:
			converted, declDiags := decl.resolve(val, s, c)
			diags = append(diags, declDiags...)
			out[decl.Name] = converted
		case decl.Default != nil:
			out[decl.Name] = *decl.Default
		default:
			missing = append(missing, decl)
			out[decl.Name] = decl.placeholder()
		}
	}

	if len(missing) > 0 {
		diags = append(diags, missingDiagnostic(missing, s, c))
	}

	return cty.ObjectVal(out).WithMarks(marks), diags
}

// undeclared reports each key of given that d does not declare, in sorted order.
func (d *Declarations) undeclared(given map[string]cty.Value, s *Supplied, c Component) hcl.Diagnostics {
	var diags hcl.Diagnostics

	for _, key := range slices.Sorted(maps.Keys(given)) {
		if _, ok := d.Lookup(key); ok {
			continue
		}

		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Undeclared value %q%s", key, c.suffix()),
			Detail:   fmt.Sprintf("The %s at %s does not declare a value named %q.", c.Kind, c.Location, key) + d.Hint(key),
			Subject:  s.pairRange(key).Ptr(),
		})
	}

	return diags
}

// resolvedType is the type of the object [Declarations.Resolve] returns.
func (d *Declarations) resolvedType() cty.Type {
	attrs := make(map[string]cty.Type, len(d.Values))

	for i := range d.Values {
		attrs[d.Values[i].Name] = d.Values[i].Type.WithoutOptionalAttributesDeep()
	}

	return cty.Object(attrs)
}

// resolve returns val, the value s sets for d, converted to the type of d.
func (d *Declaration) resolve(val cty.Value, s *Supplied, c Component) (cty.Value, hcl.Diagnostics) {
	converted, err := d.convert(val)
	if err != nil {
		return d.placeholder(), hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Invalid value %q%s", d.Name, c.suffix()),
			Detail: fmt.Sprintf(
				"The value is not suitable for the type declared on line %d of the %s's config: %s.",
				d.DefRange.Start.Line,
				c.Kind,
				formatConversionError(err),
			),
			Subject: s.valueRange(d.Name).Ptr(),
		}}
	}

	return converted, nil
}

// placeholder is the value of d when it fails to resolve.
func (d *Declaration) placeholder() cty.Value {
	return cty.UnknownVal(d.Type.WithoutOptionalAttributesDeep())
}

// nullDiagnostic reports the null that s sets for d, a required value that is not nullable.
func (d *Declaration) nullDiagnostic(s *Supplied, c Component) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  fmt.Sprintf("Null value %q%s", d.Name, c.suffix()),
		Detail: fmt.Sprintf(
			"The %s at %s requires the value %q, declared on line %d of its config, and does not accept null for it. "+
				"Set a value, or set nullable = true in its declaration.",
			c.Kind,
			c.Location,
			d.Name,
			d.DefRange.Start.Line,
		),
		Subject: s.valueRange(d.Name).Ptr(),
	}
}

// missingDiagnostic reports missing, the required values that s does not set, in declaration order.
func missingDiagnostic(missing []*Declaration, s *Supplied, c Component) *hcl.Diagnostic {
	if len(missing) == 1 {
		return &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Missing required value %q%s", missing[0].Name, c.suffix()),
			Detail: fmt.Sprintf(
				"The %s at %s requires the value %q, declared on line %d of its config, and nothing sets it.",
				c.Kind,
				c.Location,
				missing[0].Name,
				missing[0].DefRange.Start.Line,
			),
			Subject: s.rng.Ptr(),
		}
	}

	names := make([]string, 0, len(missing))
	declared := make([]string, 0, len(missing))

	for _, decl := range missing {
		names = append(names, strconv.Quote(decl.Name))
		declared = append(declared, fmt.Sprintf("%q (declared on line %d)", decl.Name, decl.DefRange.Start.Line))
	}

	last := len(declared) - 1

	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  fmt.Sprintf("Missing required values %s%s", strings.Join(names, ", "), c.suffix()),
		Detail: fmt.Sprintf(
			"The %s at %s requires the values %s and %s, and nothing sets them.",
			c.Kind,
			c.Location,
			strings.Join(declared[:last], ", "),
			declared[last],
		),
		Subject: s.rng.Ptr(),
	}
}

// suffix names the supplying block in a diagnostic summary, or is empty when c has no address.
func (c Component) suffix() string {
	if c.Address == "" {
		return ""
	}

	return fmt.Sprintf(" for %s %q", c.Kind, c.Address)
}

// formatConversionError renders err with the path of a [cty.PathError] before its message, as in
// attribute "a": a number is required, or [0]: a number is required.
func formatConversionError(err error) string {
	var pathErr cty.PathError
	if !errors.As(err, &pathErr) || len(pathErr.Path) == 0 {
		return err.Error()
	}

	return formatPath(pathErr.Path) + ": " + pathErr.Error()
}

// formatPath renders path in HCL traversal syntax, quoted after the word attribute when it starts with an attribute.
func formatPath(path cty.Path) string {
	formatted := tfdiags.FormatCtyPath(path)

	if _, ok := path[0].(cty.GetAttrStep); ok {
		return `attribute "` + strings.TrimPrefix(formatted, ".") + `"`
	}

	return formatted
}
