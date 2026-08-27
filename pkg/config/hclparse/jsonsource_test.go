package hclparse_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// jsonBlock is what the JSON source tests decode into. Untyped attributes let a fuzzed string
// land anywhere in a body without the decode rejecting it first.
type jsonBlock struct {
	Expansion *hclparse.ExpansionBlock `hcl:"expansion,block"`
	Value     *cty.Value               `hcl:"value,attr"`
	Path      *cty.Value               `hcl:"path,attr"`
	Name      string                   `hcl:",label"`
}

// jsonSource renders the single dependency block src declares as the HCL that means the same
// thing, the way the render command quotes it.
func jsonSource(tb testing.TB, src string) (*hclparse.SourceBlock, *jsonBlock, error) {
	tb.Helper()

	file, err := hclparse.NewParser().ParseFromString(src, "terragrunt.hcl.json")
	if err != nil {
		return nil, nil, err
	}

	instances, err := file.ExpandBlocks("dependency", new(jsonBlock), nil)
	if err != nil {
		return nil, nil, err
	}

	require.Len(tb, instances, 1)
	require.NotNil(tb, instances[0].Source, "src must declare an expansion block to get a source")

	return instances[0].Source, instances[0].Value.(*jsonBlock), nil
}

// TestJSONBlockSourceDropsCommentProperty pins that the "//" property hcl reads as a comment on
// a body stays out of the rendered block. Writing it back as an argument spelled "//" leaves a
// line HCL reads as a comment, so nothing downstream reports it.
func TestJSONBlockSourceDropsCommentProperty(t *testing.T) {
	t.Parallel()

	source, _, err := jsonSource(t, `{"dependency": {"vpc": {
  "//": "the network",
  "expansion": {"count": 1},
  "path": "../vpc",
  "value": {"//": "kept, since this one is not a body"}
}}}`)
	require.NoError(t, err)

	text, err := source.Body()
	require.NoError(t, err)

	assert.NotContains(t, text, "the network")
	assert.Contains(t, text, `"//" = "kept, since this one is not a body"`)
}

// FuzzJSONBlockSourceRoundTrips pins that a JSON block renders as HCL that reads back as the
// same block. The two syntaxes disagree on which characters an escape may spell and on which
// names a key may be written bare, so this walks the strings a table cannot enumerate.
func FuzzJSONBlockSourceRoundTrips(f *testing.F) {
	seeds := []string{
		"",
		"plain",
		`a"b`,
		`a\b`,
		`a\/b`,
		"a\tb",
		"a\nb",
		"a\x01b",
		"ünïcode",
		"emoji \U0001F600",
		"${count.index}",
		"$${count.index}",
		"%{ if true }a%{ endif }",
		`${lower("A\"B")}`,
		"//",
		"not an identifier",
		"${",
		"}",
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) {
			t.Skip("JSON cannot carry a string that is not UTF-8, so this never reaches a config")
		}

		quoted, err := json.Marshal(text)
		require.NoError(t, err)

		// Only an expanded block is quoted back, so the block declares an expansion it does not
		// otherwise use.
		src := `{"dependency": {` + string(quoted) + `: {` +
			`"expansion": {"count": 1},` +
			`"path": ` + string(quoted) + `,` +
			`"value": {` + string(quoted) + `: [` + string(quoted) + `]}}}}`

		source, block, err := jsonSource(t, src)
		if err != nil {
			// A ${ that opens no expression is not a template, and hcl rejects the config
			// before there is anything to render.
			return
		}

		assert.Equal(t, text, block.Name, "the label survives the render")

		if strings.Contains(text, "${") || strings.Contains(text, "%{") {
			// Formatting normalizes the spacing inside a template, so what a template renders
			// to is not the text it was written as. TestBodyAsCtyKeepsEscapedTemplateMarkers
			// and TestBodyAsCtyKeepsDirectives pin those.
			return
		}

		// BodyAsCty reads the rendered HCL the way `render --format json` serializes it, so this
		// compares the block against the config it was rendered from.
		body, err := source.BodyAsCty()
		require.NoError(t, err)

		wantValue := cty.ObjectVal(map[string]cty.Value{
			text: cty.TupleVal([]cty.Value{cty.StringVal(text)}),
		})

		assert.True(t, cty.StringVal(text).RawEquals(body.GetAttr("path")),
			"path: want %#v, got %#v", text, body.GetAttr("path"))
		assert.True(t, wantValue.RawEquals(body.GetAttr("value")),
			"value: want %#v, got %#v", wantValue, body.GetAttr("value"))
	})
}

// remainBlock mirrors the unit and stack blocks, which accept properties they do not declare.
type remainBlock struct {
	Remain    hcl.Body                 `hcl:",remain"`
	Expansion *hclparse.ExpansionBlock `hcl:"expansion,block"`
	Path      *cty.Value               `hcl:"path,attr"`
	Name      string                   `hcl:",label"`
}

// TestExpandBlocksTranscodesOnlyExpandedJSONBlocks pins that a block declaring no expansion is
// never transcoded. Only an expanded block is quoted back, and every command parses these blocks,
// so a property no HCL argument can spell must not fail a config that needs no quoted text.
func TestExpandBlocksTranscodesOnlyExpandedJSONBlocks(t *testing.T) {
	t.Parallel()

	const unrenderable = `"path": "../vpc", "not an identifier": 1`

	file, err := hclparse.NewParser().ParseFromString(
		`{"dependency": {"vpc": {`+unrenderable+`}}}`,
		"terragrunt.hcl.json",
	)
	require.NoError(t, err)

	instances, err := file.ExpandBlocks("dependency", new(remainBlock), nil)
	require.NoError(t, err)
	require.Len(t, instances, 1)
	assert.Nil(t, instances[0].Source)

	// The same body under an expansion block still cannot be quoted back, since an HCL argument
	// name cannot be written in quotes. Parsing succeeds anyway: the failure rides along on the
	// block and reaches whoever asks for the text.
	expanded, err := hclparse.NewParser().ParseFromString(
		`{"dependency": {"vpc": {"expansion": {"count": 1}, `+unrenderable+`}}}`,
		"terragrunt.hcl.json",
	)
	require.NoError(t, err)

	withExpansion, err := expanded.ExpandBlocks("dependency", new(remainBlock), nil)
	require.NoError(t, err, "a block Terragrunt cannot quote must not fail the parse")
	require.Len(t, withExpansion, 1)

	source := withExpansion[0].Source
	require.NotNil(t, source)

	text, err := source.Body()
	assert.Empty(t, text)

	var typed hclparse.JSONBlockSourceError
	require.ErrorAs(t, err, &typed)

	_, err = source.BodyAsCty()
	require.ErrorAs(t, err, &typed)
}
