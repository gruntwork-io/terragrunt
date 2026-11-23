package ctyhelper

import (
	"bytes"
	"encoding/json/jsontext"
	"fmt"
	"unicode/utf8"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	"golang.org/x/text/unicode/norm"
)

// JSONKeyCollisionError is returned for a JSON object with two distinct keys that are the same string after the
// Unicode normalization cty applies to every string, which cty cannot represent.
type JSONKeyCollisionError struct {
	Key      string
	OtherKey string
}

// Error returns the message, with both keys quoted as ASCII so the difference between them shows.
func (e JSONKeyCollisionError) Error() string {
	return fmt.Sprintf(
		"JSON object keys %+q and %+q are the same after Unicode normalization",
		e.Key,
		e.OtherKey,
	)
}

// CheckJSONKeys returns a [JSONKeyCollisionError] when an object in buf has two distinct keys that normalize to the
// same string. cty panics on such keys, so buf must pass this check before it reaches [ctyjson.ImpliedType],
// [ctyjson.UnmarshalType], or [ctyjson.Unmarshal] with a type that holds a map. CheckJSONKeys leaves every other
// problem with buf, including malformed JSON, for ctyjson to report.
func CheckJSONKeys(buf []byte) error {
	if !mayCollide(buf) {
		return nil
	}

	if collision, found := firstKeyCollision(buf); found {
		return collision
	}

	return nil
}

// ImpliedJSONType is [ctyjson.ImpliedType], returning a [JSONKeyCollisionError] for keys cty cannot represent.
func ImpliedJSONType(buf []byte) (cty.Type, error) {
	if err := CheckJSONKeys(buf); err != nil {
		return cty.NilType, err
	}

	return ctyjson.ImpliedType(buf)
}

// DecodeJSON returns the value buf holds, as [ctyjson.SimpleJSONValue] decodes it, returning a
// [JSONKeyCollisionError] for keys cty cannot represent.
func DecodeJSON(buf []byte) (cty.Value, error) {
	ty, err := ImpliedJSONType(buf)
	if err != nil {
		return cty.NilVal, err
	}

	return ctyjson.Unmarshal(buf, ty)
}

// JSONDecodeFunc returns the jsondecode function of [stdlib.JSONDecodeFunc], returning a [JSONKeyCollisionError]
// for keys cty cannot represent.
func JSONDecodeFunc() function.Function {
	return function.New(&function.Spec{
		Description: stdlib.JSONDecodeFunc.Description(),
		Params:      stdlib.JSONDecodeFunc.Params(),
		Type: func(args []cty.Value) (cty.Type, error) {
			if !args[0].IsKnown() {
				return stdlib.JSONDecodeFunc.ReturnTypeForValues(args)
			}

			return ImpliedJSONType([]byte(args[0].AsString()))
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			return ctyjson.Unmarshal([]byte(args[0].AsString()), retType)
		},
	})
}

// mayCollide reports whether buf can hold two distinct keys that normalize to the same string. At least one key of
// such a pair is not in normal form, which buf rules out unless it holds text not in normal form, a \u escape of a
// code point outside ASCII, or a \u escape followed by a byte outside ASCII, which can combine with the escaped
// character.
func mayCollide(buf []byte) bool {
	nonASCII := false

	for i := 0; i < len(buf); i++ {
		switch {
		case buf[i] >= utf8.RuneSelf:
			nonASCII = true

			continue
		case buf[i] != '\\' || i+1 == len(buf):
			continue
		case buf[i+1] == 'u' && !isASCIIEscape(buf[i+2:]):
			return true
		}

		i++
	}

	return nonASCII && !norm.NFC.IsNormal(buf)
}

// isASCIIEscape reports whether rest, the text after a \u, starts with the hex digits of a code point in ASCII and
// does not go on with a byte outside ASCII.
func isASCIIEscape(rest []byte) bool {
	if len(rest) < 4 || rest[0] != '0' || rest[1] != '0' || rest[2] < '0' || rest[2] > '7' {
		return false
	}

	return len(rest) == 4 || rest[4] < utf8.RuneSelf
}

// objectKeys holds the keys read so far from one JSON object, by their normalized form.
type objectKeys map[string]string

// firstKeyCollision returns the first pair of distinct keys in an object of buf that normalize to the same string,
// and whether it found one. It finds none past what jsontext reads of buf: up to malformed JSON, which ctyjson reports
// in its own words, or nesting deeper than jsontext allows.
//
// The walk is iterative, so the Go stack does not grow with the nesting of buf.
func firstKeyCollision(buf []byte) (JSONKeyCollisionError, bool) {
	dec := jsontext.NewDecoder(
		bytes.NewReader(buf),
		jsontext.AllowDuplicateNames(true),
		jsontext.AllowInvalidUTF8(true),
	)

	var objects []objectKeys

	// Every token takes at least one byte of buf, so a read past the last token errors within len(buf)+1 reads.
	for range len(buf) + 1 {
		kind, length := dec.StackIndex(dec.StackDepth())
		isName := kind == jsontext.KindBeginObject && length%2 == 0

		tok, err := dec.ReadToken()
		if err != nil {
			return JSONKeyCollisionError{}, false
		}

		if tok.Kind() == jsontext.KindBeginObject {
			objects = append(objects, objectKeys{})

			continue
		}

		if tok.Kind() == jsontext.KindEndObject {
			objects = objects[:len(objects)-1]

			continue
		}

		if !isName {
			continue
		}

		if collision, found := objects[len(objects)-1].add(tok.String()); found {
			return collision, true
		}
	}

	panic("jsontext read more tokens than buf has bytes")
}

// add records key, returning the collision with a distinct key read before it that normalizes to the same string, and
// whether there is one.
func (keys objectKeys) add(key string) (JSONKeyCollisionError, bool) {
	normalized := cty.NormalizeString(key)

	other, found := keys[normalized]
	if found && other != key {
		return JSONKeyCollisionError{Key: other, OtherKey: key}, true
	}

	keys[normalized] = key

	return JSONKeyCollisionError{}, false
}
