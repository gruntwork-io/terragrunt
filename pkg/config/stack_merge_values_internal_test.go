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

// FuzzMergeUnitValuesWithStackValues drives the merge helper with assorted cty value shapes and asserts no panic. Permitted outcomes: either a non-error merge whose result is object/map or nil, or an error.
func FuzzMergeUnitValuesWithStackValues(f *testing.F) {
	type seed struct {
		unitKey     string
		stackKey    string
		hasUnit     bool
		hasStack    bool
		isStackKind bool
	}

	seeds := []seed{
		{"a", "b", true, true, false},
		{"k", "k", true, true, false},
		{"a", "", true, false, false},
		{"", "a", false, true, false},
		{"", "", false, false, false},
		{"a", "b", true, true, true},
	}

	for _, s := range seeds {
		f.Add(s.hasUnit, s.hasStack, s.unitKey, s.stackKey, s.isStackKind)
	}

	f.Fuzz(func(t *testing.T, hasUnit, hasStack bool, unitKey, stackKey string, isStackKind bool) {
		var unit, stack *cty.Value

		if hasUnit && unitKey != "" {
			unit = ctyObjPtr(map[string]cty.Value{unitKey: cty.StringVal("u")})
		}

		if hasStack && stackKey != "" {
			stack = ctyObjPtr(map[string]cty.Value{stackKey: cty.StringVal("s")})
		}

		kind := unitKind
		if isStackKind {
			kind = stackKind
		}

		got, err := mergeUnitValuesWithStackValues(unit, stack, kind)
		if err != nil {
			return
		}

		if got == nil {
			return
		}

		require.True(t, got.Type().IsObjectType() || got.Type().IsMapType(), "merge result must be object or map (got %s)", got.Type().FriendlyName())
	})
}

// FuzzMergeStackAutoIncludeValues drives the autoinclude merge helper with assorted shapes; verifies no panic and that successful results are object/map (or nil).
func FuzzMergeStackAutoIncludeValues(f *testing.F) {
	seeds := []struct {
		baseKey string
		autoKey string
		hasBase bool
		hasAuto bool
	}{
		{"a", "b", true, true},
		{"k", "k", true, true},
		{"", "a", false, true},
		{"a", "", true, false},
		{"", "", false, false},
	}

	for _, s := range seeds {
		f.Add(s.hasBase, s.hasAuto, s.baseKey, s.autoKey)
	}

	f.Fuzz(func(t *testing.T, hasBase, hasAuto bool, baseKey, autoKey string) {
		var base, auto *cty.Value

		if hasBase && baseKey != "" {
			base = ctyObjPtr(map[string]cty.Value{baseKey: cty.StringVal("b")})
		}

		if hasAuto && autoKey != "" {
			auto = ctyObjPtr(map[string]cty.Value{autoKey: cty.StringVal("a")})
		}

		got, err := mergeStackAutoIncludeValues(base, auto)
		if err != nil {
			return
		}

		if got == nil {
			return
		}

		require.True(t, got.Type().IsObjectType() || got.Type().IsMapType(), "merge result must be object or map (got %s)", got.Type().FriendlyName())
	})
}
