package discovery_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	helpers.CreateGitRepo(t, tmpDir)

	// Create three units
	unitToBeModifiedDir := filepath.Join(tmpDir, "unit-to-be-modified")
	unitToBeRemovedDir := filepath.Join(tmpDir, "unit-to-be-removed")
	unitToBeUntouchedDir := filepath.Join(tmpDir, "unit-to-be-untouched")

	err = os.MkdirAll(unitToBeModifiedDir, 0755)
	require.NoError(t, err)

	err = os.MkdirAll(unitToBeRemovedDir, 0755)
	require.NoError(t, err)

	err = os.MkdirAll(unitToBeUntouchedDir, 0755)
	require.NoError(t, err)

	unitToBeModifiedHCLPath := filepath.Join(unitToBeModifiedDir, "terragrunt.hcl")
	err = os.WriteFile(unitToBeModifiedHCLPath, []byte(`# Unit to be modified`), 0644)
	require.NoError(t, err)

	unitToBeRemovedHCLPath := filepath.Join(unitToBeRemovedDir, "terragrunt.hcl")
	err = os.WriteFile(unitToBeRemovedHCLPath, []byte(`# Unit to be removed`), 0644)
	require.NoError(t, err)

	unitToBeUntouchedHCLPath := filepath.Join(unitToBeUntouchedDir, "terragrunt.hcl")
	err = os.WriteFile(unitToBeUntouchedHCLPath, []byte(`# Unit to be untouched`), 0644)
	require.NoError(t, err)

	// Initial commit
	gitAdd(t, tmpDir, ".")

	gitCommit(t, tmpDir, "Initial commit")

	// Modify the unit to be modified
	err = os.WriteFile(unitToBeModifiedHCLPath, []byte(`# Unit modified`), 0644)
	require.NoError(t, err)

	// Remove the unit to be removed (delete the directory)
	err = os.RemoveAll(unitToBeRemovedDir)
	require.NoError(t, err)

	// Add a unit to be created
	unitToBeCreatedDir := filepath.Join(tmpDir, "unit-to-be-created")
	err = os.MkdirAll(unitToBeCreatedDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitToBeCreatedDir, "terragrunt.hcl"), []byte(`# Unit created`), 0644)
	require.NoError(t, err)

	// Do nothing to the unit to be untouched

	// Commit the modification and removal in a single commit
	gitAdd(t, tmpDir, ".")
	gitCommit(t, tmpDir, "Create, modify, and remove units")

	// Create options
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Create a Git filter expression
	gitFilter := filter.NewGitFilter("HEAD~1", "HEAD")
	gitExpressions := filter.GitFilters{gitFilter}

	// Create original discovery
	originalDiscovery := discovery.NewDiscovery(tmpDir)

	// Create worktree discovery
	worktreeDiscovery := discovery.NewWorktreeDiscovery(
		gitExpressions,
	).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
		}).
		WithOriginalDiscovery(originalDiscovery)

	// Perform discovery
	l := logger.CreateLogger()
	components, worktrees, err := worktreeDiscovery.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Verify worktrees were created for both refs
	assert.NotEmpty(t, worktrees, "Worktrees should be created")
	assert.Contains(t, worktrees, "HEAD~1", "Worktree should exist for initial commit")
	assert.Contains(t, worktrees, "HEAD", "Worktree should exist for current commit")

	// Verify units were discovered
	units := components.Filter(component.UnitKind)
	unitPaths := units.Paths()

	fromWorktree := worktrees["HEAD~1"]
	toWorktree := worktrees["HEAD"]

	expectedUnitToBeCreated := filepath.Join(toWorktree, "unit-to-be-created")
	expectedUnitToBeModified := filepath.Join(toWorktree, "unit-to-be-modified")
	expectedUnitToBeRemoved := filepath.Join(fromWorktree, "unit-to-be-removed")
	expectedUnitToBeUntouched := filepath.Join(toWorktree, "unit-to-be-untouched")

	assert.Contains(t, unitPaths, expectedUnitToBeCreated, "Unit should be discovered as it was created between commits")
	assert.DirExists(t, expectedUnitToBeCreated)

	assert.Contains(t, unitPaths, expectedUnitToBeModified, "Unit should be discovered as it was modified between commits")
	assert.DirExists(t, expectedUnitToBeModified)

	assert.Contains(t, unitPaths, expectedUnitToBeRemoved, "Unit should be discovered as it was removed between commits")
	assert.DirExists(t, expectedUnitToBeRemoved)

	assert.NotContains(t, unitPaths, expectedUnitToBeUntouched, "Unit should not be discovered as it was untouched between commits")
	assert.DirExists(t, expectedUnitToBeUntouched)
}
