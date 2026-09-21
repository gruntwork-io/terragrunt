//go:build tf

package mcp_test

import (
	"path/filepath"
	"testing"

	tgmcp "github.com/gruntwork-io/terragrunt/internal/cli/commands/mcp"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// thingModule manages one resource that lives only in local state, so a real
// apply and destroy change nothing outside the test's directory.
const thingModule = `
variable "name" {
  type = string
}

resource "terraform_data" "this" {
  input = var.name
}

output "id" {
  value = terraform_data.this.output
}

output "secret" {
  value     = "secret-${var.name}"
  sensitive = true
}
`

const thingUnitA = `
terraform {
  source = "../modules/thing"
}

inputs = {
  name = "a"
}
`

const thingUnitB = `
terraform {
  source = "../modules/thing"
}

dependency "a" {
  config_path  = "../a"
  mock_outputs = { id = "mock-a" }
}

inputs = {
  name = "b-from-${dependency.a.outputs.id}"
}
`

type runAllUnit struct {
	Path   string `json:"path"`
	Result string `json:"result"`
}

type runAllOutput struct {
	Units     []runAllUnit `json:"units"`
	Degraded  []string     `json:"degraded"`
	Succeeded int          `json:"succeeded"`
	Failed    int          `json:"failed"`
}

// TestTFApplyAndDestroyChangeRealState drives the run tools against a real
// binary: a plan, an accepted apply whose outputs read back with and without
// the sensitive values, and an accepted destroy that leaves nothing to read.
func TestTFApplyAndDestroyChangeRealState(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{
		"modules/thing/main.tf": thingModule,
		"a/terragrunt.hcl":      thingUnitA,
		"b/terragrunt.hcl":      thingUnitB,
	})

	var prompts []string

	session := newAnsweringSession(t, dir, venvtest.NewOSWithEmptyEnv(), "accept", &prompts,
		func(o *tgmcp.Options) {
			o.Allow = []string{"exec"}
			o.AllowApply = true
		})

	var planned runAllOutput

	callTool(t, session, "plan", map[string]any{}, &planned)
	require.Equal(t, 2, planned.Succeeded, "plan: %+v", planned)
	assert.Empty(t, prompts, "a plan must not ask")

	var applied runAllOutput

	callTool(t, session, "apply", map[string]any{}, &applied)
	require.Equal(t, 2, applied.Succeeded, "apply: %+v", applied)
	require.Len(t, prompts, 1, "an apply must ask once")

	var redacted getOutputsOutput

	callTool(t, session, "get_outputs", map[string]any{"working_dir": "b"}, &redacted)
	assert.Equal(t, map[string]outputEntry{
		"id":     {Value: "b-from-a"},
		"secret": {Value: "(sensitive)", Sensitive: true},
	}, redacted.Outputs, "b must see the output a applied")
	assert.Equal(t, []string{"secret"}, redacted.Redacted)

	var revealed getOutputsOutput

	callTool(t, session, "get_outputs", map[string]any{"working_dir": "b", "include_sensitive": true}, &revealed)
	assert.Equal(t, outputEntry{Value: "secret-b-from-a", Sensitive: true}, revealed.Outputs["secret"])
	assert.Empty(t, revealed.Redacted)

	var destroyed runAllOutput

	callTool(t, session, "destroy", map[string]any{}, &destroyed)
	require.Equal(t, 2, destroyed.Succeeded, "destroy: %+v", destroyed)
	assert.Len(t, prompts, 2, "a destroy must ask once")

	for _, unit := range []string{"a", "b"} {
		var after getOutputsOutput

		callTool(t, session, "get_outputs", map[string]any{"working_dir": unit}, &after)
		assert.Empty(t, after.Outputs, "%s must have nothing left to read after the destroy", unit)
	}
}

// TestTFApplyFromAClientThatCannotAskChangesNothing pins that a run nobody
// could approve does not happen.
func TestTFApplyFromAClientThatCannotAskChangesNothing(t *testing.T) {
	t.Parallel()

	dir := writeTree(t, map[string]string{
		"modules/thing/main.tf": thingModule,
		"a/terragrunt.hcl":      thingUnitA,
	})

	session := newRealExecSession(t, dir, func(o *tgmcp.Options) {
		o.Allow = []string{"exec"}
		o.AllowApply = true
	})

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "apply", Arguments: map[string]any{}})
	require.NoError(t, err)
	assert.True(t, res.IsError, "a client that cannot ask must be refused")

	var out getOutputsOutput

	callTool(t, session, "get_outputs", map[string]any{"working_dir": "a"}, &out)
	assert.Empty(t, out.Outputs, "the refused apply must not have run")
}

