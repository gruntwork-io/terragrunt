package tui_test

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui"
	"github.com/gruntwork-io/terragrunt/internal/services/catalog/component"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// TestCopyCmdInstallsDiscoveredComponent covers what CopyCmd adds on top of
// component.Install: resolving a discovered component's source directory
// inside its cloned repository, and its destination from the options.
func TestCopyCmdInstallsDiscoveredComponent(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir

	writeFileFS(t, fsys, filepath.Join(repoDir, "vpc", "terragrunt.hcl"), "# vpc unit\n")
	writeFileFS(t, fsys, filepath.Join(repoDir, "vpc", "inputs.hcl"), "# inputs\n")

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := tui.NewComponentDiscovery().Discover(fsys, repo)
	require.NoError(t, err)
	require.Len(t, components, 1)
	require.Equal(t, component.KindUnit, components[0].Kind)

	workingDir := testWorkingDir
	require.NoError(t, fsys.MkdirAll(workingDir, 0o755))

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = workingDir

	cmd := tui.NewCopyCmd(logger.CreateLogger(), opts, components[0])

	chained := cmd.WithFS(fsys)
	assert.Same(t, cmd, chained, "WithFS should return the same builder for chaining")

	require.NoError(t, cmd.Run())

	assert.Equal(t, workingDir, cmd.Result().Dir, "Result should record the destination used")

	exists, err := vfs.FileExists(fsys, filepath.Join(workingDir, "inputs.hcl"))
	require.NoError(t, err)
	assert.True(t, exists, "the component's files should land in the working directory")
}

func TestCopyCmdPanicsWithoutFS(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = t.TempDir()

	assert.Panics(t, func() {
		err := tui.NewCopyCmd(logger.CreateLogger(), opts, nil).Run()
		t.Errorf("Run returned %v instead of panicking", err)
	})
}

func TestCopyCmdRejectsNilComponent(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = t.TempDir()

	err := tui.NewCopyCmd(logger.CreateLogger(), opts, nil).WithFS(vfs.NewMemMapFS()).Run()
	require.ErrorIs(t, err, tui.ErrNilComponent)
}

func TestCopyCmdRejectsEmptyWorkingDir(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	repoDir := testRepoDir
	writeFileFS(t, fsys, filepath.Join(repoDir, "vpc", "terragrunt.hcl"), "# unit\n")

	repo := newFakeRepo(t, fsys, repoDir)

	components, err := tui.NewComponentDiscovery().Discover(fsys, repo)
	require.NoError(t, err)
	require.Len(t, components, 1)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = ""

	err = tui.NewCopyCmd(logger.CreateLogger(), opts, components[0]).WithFS(fsys).Run()
	require.ErrorIs(t, err, tui.ErrEmptyWorkingDir)
}

// TestCopyCmdResultZeroValueBeforeRun verifies Result is callable before Run
// and returns the zero value.
func TestCopyCmdResultZeroValueBeforeRun(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = t.TempDir()

	cmd := tui.NewCopyCmd(logger.CreateLogger(), opts, nil)

	assert.Equal(
		t,
		component.Result{},
		cmd.Result(),
		"Result should return the zero value before Run",
	)
}

// TestCopyCmdStdioSettersAreNoops verifies the tea.ExecCommand stdio setters
// are safe no-ops. CopyCmd does not read or write through these, but they
// satisfy the tea.ExecCommand interface.
func TestCopyCmdStdioSettersAreNoops(t *testing.T) {
	t.Parallel()

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = t.TempDir()

	cmd := tui.NewCopyCmd(logger.CreateLogger(), opts, nil)

	assert.NotPanics(t, func() {
		cmd.SetStdin(bytes.NewReader(nil))
		cmd.SetStdout(&bytes.Buffer{})
		cmd.SetStderr(&bytes.Buffer{})
	})
}
