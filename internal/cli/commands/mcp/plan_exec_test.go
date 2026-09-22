//go:build exec

// Version-constraint parity test for the run tools. It drives the real runner,
// which the fail-closed venv the rest of the package runs on stops before the
// check this covers, so it stays behind the exec tag.

package mcp_test

import (
	"path/filepath"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// unitPinsTerragruntVersion names a Terragrunt version constraint, which is
// what sends a run through the version check.
const unitPinsTerragruntVersion = `
terragrunt_version_constraint = ">= 0.0.0"
`

// TestExecPlanSurvivesAUnitPinningATerragruntVersion pins that a plan of a unit
// naming terragrunt_version_constraint is answered. The constraint is checked
// inside a runner goroutine, where a panic ends the whole server because no
// tool handler can recover it.
func TestExecPlanSurvivesAUnitPinningATerragruntVersion(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	unit := filepath.Join(dir, "unit")

	fsys := vfs.NewOSFS()
	require.NoError(t, fsys.MkdirAll(unit, 0o755))
	require.NoError(t, vfs.WriteFile(
		fsys, filepath.Join(unit, "terragrunt.hcl"), []byte(unitPinsTerragruntVersion), 0o644))

	resolved, err := vfs.EvalSymlinks(fsys, dir)
	require.NoError(t, err)

	session := newRealExecSession(t, resolved, func(o *tgmcp.Options) {
		o.Allow = []string{"exec"}
	})

	// Whether the run gets anywhere depends on the tofu on this machine, so the
	// assertion is only that the call is answered.
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "plan",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
}
