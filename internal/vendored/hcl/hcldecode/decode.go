// Copyright IBM Corp. 2014, 2025
// SPDX-License-Identifier: MPL-2.0

// Package hcldecode decodes HCL bodies into Go values as gohcl does, from a decode plan built once for each Go type.
//
// gohcl reads the hcl tags of a struct, and of every block struct below it, on each call. Building the plan once per
// type leaves only the walk of the body and the conversion of its values to each call.
package hcldecode

import (
	"fmt"
	"reflect"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/zclconf/go-cty/cty"
)

var (
	exprType  = reflect.TypeFor[hcl.Expression]()
	bodyType  = reflect.TypeFor[hcl.Body]()
	attrType  = reflect.TypeFor[*hcl.Attribute]()
	attrsType = reflect.TypeFor[hcl.Attributes]()
)

// DecodeBody decodes body into val with evalCtx, as [gohcl.DecodeBody] does. Attributes decode in name order, where
// gohcl decodes them in map order.
//
// Panics when val is not a pointer to a struct or a map, or when the hcl tags of a struct it reaches are invalid.
func DecodeBody(body hcl.Body, evalCtx *hcl.EvalContext, val any) hcl.Diagnostics {
	rv := reflect.ValueOf(val)
	if rv.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("target value must be a pointer, not %s", rv.Type()))
	}

	return decodeBodyToValue(body, evalCtx, rv.Elem())
}

// ImpliedBodySchema returns the schema [DecodeBody] decodes the type of val with, which is the schema
// [gohcl.ImpliedBodySchema] returns, and whether the type has a remain field. The schema is shared, and must not be
// changed.
//
// Panics when val is not a struct or a pointer to one, or when its hcl tags are invalid.
func ImpliedBodySchema(val any) (*hcl.BodySchema, bool) {
	ty := reflect.TypeOf(val)
	if ty.Kind() == reflect.Pointer {
		ty = ty.Elem()
	}

	if ty.Kind() != reflect.Struct {
		panic(fmt.Sprintf("given value must be struct, not %T", val))
	}

	p := planOf(ty)

	return p.schema, p.remain != nil
}

// decodeBodyToValue decodes body into val, a struct or a map. A map takes the attributes of body, which gohcl decodes
// with no tags to read.
//
// Panics when val is neither.
func decodeBodyToValue(body hcl.Body, evalCtx *hcl.EvalContext, val reflect.Value) hcl.Diagnostics {
	if val.Kind() == reflect.Struct {
		return decodeBodyToStruct(body, evalCtx, val, planOf(val.Type()))
	}

	if val.Kind() == reflect.Map {
		return gohcl.DecodeBody(body, evalCtx, val.Addr().Interface())
	}

	panic(fmt.Sprintf("target value must be pointer to struct or map, not %s", val.Type()))
}

func decodeBodyToStruct(
	body hcl.Body,
	evalCtx *hcl.EvalContext,
	val reflect.Value,
	p *plan,
) hcl.Diagnostics {
	content, leftovers, diags := bodyContent(body, p)
	if content == nil {
		return diags
	}

	if p.body != nil {
		diags = append(diags, decodeWholeBody(
			body,
			evalCtx,
			val.Field(p.body.index),
			p.body,
		)...)
	}

	if p.remain != nil {
		diags = append(diags, decodeRemain(
			leftovers,
			evalCtx,
			val.Field(p.remain.index),
			p.remain,
		)...)
	}

	for i := range p.attrs {
		diags = append(diags, decodeAttribute(
			body,
			content,
			evalCtx,
			val,
			&p.attrs[i],
		)...)
	}

	blocksByType := content.Blocks.ByType()

	for i := range p.blocks {
		diags = append(diags, decodeBlocks(
			body,
			blocksByType[p.blocks[i].typeName],
			evalCtx,
			val,
			&p.blocks[i],
		)...)
	}

	return diags
}

// bodyContent returns the content of body that the schema of p names, and, when p has a remain field, what the
// schema leaves of body.
func bodyContent(body hcl.Body, p *plan) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	if p.remain != nil {
		return body.PartialContent(p.schema)
	}

	content, diags := body.Content(p.schema)

	return content, nil, diags
}

// decodeWholeBody decodes body into the field of the body tag.
func decodeWholeBody(
	body hcl.Body,
	evalCtx *hcl.EvalContext,
	field reflect.Value,
	f *bodyField,
) hcl.Diagnostics {
	if f.takesBody {
		field.Set(reflect.ValueOf(body))

		return nil
	}

	return decodeBodyToValue(body, evalCtx, field)
}

// decodeRemain decodes leftovers, what the schema left of the body, into the field of the remain tag.
func decodeRemain(
	leftovers hcl.Body,
	evalCtx *hcl.EvalContext,
	field reflect.Value,
	f *bodyField,
) hcl.Diagnostics {
	switch {
	case f.takesBody:
		field.Set(reflect.ValueOf(leftovers))

		return nil
	case f.takesAttributes:
		attrs, diags := leftovers.JustAttributes()
		field.Set(reflect.ValueOf(attrs))

		return diags
	default:
		return decodeBodyToValue(leftovers, evalCtx, field)
	}
}

