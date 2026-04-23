package redesign_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/catalog/tui/redesign"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyCmd_CopiesIntoWorkingDirectory(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)

	writeFile(t, filepath.Join(repoDir, "vpc", "terragrunt.hcl"), "# vpc unit\n")
	writeFile(t, filepath.Join(repoDir, "vpc", "inputs.hcl"), "# inputs\n")
	writeFile(t, filepath.Join(repoDir, "vpc", ".terragrunt-cache", "junk.txt"), "should be skipped")

	repo := newFakeRepo(t, repoDir)

	components, err := redesign.NewComponentDiscovery().Discover(repo)
	require.NoError(t, err)
	require.Len(t, components, 1)
	require.Equal(t, redesign.ComponentKindUnit, components[0].Kind)

	workingDir := t.TempDir()
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = workingDir

	cmd := redesign.NewCopyCmdForTest(logger.CreateLogger(), opts, components[0])
	require.NoError(t, cmd.Run())

	assert.FileExists(t, filepath.Join(workingDir, "terragrunt.hcl"))
	assert.FileExists(t, filepath.Join(workingDir, "inputs.hcl"))
	assert.NoFileExists(t, filepath.Join(workingDir, ".terragrunt-cache", "junk.txt"))
	assert.NoDirExists(t, filepath.Join(workingDir, ".terragrunt-cache"))
}

func TestCopyCmd_RefusesToOverwriteExistingFile(t *testing.T) {
	t.Parallel()

	repoDir := helpers.TmpDirWOSymlinks(t)
	writeFile(t, filepath.Join(repoDir, "stack-a", "terragrunt.stack.hcl"), "# stack\n")

	repo := newFakeRepo(t, repoDir)

	components, err := redesign.NewComponentDiscovery().Discover(repo)
	require.NoError(t, err)
	require.Len(t, components, 1)

	workingDir := t.TempDir()
	writeFile(t, filepath.Join(workingDir, "terragrunt.stack.hcl"), "# preexisting")

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = workingDir

	cmd := redesign.NewCopyCmdForTest(logger.CreateLogger(), opts, components[0])
	err = cmd.Run()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}
