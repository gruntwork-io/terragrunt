package hclparse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

const blockTagKind = "block"

// jsonCommentKey is the property hcl reads as a comment on a body rather than as part of it.
const jsonCommentKey = "//"

// firstPrintable is the lowest rune HCL accepts in a quoted string without an escape.
const firstPrintable = 0x20

// jsonMember is one property of a JSON object, kept in source order so the rendered HCL reads
// in the same order as the config it came from.
type jsonMember struct {
	name  string
	value json.RawMessage
}

// jsonBlockSource renders a block written in JSON as the equivalent HCL, so a caller can quote
// it like any other block.
func jsonBlockSource(src []byte, block *hcl.Block, out any) (*SourceBlock, error) {
	rng := jsonBlockRange(block)

	members, err := jsonBlockMembers(src, block)
	if err != nil {
		return nil, JSONBlockSourceError{Subject: &rng, Err: err}
	}

	var sb strings.Builder

	if err := writeJSONBlock(&sb, block.Type, block.Labels, members, reflect.TypeOf(out).Elem()); err != nil {
		return nil, JSONBlockSourceError{Subject: &rng, Err: err}
	}

	text := hclwrite.Format([]byte(sb.String()))

	// This HCL is quoted into a config that has to parse. Nothing downstream would catch a
	// transcoding bug before it reached a rendered file, so parse it here.
	if _, diags := hclsyntax.ParseConfig(text, block.TypeRange.Filename, hcl.InitialPos); diags.HasErrors() {
		return nil, JSONBlockSourceError{Subject: &rng, Err: diags}
	}

	return &SourceBlock{
		text:  strings.TrimRight(string(text), "\n"),
		Range: rng,
	}, nil
}

// jsonBlockRange returns the range that identifies one block. hcl gives every instance a JSON
// config wrote in an array the range of that array, so a caller grouping blocks by range would
// fold them into one.
func jsonBlockRange(block *hcl.Block) hcl.Range {
	return hcl.RangeBetween(block.DefRange, block.Body.MissingItemRange())
}

// jsonBlockMembers reads the properties of the object holding the block's body.
func jsonBlockMembers(src []byte, block *hcl.Block) ([]jsonMember, error) {
	// DefRange opens the block's value, so this starts at the object, or at the array of them.
	value := src[block.DefRange.Start.Byte:]
	if !bytes.HasPrefix(value, []byte("[")) {
		return decodeJSONObject(value)
	}

	end := block.Body.MissingItemRange().End.Byte - block.DefRange.Start.Byte

	element, err := jsonArrayElementEndingAt(value, int64(end))
	if err != nil {
		return nil, err
	}

	return decodeJSONObject(element)
}

// jsonArrayElementEndingAt returns the element of the array at the start of src that ends offset
// bytes into it.
func jsonArrayElementEndingAt(src []byte, offset int64) (json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(src))

	if err := expectDelim(dec, '['); err != nil {
		return nil, err
	}

	for dec.More() {
		var element json.RawMessage
		if err := dec.Decode(&element); err != nil {
			return nil, err
		}

		if dec.InputOffset() == offset {
			return element, nil
		}
	}

	return nil, UnlocatableJSONBlockError{}
}

// writeJSONBlock writes a block header and its body. Labels are quoted rather than written
// bare, since a JSON block label is whatever string the property name held.
func writeJSONBlock(
	sb *strings.Builder,
	blockType string,
	labels []string,
	members []jsonMember,
	structType reflect.Type,
) error {
	sb.WriteString(blockType)

	for _, label := range labels {
		sb.WriteString(" " + hclStringLiteral(label))
	}

	sb.WriteString(" {\n")

	if err := writeJSONBody(sb, members, structType); err != nil {
		return err
	}

	sb.WriteString("}\n")

	return nil
}

// writeJSONBody writes the properties of one JSON object as the attributes and nested blocks
// of an HCL body.
func writeJSONBody(sb *strings.Builder, members []jsonMember, structType reflect.Type) error {
	blocks := blockFields(structType)
	members = bodyMembers(members)

	for i, member := range members {
		nestedType, isBlock := blocks[member.name]
		if !isBlock {
			value, err := jsonValueHCL(member.value)
			if err != nil {
				return err
			}

			sb.WriteString(member.name + " = " + value + "\n")

			continue
		}

		if labels := labelFields(nestedType); len(labels) > 0 {
			return UnsupportedJSONBlockError{BlockType: member.name}
		}

		bodies, err := nestedJSONBlocks(member.value)
		if err != nil {
			return err
		}

		for _, nested := range bodies {
			if err := writeJSONBlock(sb, member.name, nil, nested, nestedType); err != nil {
				return err
			}
		}

		// An expansion block written in HCL is conventionally set off from the body it
		// iterates, so the quoted block matches. A null property wrote no block to set off.
		if len(bodies) > 0 && i < len(members)-1 {
			sb.WriteString("\n")
		}
	}

	return nil
}

