package hclparse

import (
	"reflect"
	"strconv"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

const (
	// ExpansionBlockName is the block that declares count/for_each iteration inside a
	// dependency, unit, or stack block.
	ExpansionBlockName = "expansion"

	forEachAttrName = "for_each"
	countAttrName   = "count"

	eachVarName  = "each"
	countVarName = "count"

	eachKeyAttrName    = "key"
	eachValueAttrName  = "value"
	countIndexAttrName = "index"
)

// Instance is one decoded product of expanding a block. A block with no expansion
// block yields a single Instance with both keys nil.
type Instance struct {
	Value      any
	EachKey    *string
	CountIndex *int
}

// Key returns the address segment identifying this instance: the each.key for
// for_each, the stringified index for count, and the empty string when the block
// was not expanded.
//
// Addresses are built from this in more than one place (the dependency cty map,
// stack output), so keeping the stringification here is what stops those surfaces
// from drifting apart.
func (inst Instance) Key() string {
	switch {
	case inst.EachKey != nil:
		return *inst.EachKey
	case inst.CountIndex != nil:
		return strconv.Itoa(*inst.CountIndex)
	default:
		return ""
	}
}

// ExpandBlock decodes block once per iteration element, returning one Instance per
// element. out is a prototype pointer supplying the type to decode into; a fresh
// value of that type backs every returned Instance.
//
// A block with no expansion block decodes to exactly one Instance, so callers can
// route expanded and unexpanded blocks through the same path.
//
// Expansion is driven by the presence of the statically-known expansion sub-block,
// read before the surrounding body is evaluated. That ordering is what lets the
// body reference each.value at all: the references are still unevaluated here, and
// only resolve in the per-element decode below.
func ExpandBlock(block *hcl.Block, out any, ctx *hcl.EvalContext) ([]Instance, error) {
	outType := reflect.TypeOf(out)
	if outType == nil || outType.Kind() != reflect.Pointer {
		panic("hclparse.ExpandBlock: out must be a non-nil pointer")
	}

	expansion, err := expansionBlock(block)
	if err != nil {
		return nil, err
	}

	if expansion == nil {
		instance, err := decodeInstance(block, outType, ctx)
		if err != nil {
			return nil, err
		}

		return []Instance{{Value: instance}}, nil
	}

	forEach, count, err := expansionMetaArg(expansion)
	if err != nil {
		return nil, err
	}

	if count != nil {
		return expandCount(block, outType, ctx, count)
	}

	return expandForEach(block, outType, ctx, forEach)
}

// expansionBlock returns the block's expansion sub-block, or nil when it declares none.
func expansionBlock(block *hcl.Block) (*hcl.Block, error) {
	content, _, diags := block.Body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: ExpansionBlockName}},
	})
	if diags.HasErrors() {
		return nil, diags
	}

	if len(content.Blocks) == 0 {
		return nil, nil
	}

	if len(content.Blocks) > 1 {
		return nil, DuplicateExpansionBlockError{
			BlockType: block.Type,
			Subject:   &content.Blocks[1].DefRange,
		}
	}

	return content.Blocks[0], nil
}

// expansionMetaArg returns whichever of for_each/count the expansion block sets.
// Exactly one is non-nil on success.
func expansionMetaArg(expansion *hcl.Block) (forEach, count *hcl.Attribute, err error) {
	// Content rather than PartialContent so a typo inside the expansion block is a
	// loud "unsupported argument" rather than a silently ignored attribute.
	content, diags := expansion.Body.Content(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{
			{Name: forEachAttrName},
			{Name: countAttrName},
		},
	})
	if diags.HasErrors() {
		return nil, nil, diags
	}

	forEach = content.Attributes[forEachAttrName]
	count = content.Attributes[countAttrName]

	switch {
	case forEach != nil && count != nil:
		return nil, nil, ConflictingMetaArgsError{Subject: &expansion.DefRange}
	case forEach == nil && count == nil:
		return nil, nil, MissingMetaArgError{Subject: &expansion.DefRange}
	}

	return forEach, count, nil
}

func expandCount(
	block *hcl.Block,
	outType reflect.Type,
	ctx *hcl.EvalContext,
	count *hcl.Attribute,
) ([]Instance, error) {
	value, diags := count.Expr.Value(ctx)
	if diags.HasErrors() {
		return nil, diags
	}

	var total int
	if err := gocty.FromCtyValue(value, &total); err != nil {
		return nil, InvalidCountError{Subject: &count.Range, Err: err}
	}

	if total < 0 {
		return nil, NegativeCountError{Count: total, Subject: &count.Range}
	}

	instances := make([]Instance, 0, total)

	for index := range total {
		child := ctx.NewChild()
		child.Variables = map[string]cty.Value{
			countVarName: cty.ObjectVal(map[string]cty.Value{
				countIndexAttrName: cty.NumberIntVal(int64(index)),
			}),
		}

		value, err := decodeInstance(block, outType, child)
		if err != nil {
			return nil, err
		}

		instances = append(instances, Instance{Value: value, CountIndex: new(index)})
	}

	return instances, nil
}

func expandForEach(
	block *hcl.Block,
	outType reflect.Type,
	ctx *hcl.EvalContext,
	forEach *hcl.Attribute,
) ([]Instance, error) {
	collection, diags := forEach.Expr.Value(ctx)
	if diags.HasErrors() {
		return nil, diags
	}

	collectionType := collection.Type()
	if !collectionType.IsSetType() && !collectionType.IsMapType() &&
		!collectionType.IsObjectType() {
		return nil, UnsupportedForEachTypeError{
			Type:    collectionType.FriendlyName(),
			Subject: &forEach.Range,
		}
	}

	instances := make([]Instance, 0, collection.LengthInt())

	for it := collection.ElementIterator(); it.Next(); {
		elementKey, elementValue := it.Element()

		key, err := expansionKey(elementKey, &forEach.Range)
		if err != nil {
			return nil, err
		}

		child := ctx.NewChild()
		child.Variables = map[string]cty.Value{
			eachVarName: cty.ObjectVal(map[string]cty.Value{
				eachKeyAttrName:   cty.StringVal(key),
				eachValueAttrName: elementValue,
			}),
		}

		value, err := decodeInstance(block, outType, child)
		if err != nil {
			return nil, err
		}

		instances = append(instances, Instance{Value: value, EachKey: new(key)})
	}

	return instances, nil
}

// expansionKey renders a for_each element key as the string used in addresses.
func expansionKey(key cty.Value, subject *hcl.Range) (string, error) {
	switch key.Type() {
	case cty.String:
		return key.AsString(), nil
	case cty.Number:
		return key.AsBigFloat().Text('f', -1), nil
	default:
		return "", UnsupportedForEachKeyTypeError{
			Type:    key.Type().FriendlyName(),
			Subject: subject,
		}
	}
}

// decodeInstance decodes the whole block body, expansion sub-block included, into a
// fresh value of outType.
func decodeInstance(block *hcl.Block, outType reflect.Type, ctx *hcl.EvalContext) (any, error) {
	instance := reflect.New(outType.Elem()).Interface()

	if diags := gohcl.DecodeBody(block.Body, ctx, instance); diags.HasErrors() {
		return nil, diags
	}

	return instance, nil
}
