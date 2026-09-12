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

// TestExecPlanSurvivesAUnitPinningATerragruntVersion pins that the version
// check a run performs has a version to check against. Each tool builds its
// own options, and the check dereferences what it is handed, so a unit naming
// a constraint used to panic inside a runner goroutine no tool handler could
// recover from, taking the whole server down with the one call.
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

	// Whether the run itself gets anywhere depends on the tofu on this
	// machine. Answering at all is the assertion: the constraint check runs
	// before the run does, and it no longer takes the server with it.
	_, err = session.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "plan",
		Arguments: map[string]any{},
	})
	require.NoError(t, err)
}
