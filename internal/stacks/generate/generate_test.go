// Black-box tests for the generate package. Uses only the exported API
// (generate.GenerateStacks) and real stack fixtures, not the package's
// unexported internals.
package generate_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
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

// TestGenerateStacksConcurrentWithRacing asserts that two concurrent
// invocations of generate.GenerateStacks against the same working
// directory both complete successfully without racing on the shared
// .terragrunt-stack/ output tree. Under `-race` any shared-state race
// inside the package would be flagged by the race detector; the WithRacing
// suffix ensures this test is picked up by the CI race matrix.
func TestGenerateStacksConcurrentWithRacing(t *testing.T) {
	t.Parallel()

	workingDir := setupSimpleStackFixture(t, t.TempDir())
	// Build opts on the test goroutine so NewTerragruntOptionsForTest's
	// assertions don't fire from a worker goroutine (testifylint).
	opts := []*options.TerragruntOptions{
		newTestOptions(t, workingDir),
		newTestOptions(t, workingDir),
	}

	var wg sync.WaitGroup

	wg.Add(2)

	errs := make(chan error, 2)

	for i := range 2 {
		go func() {
			defer wg.Done()

			errs <- generate.GenerateStacks(t.Context(), logger.CreateLogger(), opts[i], nil)
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	// Both goroutines targeted the same output tree; assert the final
	// state is complete (every unit present) rather than the partial/
	// corrupt state a race would have produced.
	for _, unit := range []string{"u1", "u2"} {
		require.DirExists(t, filepath.Join(workingDir, ".terragrunt-stack", unit))
	}
}

// TestGenerateStacksNoStackFile asserts that calling GenerateStacks on a
// working directory with no terragrunt.stack.hcl returns without error
// (the command is a no-op).
func TestGenerateStacksNoStackFile(t *testing.T) {
	t.Parallel()

	workingDir := t.TempDir()
	opts := newTestOptions(t, workingDir)

	require.NoError(t, generate.GenerateStacks(t.Context(), logger.CreateLogger(), opts, nil))
}
