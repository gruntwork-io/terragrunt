package valuesblock

import (
	"cmp"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
)

// Supplied is the values a component receives, with the source range of each key.
type Supplied struct {
	items map[string]suppliedItem
	value cty.Value
	rng   hcl.Range
}

// suppliedItem is where one key is set.
type suppliedItem struct {
	pair  hcl.Range
	value hcl.Range
}

// SuppliedByExpr returns value as set by expr, the values attribute of a unit or stack block. When expr is not an
// object constructor, or a key is not a literal, diagnostics about that key point at the whole of expr.
func SuppliedByExpr(value cty.Value, expr hcl.Expression) *Supplied {
	s := &Supplied{
		items: make(map[string]suppliedItem),
		value: value,
		rng:   expr.Range(),
	}

	pairs, diags := hcl.ExprMap(expr)
	if diags.HasErrors() {
		return s
	}

	for _, pair := range pairs {
		name, ok := literalKey(pair.Key)
		if !ok {
			continue
		}

		s.items[name] = suppliedItem{
			pair:  hcl.RangeBetween(pair.Key.Range(), pair.Value.Range()),
			value: pair.Value.Range(),
		}
	}

	return s
}

// SuppliedByAttributes returns value as set by attrs, the attributes of a terragrunt.values.hcl file. Diagnostics
// that concern no single attribute point at whole.
func SuppliedByAttributes(value cty.Value, attrs hcl.Attributes, whole hcl.Range) *Supplied {
	s := &Supplied{
		items: make(map[string]suppliedItem, len(attrs)),
		value: value,
		rng:   whole,
	}

	for name, attr := range attrs {
		s.items[name] = suppliedItem{
			pair:  attr.Range,
			value: attr.Expr.Range(),
		}
	}

	return s
}

// EvaluateAttributes evaluates each of attrs, the attributes of a terragrunt.values.hcl file, in evalCtx and returns
// them as one object. It evaluates them in source order, so its diagnostics follow the file.
func EvaluateAttributes(attrs hcl.Attributes, evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	values := make(map[string]cty.Value, len(attrs))

	var diags hcl.Diagnostics

	for _, attr := range slices.SortedFunc(maps.Values(attrs), bySourcePosition) {
		val, valDiags := attr.Expr.Value(evalCtx)
		diags = append(diags, valDiags...)
		values[attr.Name] = val
	}

	return cty.ObjectVal(values), diags
}

// NothingSupplied returns an empty set of values. Diagnostics about required values point at at.
func NothingSupplied(at hcl.Range) *Supplied {
	return &Supplied{
		value: cty.EmptyObjectVal,
		rng:   at,
	}
}

// KeyRange returns the range of the literal key name in the object expression expr. It is the range of the whole
// expression when expr is computed, such as a merge call, and has no such key.
func KeyRange(expr hcl.Expression, name string) hcl.Range {
	pairs, diags := hcl.ExprMap(expr)
	if diags.HasErrors() {
		return expr.Range()
	}

	for _, pair := range pairs {
		if key, ok := literalKey(pair.Key); ok && key == name {
			return pair.Key.Range()
		}
	}

	return expr.Range()
}

// pairRange returns the range of the key and value that set name, or the range of the whole supply when no literal
// key sets it.
func (s *Supplied) pairRange(name string) hcl.Range {
	if item, ok := s.items[name]; ok {
		return item.pair
	}

	return s.rng
}

// valueRange returns the range of the value set for name, or the range of the whole supply when no literal key sets
// it.
func (s *Supplied) valueRange(name string) hcl.Range {
	if item, ok := s.items[name]; ok {
		return item.value
	}

	return s.rng
}

// literalKey returns the string a constant object key evaluates to, and false when the key is computed.
func literalKey(expr hcl.Expression) (string, bool) {
	key, diags := expr.Value(nil)
	if diags.HasErrors() || !key.Type().Equals(cty.String) || key.IsNull() || !key.IsKnown() {
		return "", false
	}

	return key.AsString(), true
}

// bySourcePosition orders attributes by where they start in their file.
func bySourcePosition(a, b *hcl.Attribute) int {
	return cmp.Compare(a.Range.Start.Byte, b.Range.Start.Byte)
}
