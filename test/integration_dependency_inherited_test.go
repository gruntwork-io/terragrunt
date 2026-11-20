package test_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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

// TestRunReportsAReadCycleThroughADependency pins that a run reports a read_terragrunt_config cycle that passes through
// a dependency block, where the dependency cycle check reads the config that starts the cycle again.
func TestRunReportsAReadCycleThroughADependency(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/read-cycle-through-dependency")
	stub := &runCmdStub{root: root, marks: map[string]int{}}

	v := venv.MemVenv().
		WithFS(venvtest.NewFS(t, root, map[string]string{
			filepath.Join("app", "terragrunt.hcl"): `
dependency "d" {
  config_path  = "../d"
  mock_outputs = { id = "mock" }
}
`,
			filepath.Join("app", "main.tf"): "",
			filepath.Join("d", "terragrunt.hcl"): `
locals {
  e = read_terragrunt_config("../e/terragrunt.hcl")
}
`,
			filepath.Join("d", "main.tf"): "",
			filepath.Join("e", "terragrunt.hcl"): `
dependency "d" {
  config_path  = "../d"
  mock_outputs = { id = "mock" }
}
`,
			filepath.Join("e", "main.tf"): "",
		})).
		WithExec(vexec.NewMemExec(stub.handler(), vexec.WithLookPath(reportStubLookPath)))

	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()

	err := helpers.RunTerragruntCommandWithVenv(
		t,
		ctx,
		v.WithWriter(io.Discard).WithErrWriter(io.Discard),
		"terragrunt run plan --non-interactive --working-dir "+filepath.Join(root, "app"),
	)
	require.NoError(t, ctx.Err(), "the run must end before the deadline")
	assert.True(t, readCycleIn(err), "the run must report the read cycle, got: %v", err)
}

// readCycleIn reports whether err holds a [config.ReadTerragruntConfigCycleError], directly or as the error of a
// function call a diagnostic in it reports.
func readCycleIn(err error) bool {
	if _, ok := errors.AsType[config.ReadTerragruntConfigCycleError](err); ok {
		return true
	}

	diags, ok := errors.AsType[hcl.Diagnostics](err)
	if !ok {
		return false
	}

	return slices.ContainsFunc(diags, func(diag *hcl.Diagnostic) bool {
		call, ok := diag.Extra.(hclsyntax.FunctionCallDiagExtra)

		return ok && readCycleIn(call.FunctionCallError())
	})
}
