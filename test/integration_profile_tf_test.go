//go:build tf

//nolint:paralleltest // CPU profiling is process-global (pprof.StartCPUProfile), so this test must not run in parallel with the rest of the profiling suite.
package test_test

import (
	"io/fs"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFProfileCPUDoesNotPropagateToTofu(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputs)
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	tmpDir := helpers.TmpDirWOSymlinks(t)
	profilePath := filepath.Join(tmpDir, "terragrunt_cpu.prof")

	t.Setenv("TG_EXPERIMENT", "profiling")
	t.Setenv("TG_PROFILE_CPU", profilePath)

	helpers.RunTerragrunt(t, "terragrunt plan --non-interactive --working-dir "+rootPath)

	requireNonEmptyFile(t, profilePath)

	assert.Empty(t, findProfileFiles(t, tmpEnvPath),
		"TG_PROFILE_CPU alone should not produce OpenTofu profiles anywhere under the working dir")
	assert.Equal(t, []string{profilePath}, findProfileFiles(t, tmpDir),
		"the Terragrunt profile should be the only profile produced")
}

// findProfileFiles returns all *.prof files under root, recursively.
func findProfileFiles(t *testing.T, root string) []string {
	t.Helper()

	var (
		mu       sync.Mutex
		profiles []string
	)

	err := vfs.WalkDirParallel(vfs.NewOSFS(), root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(path) == ".prof" {
			mu.Lock()
			defer mu.Unlock()

			profiles = append(profiles, path)
		}

		return nil
	})
	require.NoError(t, err)

	slices.Sort(profiles)

	return profiles
}
