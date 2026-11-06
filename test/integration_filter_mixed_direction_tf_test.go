//go:build tf

package test_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTFRunAllFilterAllowDestroyFinishesAfterDeletedUnitFails is a regression test for a
// `run --all --filter-allow-destroy` hang. When a unit deleted in the git range fails its
// `plan -destroy` (here: at HCL evaluation, before OpenTofu runs), the failure used to early-exit
// the deleted unit's *surviving* dependencies even though readiness never lets a destroy gate a
// plan. Surviving units downstream of those then waited forever for a dependency that would never
// be "succeeded", and the run went silent after the last running unit finished - no summary, no
// error, until killed from outside.
//
// Graph (up = surviving plan, down = deleted, plan -destroy):
//
//	base (up, slow) <- a (up) <- b (up)
//	gone1 (down, fails) -> base, a         gone1 fails while `a` is still gated on the slow `base`
//	gone2 (down, ok)    -> b               exists only so `b` is selected into the queue
//
// Only direct dependencies of changed units join the queue, which is why gone1 names `base` too.
func TestTFRunAllFilterAllowDestroyFinishesAfterDeletedUnitFails(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	runner := helpers.InitTestGitRunner(t, tmpDir)

	require.NoError(t, os.WriteFile(
		filepath.Join(tmpDir, ".gitignore"),
		[]byte(".terragrunt-cache/\n.terraform/\n.terraform.lock.hcl\n*.tfstate*\n"),
		0644,
	))

	writeUnit := func(name, hcl string) {
		t.Helper()

		dir := filepath.Join(tmpDir, name)
		require.NoError(t, os.MkdirAll(dir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "terragrunt.hcl"), []byte(hcl), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"),
			[]byte(`output "name" { value = "`+name+`" }`), 0644))
	}

	dependency := func(name string) string {
		return `
dependency "` + name + `" {
  config_path = "../` + name + `"
  mock_outputs = { name = "mock" }
  mock_outputs_allowed_terraform_commands = ["plan"]
}
`
	}

	// `base` sleeps so that `a` is still gated (Ready, not Running) when gone1 fails.
	writeUnit("base", `
terraform {
  before_hook "slow" {
    commands = ["plan"]
    execute  = ["sleep", "5"]
  }
}
`)
	writeUnit("a", dependency("base"))
	writeUnit("b", dependency("a"))
	writeUnit("gone1", dependency("base")+dependency("a")+`
inputs = { broken = does_not_exist.value }
`)
	writeUnit("gone2", dependency("b"))

	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "gone1")))
	require.NoError(t, os.RemoveAll(filepath.Join(tmpDir, "gone2")))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Delete gone1 and gone2"))

	// Before the fix this invocation never returned; the context deadline turns that into a
	// clean failure instead of a hung test.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	cmd := "terragrunt run --all --no-color --non-interactive --working-dir " + tmpDir +
		" --no-filters-file --filter-allow-destroy --filter '...[HEAD~1...HEAD]...'" +
		" --report-file " + helpers.ReportFile + " -- plan"

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutputWithContext(t, ctx, cmd)
	output := stdout + stderr

	require.NoError(t, ctx.Err(), "run --all did not finish: the queue is wedged\n%s", output)
	require.Error(t, err, "gone1's evaluation error must fail the run")
	assert.False(t, errors.Is(err, context.DeadlineExceeded))
	assert.Contains(t, output, "does_not_exist", "the deleted unit's own error must be reported")

	results := map[string]string{}
	for _, run := range helpers.ReadReport(t, tmpDir, helpers.ReportFile) {
		results[filepath.Base(run.Name)] = run.Result
	}

	for _, unit := range []string{"base", "a", "b", "gone2"} {
		assert.Equal(t, string(report.ResultSucceeded), results[unit],
			"%s never gated on gone1 and must run to completion; report: %v", unit, results)
	}

	assert.Equal(t, string(report.ResultFailed), results["gone1"], "report: %v", results)
	assert.NotContains(t, strings.ToLower(output), "early exit",
		"no surviving unit should be cancelled by the deleted unit's failure")
}
