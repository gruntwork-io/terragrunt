// Copyright IBM Corp. 2014, 2025
// SPDX-License-Identifier: MPL-2.0

package hcldecode

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
)

// plan is how [DecodeBody] decodes into one struct type.
type plan struct {
	// schema is the schema of the body, as [gohcl.ImpliedBodySchema] builds it.
	schema *hcl.BodySchema
	// remain is the field that takes what schema leaves of the body, nil when there is none.
	remain *bodyField
	// body is the field that takes the whole body, nil when there is none.
	body *bodyField
	// attrs are the attribute fields, by name.
	attrs []attrField
	// blocks are the block fields, by block type.
	blocks []blockField
	// labels are the label fields, in field order.
	labels []labelField
	// defRange is the index of the field that takes the block's definition range, -1 when there is none.
	defRange int
	// typeRange is the index of the field that takes the range of the block's type, -1 when there is none.
	typeRange int
}

// bodyField is a field that takes a body, or what a schema leaves of one.
type bodyField struct {
	// index is the index of the field.
	index int
	// takesBody reports whether an [hcl.Body] can be assigned to the field.
	takesBody bool
	// takesAttributes reports whether [hcl.Attributes] can be assigned to the field.
	takesAttributes bool
}

// attrField is a field that takes an attribute.
type attrField struct {
	// name is the name of the attribute.
	name string
	// index is the index of the field.
	index int
	// rangeIndex is the index of the field that takes the range of the attribute, -1 when there is none.
	rangeIndex int
	// nameRangeIndex is the index of the field that takes the range of the attribute's name, -1 when there is none.
	nameRangeIndex int
	// valueRangeIndex is the index of the field that takes the range of the attribute's value, -1 when there is none.
	valueRangeIndex int
	// takesAttribute reports whether an *[hcl.Attribute] can be assigned to the field.
	takesAttribute bool
	// takesExpression reports whether an [hcl.Expression] can be assigned to the field.
	takesExpression bool
}

// blockField is a field that takes the blocks of one type.
type blockField struct {
	// elem is the struct type each block decodes into.
	elem reflect.Type
	// typeName is the type of the blocks.
	typeName string
	// index is the index of the field.
	index int
	// isSlice reports whether the field takes any number of blocks.
	isSlice bool
	// isPtr reports whether the field, or each element of it, is a pointer to elem.
	isPtr bool
}

// labelField is a field that takes one label of a block.
type labelField struct {
	// name is the name of the label.
	name string
	// index is the index of the field.
	index int
	// rangeIndex is the index of the field that takes the range of the label, -1 when there is none.
	rangeIndex int
}

var (
	// plansMu guards plans.
	plansMu sync.RWMutex
	// plans holds the plan of each struct type [planOf] was asked for. It holds no more entries than the program has
	// struct types.
	plans = map[reflect.Type]*plan{}
)

// planOf returns the plan of ty, a struct type, building it on the first call for ty.
//
// Panics when the hcl tags of ty are invalid.
func planOf(ty reflect.Type) *plan {
	if p, ok := cachedPlan(ty); ok {
		return p
	}

	built := newPlan(ty)

	plansMu.Lock()
	defer plansMu.Unlock()

	if p, ok := plans[ty]; ok {
		return p
	}

	plans[ty] = built

	return built
}

// cachedPlan returns the plan of ty, and whether [planOf] has built one.
func cachedPlan(ty reflect.Type) (*plan, bool) {
	plansMu.RLock()
	defer plansMu.RUnlock()

	p, ok := plans[ty]

	return p, ok
}

// newPlan builds the plan of ty, a struct type.
func newPlan(ty reflect.Type) *plan {
	tags := tagsOf(ty)

	p := &plan{
		schema:    &hcl.BodySchema{},
		defRange:  tags.defRange,
		typeRange: tags.typeRange,
	}

	if tags.remain >= 0 {
		p.remain = newBodyField(ty, tags.remain)
	}

	if tags.body >= 0 {
		p.body = newBodyField(ty, tags.body)
	}

	for _, name := range sortedKeys(tags.attrs) {
		index := tags.attrs[name]
		field := ty.Field(index)

		p.attrs = append(p.attrs, attrField{
			name:            name,
			index:           index,
			rangeIndex:      indexOr(tags.attrRanges, name),
			nameRangeIndex:  indexOr(tags.attrNameRanges, name),
			valueRangeIndex: indexOr(tags.attrValueRanges, name),
			takesAttribute:  attrType.AssignableTo(field.Type),
			takesExpression: exprType.AssignableTo(field.Type),
		})

		p.schema.Attributes = append(p.schema.Attributes, hcl.AttributeSchema{
			Name:     name,
			Required: isRequired(field.Type, tags.optional[name]),
		})
	}

	for _, typeName := range sortedKeys(tags.blocks) {
		f := newBlockField(ty, typeName, tags.blocks[typeName])
		p.blocks = append(p.blocks, f)

		p.schema.Blocks = append(p.schema.Blocks, hcl.BlockHeaderSchema{
			Type:       typeName,
			LabelNames: labelNames(tagsOf(f.elem).labels),
		})
	}

	for _, label := range tags.labels {
		p.labels = append(p.labels, labelField{
			name:       label.name,
			index:      label.index,
			rangeIndex: indexOr(tags.labelRanges, label.name),
		})
	}

	return p
}

