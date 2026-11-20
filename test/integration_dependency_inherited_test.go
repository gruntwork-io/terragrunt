package test_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunReportsTheErrorOfADependencyThatReadsTheLocalsOfItsDependent pins the error a run of a unit reports for a
// dependency whose feature block reads a local only the unit defines: the error of the dependency output fetch, since
// the dependency cycle check reads the dependency with the unit's locals, as the fetch does.
func TestRunReportsTheErrorOfADependencyThatReadsTheLocalsOfItsDependent(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/inherited-locals")
	stub := &runCmdStub{root: root, marks: map[string]int{}}

	v := venv.MemVenv().
		WithFS(venvtest.NewFS(t, root, map[string]string{
			filepath.Join("app", "terragrunt.hcl"): `
locals {
  env = "dev"
}

dependency "db" {
  config_path = "../db"

  mock_outputs = {
    id = "mock"
  }
}

inputs = {
  db_id = dependency.db.outputs.id
}
`,
			filepath.Join("app", "main.tf"): "",
			filepath.Join("db", "terragrunt.hcl"): `
feature "env" {
  default = local.env
}

dependency "net" {
  config_path = "../net"

  mock_outputs = {
    id = "mock"
  }
}
`,
			filepath.Join("db", "main.tf"):         "",
			filepath.Join("net", "terragrunt.hcl"): "",
			filepath.Join("net", "main.tf"):        "",
		})).
		WithExec(vexec.NewMemExec(stub.handler(), vexec.WithLookPath(reportStubLookPath)))

	_, _, err := helpers.RunTerragruntCommandWithOutputWithVenv(
		t,
		v,
		"terragrunt run plan --non-interactive --working-dir "+filepath.Join(root, "app"),
	)

	var diags hcl.Diagnostics
	require.ErrorAs(t, err, &diags)
	require.Len(t, diags, 1)
	assert.Equal(t, "Unsupported attribute", diags[0].Summary)
	assert.Empty(t, stub.unexpectedCommands())
}
