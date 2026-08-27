package hclparse_test

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// sourceBlock builds a SourceBlock over text the way [hclparse.File.ExpandBlocks] would.
func sourceBlock(text string) *hclparse.SourceBlock {
	return hclparse.NewSourceBlock(text, hcl.Range{Filename: "terragrunt.hcl"})
}

// assertCty compares by value. A cty number carries big.Float state that a deep compare reads as
// a difference even when the two numbers are the same.
func assertCty(t *testing.T, want, got cty.Value) {
	t.Helper()

	assert.True(t, want.RawEquals(got), "want %#v, got %#v", want, got)
}

// TestBodyAsCtyRejectsLabeledNestedBlock pins a guard nothing reaches today, since no
// dependency, unit or stack block declares a labeled nested block. JSON nests such a block once
// per label, so writing it as one object would claim a config the caller never wrote.
func TestBodyAsCtyRejectsLabeledNestedBlock(t *testing.T) {
	t.Parallel()

	_, err := sourceBlock(`dependency "aurora" {
  generate "backend" {
    path = "backend.tf"
  }
}`).BodyAsCty()

	var typed hclparse.UnquotableBlockError
	require.ErrorAs(t, err, &typed)
}

// TestBodyAsCtyRejectsUnparseableText pins that text which is not one block is refused rather
// than serialized into something a config would read differently.
func TestBodyAsCtyRejectsUnparseableText(t *testing.T) {
	t.Parallel()

	for name, text := range map[string]string{
		"two blocks":  "dependency \"a\" {}\ndependency \"b\" {}",
		"no block":    "config_path = \"../a\"",
		"not hcl":     "{\"dependency\": {}}",
		"empty input": "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := sourceBlock(text).BodyAsCty()
			require.Error(t, err)
		})
	}
}

// TestBodyAsCtyKeepsEmptyCollections pins that an empty object comes back as an empty object
// and an empty list as an empty list, rather than as the template a value would fall back to.
func TestBodyAsCtyKeepsEmptyCollections(t *testing.T) {
	t.Parallel()

	value, err := sourceBlock(`dependency "aurora" {
  expansion {
    count = 1
  }

  config_path  = "../a-${count.index}"
  mock_outputs = {
    nothing = {}
    none    = []
  }
}`).BodyAsCty()
	require.NoError(t, err)

	mocks := value.GetAttr("mock_outputs")
	assertCty(t, cty.EmptyObjectVal, mocks.GetAttr("nothing"))
	assertCty(t, cty.EmptyTupleVal, mocks.GetAttr("none"))

	assertCty(t, cty.NumberIntVal(1), value.GetAttr("expansion").GetAttr("count"))
	assertCty(t, cty.StringVal("../a-${count.index}"), value.GetAttr("config_path"))
}

// TestBodyAsCtyKeepsEscapedTemplateMarkers pins that a marker the config escaped comes back
// escaped, so the rendered configuration still reads as the literal text it was written as.
func TestBodyAsCtyKeepsEscapedTemplateMarkers(t *testing.T) {
	t.Parallel()

	value, err := sourceBlock(`dependency "aurora" {
  expansion {
    count = 1
  }

  config_path  = "../a-${count.index}"
  mock_outputs = {
    literal = "$${not an interpolation}"
    nested  = ["%%{not a directive}"]
  }
}`).BodyAsCty()
	require.NoError(t, err)

	mocks := value.GetAttr("mock_outputs")
	assertCty(t, cty.StringVal("$${not an interpolation}"), mocks.GetAttr("literal"))
	assertCty(t, cty.StringVal("%%{not a directive}"), mocks.GetAttr("nested").Index(cty.NumberIntVal(0)))
}

// TestBodyAsCtyKeepsDirectives pins that a directive comes back as the directive rather than as
// the branch it takes.
func TestBodyAsCtyKeepsDirectives(t *testing.T) {
	t.Parallel()

	value, err := sourceBlock(`dependency "aurora" {
  expansion {
    count = 1
  }

  config_path = "../a-%{ if true }one%{ else }two%{ endif }"
}`).BodyAsCty()
	require.NoError(t, err)

	assertCty(t, cty.StringVal("../a-%{ if true }one%{ else }two%{ endif }"), value.GetAttr("config_path"))
}
