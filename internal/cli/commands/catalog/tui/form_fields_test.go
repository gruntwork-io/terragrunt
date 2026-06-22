package tui_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/pkg/config"
)

// TestFormFieldsFromParsedVariables_BoolStringRawTypes exercises the per-type
// branches of field construction: bool defaults become checkboxes, string
// defaults are JSON-unwrapped into literal-mode initials (falling back to the
// raw value when not JSON), and every other type stays raw HCL.
func TestFormFieldsFromParsedVariables_BoolStringRawTypes(t *testing.T) {
	t.Parallel()

	optional := []*config.ParsedVariable{
		{Name: "enabled", Type: "bool", DefaultValue: "true"},
		{Name: "disabled", Type: "bool", DefaultValue: "false"},
		{Name: "unset_bool", Type: "bool", DefaultValue: ""},
		{Name: "tier", Type: "string", DefaultValue: `"prod"`},
		{Name: "raw_string", Type: "string", DefaultValue: "prod"},
		{Name: "empty_string", Type: "string", DefaultValue: ""},
		{Name: "count", Type: "number", DefaultValue: "5"},
	}

	fields := tui.FieldsFromParsedVariables(nil, optional)
	require.Len(t, fields, len(optional))

	byName := map[string]tui.FormField{}
	for _, f := range fields {
		byName[f.Name] = f
	}

	assert.True(t, byName["enabled"].Checkbox)
	assert.True(t, byName["enabled"].Bool, `default "true" seeds the checkbox on`)
	assert.True(t, byName["enabled"].BoolInitial)

	assert.True(t, byName["disabled"].Checkbox)
	assert.False(t, byName["disabled"].Bool, `default "false" seeds the checkbox off`)

	assert.True(t, byName["unset_bool"].Checkbox)
	assert.False(t, byName["unset_bool"].Bool, "an unparseable bool default falls back to false")

	assert.True(t, byName["tier"].Literal)
	assert.Equal(t, "prod", byName["tier"].Initial, "JSON string default is unwrapped")

	assert.True(t, byName["raw_string"].Literal)
	assert.Equal(t, "prod", byName["raw_string"].Initial, "non-JSON string default passes through")

	assert.True(t, byName["empty_string"].Literal)
	assert.Empty(t, byName["empty_string"].Initial, "an empty string default stays empty")

	assert.False(t, byName["count"].Literal, "number stays raw HCL")
	assert.False(t, byName["count"].Checkbox)
	assert.Equal(t, "5", byName["count"].Initial)
}

// TestFormFieldsFromValuesReferences_StringAndComplexDefaults covers the
// non-bool branches of newValuesField: a known-string optional becomes
// literal-mode, while a complex value (here a tuple) stays raw HCL with the
// default pre-formatted.
func TestFormFieldsFromValuesReferences_StringAndComplexDefaults(t *testing.T) {
	t.Parallel()

	refs := tui.ValuesReferences{
		Optional: []tui.OptionalValue{
			{Name: "region", Default: cty.StringVal("us-east-1")},
			{Name: "zones", Default: cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})},
		},
	}

	fields := tui.FieldsFromValuesReferences(refs)
	require.Len(t, fields, 2)

	assert.Equal(t, "region", fields[0].Name)
	assert.True(t, fields[0].Literal, "string default is literal-mode")
	assert.Equal(t, "string", fields[0].TypeStr)
	assert.Equal(t, "us-east-1", fields[0].Initial)

	assert.Equal(t, "zones", fields[1].Name)
	assert.False(t, fields[1].Literal, "complex default stays raw HCL")
	assert.False(t, fields[1].Checkbox)
	assert.Equal(t, "any", fields[1].TypeStr)
	assert.Equal(t, `["a", "b"]`, fields[1].Initial, "complex default is pre-formatted as HCL")
}
