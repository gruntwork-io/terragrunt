// Black-box tests for the generate package. Uses only the exported API
// (generate.GenerateStacks) and a minimal stack fixture written into a
// t.TempDir, not the package's unexported internals.
package generate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// setupSimpleStackFixture writes a minimal two-unit stack fixture into
// tmpDir and returns the absolute path of live/ (the intended --working-dir
// argument).
func setupSimpleStackFixture(t *testing.T, tmpDir string) string {
	t.Helper()

	live := filepath.Join(tmpDir, "live")
	unit := filepath.Join(tmpDir, "unit")

	require.NoError(t, os.MkdirAll(live, 0o755))
	require.NoError(t, os.MkdirAll(unit, 0o755))

	stackHCL := `
unit "u1" {
  source = "../unit"
  path   = "u1"
}

unit "u2" {
  source = "../unit"
  path   = "u2"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(live, "terragrunt.stack.hcl"), []byte(stackHCL), 0o600))

	unitHCL := `terraform { source = "." }`
	require.NoError(t, os.WriteFile(filepath.Join(unit, "terragrunt.hcl"), []byte(unitHCL), 0o600))

	mainTF := `resource "local_file" "f" {
  content  = "hi"
  filename = "${path.module}/out.txt"
}
`
	require.NoError(t, os.WriteFile(filepath.Join(unit, "main.tf"), []byte(mainTF), 0o600))

	return live
}

func newTestOptions(t *testing.T, workingDir string) *options.TerragruntOptions {
	t.Helper()

	opts, err := options.NewTerragruntOptionsForTest(filepath.Join(workingDir, "terragrunt.stack.hcl"))
	require.NoError(t, err)

	opts.WorkingDir = workingDir
	opts.Parallelism = 4

	return opts
}

// TestGenerateStacksProducesExpectedUnits asserts the happy path: calling
// the exported generate.GenerateStacks on a fresh fixture writes both unit
// directories under .terragrunt-stack/.
func TestGenerateStacksProducesExpectedUnits(t *testing.T) {
	t.Parallel()

	workingDir := setupSimpleStackFixture(t, t.TempDir())
	opts := newTestOptions(t, workingDir)

	err := generate.GenerateStacks(t.Context(), logger.CreateLogger(), opts, nil)
	require.NoError(t, err)

	for _, unit := range []string{"u1", "u2"} {
		dest := filepath.Join(workingDir, ".terragrunt-stack", unit)
		info, err := os.Stat(dest)
		require.NoError(t, err, "generated unit dir missing: %s", dest)
		require.True(t, info.IsDir(), "generated unit is not a directory: %s", dest)

		require.FileExists(t, filepath.Join(dest, "terragrunt.hcl"), "unit %s missing terragrunt.hcl", unit)
		require.FileExists(t, filepath.Join(dest, "main.tf"), "unit %s missing main.tf", unit)
	}
}

// TestGenerateStacksDedupAtDiscoveryWithRacing verifies the canonicalization
// fix: two in-process GenerateStacks invocations against the same working
// directory complete cleanly without the race detector firing and leave a
// complete output tree. The guarantee is intra-invocation dedup at the
// discovery boundary, not inter-invocation serialization, so this test
// asserts what the code actually delivers. Callers that truly need
// cross-invocation isolation must coordinate externally.
//
// The WithRacing suffix routes this test into the CI -race matrix; any
// Go-level data race within a single invocation would be flagged there.
func TestGenerateStacksDedupAtDiscoveryWithRacing(t *testing.T) {
	t.Parallel()

	workingDir := setupSimpleStackFixture(t, t.TempDir())
	// Build opts on the test goroutine so NewTerragruntOptionsForTest's
	// assertions don't fire from a worker goroutine (testifylint).
	opts := []*options.TerragruntOptions{
		newTestOptions(t, workingDir),
		newTestOptions(t, workingDir),
	}

	var eg errgroup.Group

	for i := range 2 {
		eg.Go(func() error {
			return generate.GenerateStacks(t.Context(), logger.CreateLogger(), opts[i], nil)
		})
	}

	require.NoError(t, eg.Wait())

	for _, unit := range []string{"u1", "u2"} {
		require.DirExists(t, filepath.Join(workingDir, ".terragrunt-stack", unit))
	}
}

// TestGenerateStacksEmptyWorkingDirReturnsNoError asserts that
// GenerateStacks against an empty working dir (no terragrunt.stack.hcl)
// returns nil without error. The CLI surface-level "No stack files found"
// warning is exercised via the integration tests, which drive the real CLI
// entry point. This unit test covers only the no-error contract.
func TestGenerateStacksEmptyWorkingDirReturnsNoError(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newTestOptions(t, workingDir)

	require.NoError(t, generate.GenerateStacks(t.Context(), logger.CreateLogger(), opts, nil))
}