// bodyMembers drops the properties hcl does not read as part of a body, so a rendered block
// holds only what the config set.
func bodyMembers(members []jsonMember) []jsonMember {
	kept := make([]jsonMember, 0, len(members))

	for _, member := range members {
		if member.name == jsonCommentKey {
			continue
		}

		kept = append(kept, member)
	}

	return kept
}

// nestedJSONBlocks reads the bodies of the nested blocks one property holds. JSON writes
// repeated blocks of one type as an array and no block at all as null, so a property naming a
// block type stands for any number of them.
func nestedJSONBlocks(raw json.RawMessage) ([][]jsonMember, error) {
	trimmed := bytes.TrimSpace(raw)

	switch {
	case string(trimmed) == "null":
		return nil, nil
	case bytes.HasPrefix(trimmed, []byte("[")):
		elements, err := decodeJSONArray(trimmed)
		if err != nil {
			return nil, err
		}

		bodies := make([][]jsonMember, 0, len(elements))

		for _, element := range elements {
			members, err := decodeJSONObject(element)
			if err != nil {
				return nil, err
			}

			bodies = append(bodies, members)
		}

		return bodies, nil
	}

	members, err := decodeJSONObject(trimmed)
	if err != nil {
		return nil, err
	}

	return [][]jsonMember{members}, nil
}

// jsonValueHCL renders one JSON value as the HCL expression that means the same thing.
//
// Numbers, booleans and null are spelled the same in both. Objects and arrays are written in a
// syntax HCL also accepts, but they are still rebuilt rather than quoted through. A string
// anywhere inside them has to be re-escaped, since JSON allows escapes such as \/ that HCL
// rejects.
func jsonValueHCL(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", UnsupportedJSONValueError{Value: string(raw)}
	}

	switch trimmed[0] {
	case '{':
		members, err := decodeJSONObject(trimmed)
		if err != nil {
			return "", err
		}

		parts := make([]string, 0, len(members))

		for _, member := range members {
			value, err := jsonValueHCL(member.value)
			if err != nil {
				return "", err
			}

			parts = append(parts, objectKey(member.name)+" = "+value)
		}

		if len(parts) == 0 {
			return "{}", nil
		}

		// One key per line, the way hclwrite renders the same value in the preview below the
		// block. Formatting aligns them once the whole block is written.
		return "{\n" + strings.Join(parts, "\n") + "\n}", nil
	case '[':
		elements, err := decodeJSONArray(trimmed)
		if err != nil {
			return "", err
		}

		parts := make([]string, 0, len(elements))

		for _, element := range elements {
			value, err := jsonValueHCL(element)
			if err != nil {
				return "", err
			}

			parts = append(parts, value)
		}

		return "[" + strings.Join(parts, ", ") + "]", nil
	case '"':
		var text string
		if err := json.Unmarshal(trimmed, &text); err != nil {
			return "", err
		}

		return hclTemplateLiteral(text), nil
	default:
		return string(trimmed), nil
	}
}

// objectKey writes a JSON property name as an HCL object key, quoting only the names that
// cannot be written bare.
func objectKey(name string) string {
	if hclsyntax.ValidIdentifier(name) {
		return name
	}

	return hclStringLiteral(name)
}

// hclTemplateLiteral quotes text as an HCL string, leaving the interpolations and directives it
// contains as written. HCL reads a JSON string value as a template too, so a config_path written
// as "../aurora-${count.index}" means the same thing in both syntaxes. Quoting it back
// unevaluated lets the rendered block stand in for the one the config declared.
//
// Only the text around those sequences is escaped. HCL parses what is inside ${ } as an
// expression, where a quote opens a string of its own and an escaped quote does not parse. JSON
// escapes every quote in the string it holds. The two disagree on exactly the text that carries
// a reference, so escaping follows the template rather than the string.
func hclTemplateLiteral(text string) string {
	var sb strings.Builder

	sb.WriteByte('"')

	for i := 0; i < len(text); {
		switch {
		case strings.HasPrefix(text[i:], `$${`), strings.HasPrefix(text[i:], `%%{`):
			// Already the escaped form, which HCL reads as text.
			sb.WriteString(text[i : i+len(`$${`)])
			i += len(`$${`)
		case strings.HasPrefix(text[i:], `${`), strings.HasPrefix(text[i:], `%{`):
			end := templateSequenceEnd(text, i)
			sb.WriteString(text[i:end])
			i = end
		default:
			char, size := utf8.DecodeRuneInString(text[i:])
			writeEscapedRune(&sb, char)

			i += size
		}
	}

	sb.WriteByte('"')

	return sb.String()
}

