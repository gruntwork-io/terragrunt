package ctyhelper_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

const (
	precomposedE         = "\u00e9"
	decomposedE          = "e\u0301"
	kelvinSign           = "\u212a"
	combiningAcute       = "\u0301"
	combiningLongSolidus = "\u0338"
	notLessThan          = "\u226e"

	// Each escaped constant holds JSON escapes of the characters its name gives.
	escapedPrecomposedE = `\u00e9`
	escapedDecomposedE  = `e\u0301`
	escapedE            = `\u0065`
	escapedLessThan     = `\u003c`
)

// decodeJSONMatchesCtyJSON lists JSON that ctyjson decodes without panicking, valid or not.
var decodeJSONMatchesCtyJSON = []struct {
	name string
	json string
}{
	{name: "ascii object", json: `{"a":1,"b":[true,null,"x"],"c":{"d":"e"}}`},
	{name: "escapes of ascii characters", json: `{"\u0061":"\u003c\u0026\u003e","b":"\\u00e9"}`},
	{name: "precomposed key", json: `{"` + precomposedE + `":1}`},
	{name: "decomposed key", json: `{"` + decomposedE + `":1}`},
	{name: "escaped decomposed key", json: `{"` + escapedDecomposedE + `":1}`},
	{
		name: "keys that stay distinct after normalization",
		json: `{"` + precomposedE + `":1,"e":2,"` + kelvinSign + `x":3}`,
	},
	{name: "same non-ascii key in sibling objects", json: `[{"` + precomposedE + `":1},{"` + decomposedE + `":2}]`},
	{name: "non-ascii values", json: `{"a":"` + precomposedE + `","b":"` + decomposedE + `"}`},
	{name: "repeated non-ascii key", json: `{"` + precomposedE + `":1,"` + precomposedE + `":2}`},
	{name: "repeated key of different types", json: `{"` + precomposedE + `":1,"` + precomposedE + `":"x"}`},
	{name: "malformed non-ascii", json: `{"` + precomposedE + `":}`},
	{name: "malformed ascii", json: `{"a":`},
	{name: "empty", json: ``},
	{name: "escape before a combining character", json: `{"` + escapedE + combiningAcute + `":1,"e":2}`},
	{name: "non-ascii in normal form", json: `{"` + precomposedE + `":"` + notLessThan + `","` + kelvinSign + `":3}`},
}

func TestDecodeJSONMatchesCtyJSON(t *testing.T) {
	t.Parallel()

	for _, tt := range decodeJSONMatchesCtyJSON {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var want ctyjson.SimpleJSONValue

			wantErr := want.UnmarshalJSON([]byte(tt.json))

			got, err := ctyhelper.DecodeJSON([]byte(tt.json))
			assert.Equal(t, wantErr, err)

			if wantErr == nil {
				assert.True(t, want.Value.RawEquals(got), "got %#v, want %#v", got, want.Value)
			}
		})
	}
}

func TestDecodeJSONRejectsKeysThatNormalizeToTheSameString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
		want ctyhelper.JSONKeyCollisionError
	}{
		{
			name: "raw keys",
			json: `{"` + precomposedE + `":1,"` + decomposedE + `":2}`,
			want: ctyhelper.JSONKeyCollisionError{Key: precomposedE, OtherKey: decomposedE},
		},
		{
			name: "escaped keys",
			json: `{"` + escapedPrecomposedE + `":1,"` + escapedDecomposedE + `":2}`,
			want: ctyhelper.JSONKeyCollisionError{Key: precomposedE, OtherKey: decomposedE},
		},
		{
			name: "non-ascii key that normalizes to ascii",
			json: `{"K":1,"` + kelvinSign + `":2}`,
			want: ctyhelper.JSONKeyCollisionError{Key: "K", OtherKey: kelvinSign},
		},
		{
			name: "nested object",
			json: `{"a":[1,{"b":{"` + decomposedE + `":1,"x":[],"` + precomposedE + `":2}}]}`,
			want: ctyhelper.JSONKeyCollisionError{Key: decomposedE, OtherKey: precomposedE},
		},
		{
			name: "escape that combines with the character after it",
			json: `{"` + escapedE + combiningAcute + `":1,"` + precomposedE + `":2}`,
			want: ctyhelper.JSONKeyCollisionError{Key: decomposedE, OtherKey: precomposedE},
		},
		{
			name: "escaped symbol that combines with the character after it",
			json: `{"` + notLessThan + `":1,"` + escapedLessThan + combiningLongSolidus + `":2}`,
			want: ctyhelper.JSONKeyCollisionError{Key: notLessThan, OtherKey: "<" + combiningLongSolidus},
		},
		{
			name: "colliding keys after a repeated key",
			json: `{"` + precomposedE + `":1,"` + precomposedE + `":1,"` + decomposedE + `":1}`,
			want: ctyhelper.JSONKeyCollisionError{Key: precomposedE, OtherKey: decomposedE},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := ctyhelper.DecodeJSON([]byte(tt.json))

			var collision ctyhelper.JSONKeyCollisionError

			require.ErrorAs(t, err, &collision)
			assert.Equal(t, tt.want, collision)
			assert.NotContains(t, collision.Error(), "\n")
		})
	}
}