// TestTFHookNamedByConfigurationDoesNotRun pins that --allow=exec does not
// reach a hook. The plan still runs, and the degraded list names the hook.
func TestTFHookNamedByConfigurationDoesNotRun(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(t.TempDir(), "ran.txt")

	dir := writeTree(t, map[string]string{
		"modules/thing/main.tf": thingModule,
		"hooked/terragrunt.hcl": `
terraform {
  source = "../modules/thing"

  before_hook "marker" {
    commands = ["plan"]
    execute  = ["sh", "-c", "echo ran > ` + marker + `"]
  }
}

inputs = {
  name = "hooked"
}
`,
	})

	session := newRealExecSession(t, dir, func(o *tgmcp.Options) { o.Allow = []string{"exec"} })

	var out runAllOutput

	callTool(t, session, "plan", map[string]any{}, &out)
	assert.Equal(t, 1, out.Succeeded, "plan: %+v", out)
	assert.NotEmpty(t, out.Degraded, "the refused hook must be reported")

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.False(t, exists, "the hook must not have run")
}

// TestTFApplyFailsOnAHookNothingAllowed pins that an apply does not proceed
// past a hook it refused. The plan tool stubs a refused hook as a success, and
// an apply doing the same would skip a hook written to block it.
func TestTFApplyFailsOnAHookNothingAllowed(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(t.TempDir(), "ran.txt")

	dir := writeTree(t, map[string]string{
		"modules/thing/main.tf": thingModule,
		"gated/terragrunt.hcl": `
terraform {
  source = "../modules/thing"

  before_hook "gate" {
    commands = ["apply"]
    execute  = ["sh", "-c", "echo ran > ` + marker + `"]
  }
}

inputs = {
  name = "gated"
}
`,
	})

	var prompts []string

	session := newAnsweringSession(t, dir, venvtest.NewOSWithEmptyEnv(), "accept", &prompts,
		func(o *tgmcp.Options) {
			o.Allow = []string{"exec"}
			o.AllowApply = true
		})

	var applied runAllOutput

	callTool(t, session, "apply", map[string]any{}, &applied)
	require.Len(t, prompts, 1, "the apply must ask once")
	assert.Equal(t, 1, applied.Failed, "the refused hook must fail the unit: %+v", applied)
	assert.NotEmpty(t, applied.Degraded, "the refused hook must be reported")

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.False(t, exists, "the hook must not have run")

	var out getOutputsOutput

	callTool(t, session, "get_outputs", map[string]any{"working_dir": "gated"}, &out)
	assert.Empty(t, out.Outputs, "nothing must have been applied")
}

// TestTFConfigurationCannotChooseTheBinary pins that a unit's terraform_binary
// does not pick what a run starts. Terragrunt starts that binary as one of its
// own commands, so a tree naming a file it ships would otherwise run that file
// under --allow=exec alone.
func TestTFConfigurationCannotChooseTheBinary(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(t.TempDir(), "ran.txt")

	dir := writeTree(t, map[string]string{
		"modules/thing/main.tf": thingModule,
		"unit/terragrunt.hcl": `
terraform_binary = "./tofu"

terraform {
  source = "../modules/thing"
}

inputs = {
  name = "unit"
}
`,
	})
	require.NoError(t, vfs.WriteFile(
		vfs.NewOSFS(),
		filepath.Join(dir, "unit", "tofu"),
		[]byte("#!/bin/sh\necho ran > "+marker+"\n"),
		0o755,
	))

	session := newRealExecSession(t, dir, func(o *tgmcp.Options) { o.Allow = []string{"exec"} })

	var out runAllOutput

	callTool(t, session, "plan", map[string]any{}, &out)

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.False(t, exists, "the binary the configuration named must not have run")
	assert.Equal(t, 1, out.Succeeded, "the server's own binary must plan the unit: %+v", out)
}

// TestTFConfigurationCannotStartAnEngine pins that a unit's engine block does
// not start its plugin. Terragrunt starts the plugin as one of its own
// commands, so under the iac-engine experiment a tree naming a file it ships
// would otherwise run that file under --allow=exec alone.
func TestTFConfigurationCannotStartAnEngine(t *testing.T) {
	t.Parallel()

	marker := filepath.Join(t.TempDir(), "ran.txt")
	enginePath := filepath.Join(t.TempDir(), "engine")

	require.NoError(t, vfs.WriteFile(
		vfs.NewOSFS(),
		enginePath,
		[]byte("#!/bin/sh\necho ran > "+marker+"\n"),
		0o755,
	))

	dir := writeTree(t, map[string]string{
		"modules/thing/main.tf": thingModule,
		"unit/terragrunt.hcl": `
engine {
  source = "` + filepath.ToSlash(enginePath) + `"
}

terraform {
  source = "../modules/thing"
}

inputs = {
  name = "unit"
}
`,
	})

	session := newRealExecSession(t, dir, func(o *tgmcp.Options) {
		o.Allow = []string{"exec"}
		require.NoError(t, o.Experiments.EnableExperiment(experiment.IacEngine))
	})

	var out runAllOutput

	callTool(t, session, "plan", map[string]any{}, &out)

	exists, err := vfs.FileExists(vfs.NewOSFS(), marker)
	require.NoError(t, err)
	assert.False(t, exists, "the engine the configuration named must not have started")
	assert.Equal(t, 1, out.Succeeded, "the server's own binary must plan the unit: %+v", out)
}
