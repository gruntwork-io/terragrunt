package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// TestMergeUnitValuesWithStackValues_StackKindReturnsUnitValues pins that for stack components, the helper is a no-op and returns the unit-declared values unchanged. Stack-level values propagate via the recursive GenerateStackFile's own values-file read, not via this helper.
func TestMergeUnitValuesWithStackValues_StackKindReturnsUnitValues(t *testing.T) {
	t.Parallel()

	unit := ctyObjPtr(map[string]cty.Value{"a": cty.StringVal("from-unit")})
	stack := ctyObjPtr(map[string]cty.Value{"b": cty.StringVal("from-stack")})

	got, err := mergeUnitValuesWithStackValues(unit, stack, stackKind)
	require.NoError(t, err)
	assert.Same(t, unit, got, "stackKind path must return the unit values pointer unchanged")
}

// TestMergeUnitValuesWithStackValues_UnitKindMerges combines stack and unit values for unit components.
func TestMergeUnitValuesWithStackValues_UnitKindMerges(t *testing.T) {
	t.Parallel()

	unit := ctyObjPtr(map[string]cty.Value{"a": cty.StringVal("from-unit")})
	stack := ctyObjPtr(map[string]cty.Value{"b": cty.StringVal("from-stack")})

	got, err := mergeUnitValuesWithStackValues(unit, stack, unitKind)
	require.NoError(t, err)
	assert.NotNil(t, got)
	values := got.AsValueMap()
	assert.Equal(t, "from-unit", values["a"].AsString())
	assert.Equal(t, "from-stack", values["b"].AsString())
}

// TestMergeUnitValuesWithStackValues_UnitWinsOnConflict ensures unit-declared values override propagated stack values on the same key.
func TestMergeUnitValuesWithStackValues_UnitWinsOnConflict(t *testing.T) {
	t.Parallel()

	unit := ctyObjPtr(map[string]cty.Value{"k": cty.StringVal("from-unit")})
	stack := ctyObjPtr(map[string]cty.Value{"k": cty.StringVal("from-stack")})

	got, err := mergeUnitValuesWithStackValues(unit, stack, unitKind)
	require.NoError(t, err)
	assert.Equal(t, "from-unit", got.AsValueMap()["k"].AsString())
}

// TestMergeUnitValuesWithStackValues_NilStack returns the unit values when there are no stack values to propagate.
func TestMergeUnitValuesWithStackValues_NilStack(t *testing.T) {
	t.Parallel()

	unit := ctyObjPtr(map[string]cty.Value{"a": cty.StringVal("v")})

	got, err := mergeUnitValuesWithStackValues(unit, nil, unitKind)
	require.NoError(t, err)
	assert.Same(t, unit, got)
}

// TestMergeUnitValuesWithStackValues_NilUnit yields a value containing only the stack-level values when the unit has none.
func TestMergeUnitValuesWithStackValues_NilUnit(t *testing.T) {
	t.Parallel()

	stack := ctyObjPtr(map[string]cty.Value{"a": cty.StringVal("v")})

	got, err := mergeUnitValuesWithStackValues(nil, stack, unitKind)
	require.NoError(t, err)
	assert.Equal(t, "v", got.AsValueMap()["a"].AsString())
}

func TestMergeUnitValuesWithStackValues_InvalidUnitValuesErrors(t *testing.T) {
	t.Parallel()

	unit := cty.StringVal("invalid")
	stack := ctyObjPtr(map[string]cty.Value{"a": cty.StringVal("v")})

	_, err := mergeUnitValuesWithStackValues(&unit, stack, unitKind)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unit values must be object or map")
}

func TestMergeStackAutoIncludeValues_InvalidBaseValuesErrors(t *testing.T) {
	t.Parallel()

	base := cty.StringVal("invalid")
	autoInclude := ctyObjPtr(map[string]cty.Value{"a": cty.StringVal("v")})

	_, err := mergeStackAutoIncludeValues(&base, autoInclude)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stack block values must be object or map")
}

func ctyObjPtr(m map[string]cty.Value) *cty.Value {
	v := cty.ObjectVal(m)
	return &v
}