// isRequired reports whether an attribute that decodes into a field of type fieldType is required, as
// [gohcl.ImpliedBodySchema] decides: a field that takes an expression or a pointer, or that is tagged optional, is
// not.
func isRequired(fieldType reflect.Type, optional bool) bool {
	return !fieldType.AssignableTo(exprType) && fieldType.Kind() != reflect.Pointer && !optional
}

func newBodyField(ty reflect.Type, index int) *bodyField {
	fieldType := ty.Field(index).Type

	return &bodyField{
		index:           index,
		takesBody:       bodyType.AssignableTo(fieldType),
		takesAttributes: attrsType.AssignableTo(fieldType),
	}
}

// newBlockField returns the field of ty at index, which takes the blocks of type typeName.
//
// Panics when the field is not a struct, a pointer to one, or a slice of either.
func newBlockField(ty reflect.Type, typeName string, index int) blockField {
	field := ty.Field(index)
	f := blockField{typeName: typeName, index: index, elem: field.Type}

	if f.elem.Kind() == reflect.Slice {
		f.isSlice = true
		f.elem = f.elem.Elem()
	}

	if f.elem.Kind() == reflect.Pointer {
		f.isPtr = true
		f.elem = f.elem.Elem()
	}

	if f.elem.Kind() != reflect.Struct {
		panic(fmt.Sprintf(
			"hcl 'block' tag kind cannot be applied to %s field %s: struct required",
			field.Type,
			field.Name,
		))
	}

	return f
}

func labelNames(labels []taggedLabel) []string {
	if len(labels) == 0 {
		return nil
	}

	names := make([]string, len(labels))
	for i, label := range labels {
		names[i] = label.name
	}

	return names
}

// fieldTags is what the hcl tags of a struct type say, by field index.
type fieldTags struct {
	attrs           map[string]int
	blocks          map[string]int
	optional        map[string]bool
	attrRanges      map[string]int
	attrNameRanges  map[string]int
	attrValueRanges map[string]int
	labelRanges     map[string]int
	labels          []taggedLabel
	remain          int
	body            int
	defRange        int
	typeRange       int
}

// taggedLabel is a field tagged as a label.
type taggedLabel struct {
	name  string
	index int
}

// tagsOf reads the hcl tags of the fields of ty, a struct type. An index that no field has is -1.
//
// Panics when a tag names an unknown kind, or when more than one field has a tag that only one field may have.
func tagsOf(ty reflect.Type) *fieldTags {
	tags := &fieldTags{
		attrs:           map[string]int{},
		blocks:          map[string]int{},
		optional:        map[string]bool{},
		attrRanges:      map[string]int{},
		attrNameRanges:  map[string]int{},
		attrValueRanges: map[string]int{},
		labelRanges:     map[string]int{},
		remain:          -1,
		body:            -1,
		defRange:        -1,
		typeRange:       -1,
	}

	for i := range ty.NumField() {
		field := ty.Field(i)

		tag := field.Tag.Get("hcl")
		if tag == "" {
			continue
		}

		name, kind, found := strings.Cut(tag, ",")
		if !found {
			kind = "attr"
		}

		switch kind {
		case "attr":
			tags.attrs[name] = i
		case "optional":
			tags.attrs[name] = i
			tags.optional[name] = true
		case "block":
			tags.blocks[name] = i
		case "label":
			tags.labels = append(tags.labels, taggedLabel{name: name, index: i})
		case "remain":
			setOnce(&tags.remain, i, kind)
		case "body":
			setOnce(&tags.body, i, kind)
		case "def_range":
			setOnce(&tags.defRange, i, kind)
		case "type_range":
			setOnce(&tags.typeRange, i, kind)
		case "label_range":
			tags.labelRanges[name] = i
		case "attr_range":
			tags.attrRanges[name] = i
		case "attr_name_range":
			tags.attrNameRanges[name] = i
		case "attr_value_range":
			tags.attrValueRanges[name] = i
		default:
			panic(fmt.Sprintf(
				"invalid hcl field tag kind %q on %s %q",
				kind,
				field.Type,
				field.Name,
			))
		}
	}

	return tags
}

// setOnce sets *index to i, the index of a field tagged kind.
//
// Panics when an earlier field already set it.
func setOnce(index *int, i int, kind string) {
	if *index >= 0 {
		panic(fmt.Sprintf("only one '%s' tag is permitted", kind))
	}

	*index = i
}

func indexOr(indexes map[string]int, name string) int {
	if index, ok := indexes[name]; ok {
		return index
	}

	return -1
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}