func TestJSONDecodeFuncRejectsKeysThatNormalizeToTheSameString(t *testing.T) {
	t.Parallel()

	_, err := ctyhelper.JSONDecodeFunc().Call([]cty.Value{
		cty.StringVal(`{"` + escapedPrecomposedE + `":1,"` + escapedDecomposedE + `":2}`),
	})

	var panicErr function.PanicError

	require.ErrorAs(t, err, new(ctyhelper.JSONKeyCollisionError))
	assert.NotErrorAs(t, err, &panicErr)
}

func TestJSONDecodeFuncMatchesStdlib(t *testing.T) {
	t.Parallel()

	tests := []struct {
		arg  cty.Value
		name string
	}{
		{name: "object", arg: cty.StringVal(`{"` + precomposedE + `":[1,"a",null],"b":{"c":true}}`)},
		{name: "marked", arg: cty.StringVal(`{"a":1}`).Mark("sensitive")},
		{name: "unknown", arg: cty.UnknownVal(cty.String)},
		{name: "unknown with a string prefix", arg: cty.UnknownVal(cty.String).Refine().StringPrefix(`"a`).NewValue()},
		{name: "unknown with an invalid prefix", arg: cty.UnknownVal(cty.String).Refine().StringPrefix(`x`).NewValue()},
		{name: "malformed", arg: cty.StringVal(`{"a":`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			want, wantErr := stdlib.JSONDecodeFunc.Call([]cty.Value{tt.arg})
			got, err := ctyhelper.JSONDecodeFunc().Call([]cty.Value{tt.arg})

			assert.Equal(t, wantErr, err)

			if wantErr == nil {
				assert.True(t, want.RawEquals(got), "got %#v, want %#v", got, want)
			}
		})
	}
}

func FuzzDecodeJSON(f *testing.F) {
	for _, tt := range decodeJSONMatchesCtyJSON {
		f.Add(tt.json)
	}

	f.Add(`{"` + precomposedE + `":1,"` + decomposedE + `":2}`)
	f.Add(`{"K":{"\u212a":[{"` + escapedPrecomposedE + `":1,"` + escapedDecomposedE + `":2}]}}`)
	f.Add(`{"` + escapedLessThan + combiningLongSolidus + `":1,"` + notLessThan + `":2}`)
	f.Add(`[` + strings.Repeat(`{"a":`, 20) + `1` + strings.Repeat(`}`, 20) + `]`)

	f.Fuzz(func(t *testing.T, s string) {
		if longExponent.MatchString(s) {
			t.Skip("ctyjson takes minutes to parse a number with a long exponent")
		}

		want, wantErr, panicked := decodeWithCtyJSON([]byte(s))

		got, err := ctyhelper.DecodeJSON([]byte(s))

		switch {
		case panicked:
			require.ErrorAs(t, err, new(ctyhelper.JSONKeyCollisionError))
		case wantErr != nil:
			require.Error(t, err)
		default:
			require.NoError(t, err)
			assert.True(t, want.RawEquals(got), "got %#v, want %#v", got, want)
		}
	})
}

// longExponent matches a number exponent long enough to stall ctyjson.
var longExponent = regexp.MustCompile(`[eE][+-]?[0-9]{4,}`)

// decodeWithCtyJSON decodes buf as [ctyjson.SimpleJSONValue] does, reporting whether ctyjson panicked.
func decodeWithCtyJSON(buf []byte) (val cty.Value, err error, panicked bool) {
	defer func() {
		if recover() != nil {
			panicked = true
		}
	}()

	var v ctyjson.SimpleJSONValue

	err = v.UnmarshalJSON(buf)

	return v.Value, err, false
}
