package tfimpl_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/stretchr/testify/assert"
)

// TestDefaultBinaryDoesNotSpawn guards the PATH-lookup-only contract:
// NewNoSpawnExec resolves LookPath but errors on any process launch, so a probe
// that spawns falls through to Terraform here.
func TestDefaultBinaryDoesNotSpawn(t *testing.T) {
	t.Parallel()

	assert.Equal(t, tfimpl.TofuDefaultPath, tfimpl.DefaultBinary(vexec.NewNoSpawnExec()))
}

func TestDefaultBinaryFallsBackToTerraform(t *testing.T) {
	t.Parallel()

	e := &vexec.NoLookPathExec{Exec: vexec.NewNoSpawnExec()}

	assert.Equal(t, tfimpl.TerraformDefaultPath, tfimpl.DefaultBinary(e))
}

func TestBinaryOrDefault(t *testing.T) {
	t.Parallel()

	e := &vexec.NoLookPathExec{Exec: vexec.NewNoSpawnExec()}

	assert.Equal(t, "/opt/bin/custom-tofu", tfimpl.BinaryOrDefault(e, "/opt/bin/custom-tofu"))
	assert.Equal(t, tfimpl.TerraformDefaultPath, tfimpl.BinaryOrDefault(e, ""))
}