// hclStringLiteral quotes text as an HCL string that reads back as exactly that text. Any
// template marker in it is escaped, so HCL reads it rather than evaluating it.
func hclStringLiteral(text string) string {
	var sb strings.Builder

	sb.WriteByte('"')

	for _, char := range escapeTemplateMarkers(text) {
		writeEscapedRune(&sb, char)
	}

	sb.WriteByte('"')

	return sb.String()
}

// escapeTemplateMarkers spells the sequences that open a template so HCL reads them as text
// rather than evaluating them.
func escapeTemplateMarkers(text string) string {
	replaced := strings.ReplaceAll(text, `${`, `$${`)

	return strings.ReplaceAll(replaced, `%{`, `%%{`)
}

func writeEscapedRune(sb *strings.Builder, char rune) {
	switch char {
	case '\\':
		sb.WriteString(`\\`)
	case '"':
		sb.WriteString(`\"`)
	case '\n':
		sb.WriteString(`\n`)
	case '\r':
		sb.WriteString(`\r`)
	case '\t':
		sb.WriteString(`\t`)
	default:
		if char < firstPrintable {
			fmt.Fprintf(sb, `\u%04x`, char)

			return
		}

		sb.WriteRune(char)
	}
}

// templateSequenceEnd returns the index just past the brace closing the template sequence that
// opens at start, or the end of text when nothing closes it.
func templateSequenceEnd(text string, start int) int {
	depth := 0

	for i := start + 1; i < len(text); {
		switch text[i] {
		case '{':
			depth++

			i++
		case '}':
			depth--

			i++

			if depth == 0 {
				return i
			}
		case '"':
			i = quotedEnd(text, i)
		default:
			i++
		}
	}

	return len(text)
}

// quotedEnd returns the index just past the string opening at start, so that a brace written
// inside it does not close the template sequence around it.
func quotedEnd(text string, start int) int {
	for i := start + 1; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}

	return len(text)
}

// decodeJSONObject reads the object at the start of src, keeping its properties in order. It
// ignores trailing content, so it can read one block out of a whole config file.
func decodeJSONObject(src []byte) ([]jsonMember, error) {
	dec := json.NewDecoder(bytes.NewReader(src))

	if err := expectDelim(dec, '{'); err != nil {
		return nil, err
	}

	var members []jsonMember

	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}

		name, ok := token.(string)
		if !ok {
			return nil, UnsupportedJSONValueError{Value: fmt.Sprint(token)}
		}

		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}

		members = append(members, jsonMember{name: name, value: value})
	}

	return members, nil
}

// decodeJSONArray reads the array at the start of src, keeping its elements in order.
func decodeJSONArray(src []byte) ([]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(src))

	if err := expectDelim(dec, '['); err != nil {
		return nil, err
	}

	var elements []json.RawMessage

	for dec.More() {
		var element json.RawMessage
		if err := dec.Decode(&element); err != nil {
			return nil, err
		}

		elements = append(elements, element)
	}

	return elements, nil
}

func expectDelim(dec *json.Decoder, want json.Delim) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}

	if delim, ok := token.(json.Delim); !ok || delim != want {
		return UnsupportedJSONValueError{Value: fmt.Sprint(token)}
	}

	return nil
}

// blockFields returns the `hcl:"name,block"` fields of a block struct, keyed by the name the
// config writes. JSON gives no other way to tell a nested block from an attribute, and gohcl
// reads the same tags, so the two cannot drift.
func blockFields(structType reflect.Type) map[string]reflect.Type {
	fields := map[string]reflect.Type{}

	for field := range structType.Fields() {
		name, kind, found := strings.Cut(field.Tag.Get("hcl"), ",")
		if !found || kind != blockTagKind {
			continue
		}

		fields[name] = elementType(field.Type)
	}

	return fields
}

// elementType unwraps the pointers and slices a block field is declared with, down to the
// struct it decodes into.
func elementType(fieldType reflect.Type) reflect.Type {
	for fieldType.Kind() == reflect.Pointer || fieldType.Kind() == reflect.Slice {
		fieldType = fieldType.Elem()
	}

	return fieldType
}