// decodeAttribute decodes the attribute of content that f names into its field of val. An absent attribute leaves the
// field as it is, unless the field takes an expression, which then evaluates to null.
func decodeAttribute(
	body hcl.Body,
	content *hcl.BodyContent,
	evalCtx *hcl.EvalContext,
	val reflect.Value,
	f *attrField,
) hcl.Diagnostics {
	field := val.Field(f.index)

	attr := content.Attributes[f.name]
	if attr == nil {
		if f.takesExpression {
			null := hcl.StaticExpr(cty.NullVal(cty.DynamicPseudoType), body.MissingItemRange())
			field.Set(reflect.ValueOf(null))
		}

		return nil
	}

	if f.rangeIndex >= 0 {
		val.Field(f.rangeIndex).Set(reflect.ValueOf(attr.Range))
	}

	if f.nameRangeIndex >= 0 {
		val.Field(f.nameRangeIndex).Set(reflect.ValueOf(attr.NameRange))
	}

	if f.valueRangeIndex >= 0 {
		val.Field(f.valueRangeIndex).Set(reflect.ValueOf(attr.Expr.Range()))
	}

	switch {
	case f.takesAttribute:
		field.Set(reflect.ValueOf(attr))
	case f.takesExpression:
		field.Set(reflect.ValueOf(attr.Expr))
	default:
		return gohcl.DecodeExpression(attr.Expr, evalCtx, field.Addr().Interface())
	}

	return nil
}

// decodeBlocks decodes blocks, the blocks of the body of type f.typeName, into their field of val.
func decodeBlocks(
	body hcl.Body,
	blocks hcl.Blocks,
	evalCtx *hcl.EvalContext,
	val reflect.Value,
	f *blockField,
) hcl.Diagnostics {
	field := val.Field(f.index)

	if len(blocks) > 1 && !f.isSlice {
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("Duplicate %s block", f.typeName),
			Detail: fmt.Sprintf(
				"Only one %s block is allowed. Another was defined at %s.",
				f.typeName,
				blocks[0].DefRange.String(),
			),
			Subject: &blocks[1].DefRange,
		}}
	}

	if len(blocks) == 0 {
		if !f.isSlice && !f.isPtr {
			return hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Missing %s block", f.typeName),
				Detail:   fmt.Sprintf("A %s block is required.", f.typeName),
				Subject:  body.MissingItemRange().Ptr(),
			}}
		}

		if field.IsNil() {
			field.Set(reflect.Zero(field.Type()))
		}

		return nil
	}

	if f.isSlice {
		return decodeBlockSlice(blocks, evalCtx, field, f)
	}

	if !f.isPtr {
		return decodeBlockToValue(blocks[0], evalCtx, field)
	}

	v := field
	if v.IsNil() {
		v = reflect.New(f.elem)
	}

	diags := decodeBlockToValue(blocks[0], evalCtx, v.Elem())
	field.Set(v)

	return diags
}

// decodeBlockSlice decodes blocks into field, a slice of f.elem or of pointers to it. Elements the slice already holds
// are decoded into again, and the slice is cut to the number of blocks.
func decodeBlockSlice(
	blocks hcl.Blocks,
	evalCtx *hcl.EvalContext,
	field reflect.Value,
	f *blockField,
) hcl.Diagnostics {
	var diags hcl.Diagnostics

	sli := field
	if sli.IsNil() {
		sli = reflect.MakeSlice(field.Type(), len(blocks), len(blocks))
	}

	for i, block := range blocks {
		if !f.isPtr {
			if i >= sli.Len() {
				sli = reflect.Append(sli, reflect.New(f.elem).Elem())
			}

			diags = append(diags, decodeBlockToValue(block, evalCtx, sli.Index(i))...)

			continue
		}

		if i >= sli.Len() {
			sli = reflect.Append(sli, reflect.New(f.elem))
		}

		v := sli.Index(i)
		if v.IsNil() {
			v = reflect.New(f.elem)
		}

		diags = append(diags, decodeBlockToValue(block, evalCtx, v.Elem())...)
		sli.Index(i).Set(v)
	}

	if sli.Len() > len(blocks) {
		sli.SetLen(len(blocks))
	}

	field.Set(sli)

	return diags
}

// decodeBlockToValue decodes the body of block into val, and sets the fields of val that take its labels and ranges.
func decodeBlockToValue(
	block *hcl.Block,
	evalCtx *hcl.EvalContext,
	val reflect.Value,
) hcl.Diagnostics {
	p := planOf(val.Type())
	diags := decodeBodyToStruct(block.Body, evalCtx, val, p)

	for i, label := range block.Labels {
		lf := p.labels[i]
		val.Field(lf.index).Set(reflect.ValueOf(label))

		if lf.rangeIndex >= 0 {
			val.Field(lf.rangeIndex).Set(reflect.ValueOf(block.LabelRanges[i]))
		}
	}

	if p.typeRange >= 0 {
		val.Field(p.typeRange).Set(reflect.ValueOf(block.TypeRange))
	}

	if p.defRange >= 0 {
		val.Field(p.defRange).Set(reflect.ValueOf(block.DefRange))
	}

	return diags
}
