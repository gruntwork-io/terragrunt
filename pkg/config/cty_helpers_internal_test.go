package config //nolint:testpackage // needs access to unexported includeConfigAsCtyVal / includeBlockLabel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
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
