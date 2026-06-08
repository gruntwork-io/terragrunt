package config //nolint:testpackage // needs access to unexported includeConfigAsCtyVal / includeBlockLabel

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// TestIncludeConfigAsCtyValWrapsErrorWithIncludeNameAndPath is a regression test for
// https://github.com/gruntwork-io/terragrunt/issues/6282. When resolving an exposed include fails, the error must
// identify WHICH include block and WHICH included (parent) file caused it — otherwise low-level, location-less
// errors (e.g. the gocty "unsuitable value: a bool is required") are impossible to debug.
func TestIncludeConfigAsCtyValWrapsErrorWithIncludeNameAndPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Parent (included) config with an invalid expression so its full parse fails deterministically.
	parentPath := filepath.Join(tmpDir, "root.hcl")
	require.NoError(t, os.WriteFile(parentPath, []byte("locals {\n  x = \n}\n"), 0644))

	childPath := filepath.Join(tmpDir, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(childPath, []byte("terraform {\n  source = \".\"\n}\n"), 0644))

	l := logger.CreateLogger()
	ctx, pctx := NewParsingContext(t.Context(), l, WithStrictControls(controls.New()))
	pctx.TerragruntConfigPath = childPath
	pctx.WorkingDir = tmpDir

	expose := true
	inc := IncludeConfig{Name: "root", Path: parentPath, Expose: &expose}

	_, err := includeConfigAsCtyVal(ctx, pctx, l, inc)
	require.Error(t, err)
	require.Contains(t, err.Error(), `exposed include "root"`,
		"error should name the include block so the user knows WHERE resolution failed")
	require.Contains(t, err.Error(), parentPath, "error should name the included (parent) file")
}

// TestIncludeBlockLabel verifies the human-readable identifier used in exposed-include error messages.
func TestIncludeBlockLabel(t *testing.T) {
	t.Parallel()

	require.Equal(t, `"root"`, includeBlockLabel(IncludeConfig{Name: "root"}))
	require.Equal(t, "(bare include)", includeBlockLabel(IncludeConfig{Name: bareIncludeKey}))
}

// TestCtyPathString covers field-level annotation of exposed-include resolution errors (issue #6282). The
// conversion layer folds the field name and the failing attribute path into a single dotted locator (e.g.
// `dependency.outputs["enabled"]`) so the message is unambiguous, and degrades to just the field name when
// go-cty has no path. The underlying cty.PathError is preserved through the %w chain.
func TestCtyPathString(t *testing.T) {
	t.Parallel()

	// Conversion that descends through a Go map to reach the bad value -> populated cty.Path.
	target := cty.Object(map[string]cty.Type{"outputs": cty.Map(cty.Bool)})
	_, descend := gocty.ToCtyValue(map[string]any{"outputs": map[string]cty.Value{"enabled": cty.StringVal("x")}}, target)
	require.Error(t, descend)
	assert.Equal(t, `.outputs["enabled"]`, ctyPathString(descend))

	// Mirror the conversion-layer wrap: field name + path collapse into one locator.
	folded := fmt.Errorf("%s%s: %w", MetadataDependency, ctyPathString(descend), descend)
	assert.Equal(t, `dependency.outputs["enabled"]: unsuitable value: a bool is required`, folded.Error())

	var pathErr cty.PathError
	require.ErrorAs(t, folded, &pathErr, "cty.PathError should survive the %w chain")

	// Top-level conversion of an already-typed value -> empty path: folds to just the field name.
	_, bare := gocty.ToCtyValue(cty.StringVal("x"), cty.Bool)
	require.Error(t, bare)
	assert.Empty(t, ctyPathString(bare))

	foldedBare := fmt.Errorf("%s%s: %w", MetadataDependency, ctyPathString(bare), bare)
	assert.Equal(t, `dependency: unsuitable value: a bool is required`, foldedBare.Error())
}
