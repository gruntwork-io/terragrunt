package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
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
	originalDiscovery := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
		})

	// Create worktree discovery
	worktreeDiscovery := discovery.NewWorktreeDiscovery(
		gitExpressions,
	).
		WithOriginalDiscovery(originalDiscovery)

	// Perform discovery
	l := logger.CreateLogger()
	w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, gitExpressions)
	require.NoError(t, err)

	components, err := worktreeDiscovery.Discover(t.Context(), l, opts, w)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.Background(), l)
		require.NoError(t, cleanupErr)
	})

	// Verify worktrees were created for both refs
	assert.NotEmpty(t, w.RefsToPaths, "Worktrees should be created")
	assert.Contains(t, w.RefsToPaths, "HEAD~1", "Worktree should exist for initial commit")
	assert.Contains(t, w.RefsToPaths, "HEAD", "Worktree should exist for current commit")

	// Verify units were discovered
	units := components.Filter(component.UnitKind)
	unitPaths := units.Paths()

	fromWorktree := w.RefsToPaths["HEAD~1"]
	toWorktree := w.RefsToPaths["HEAD"]

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

func TestWorktreeDiscoveryContextCommandArgsUpdate(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	helpers.CreateGitRepo(t, tmpDir)

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

	l := logger.CreateLogger()

	tests := []struct {
		name             string
		discoveryContext *component.DiscoveryContext
		expectError      bool
		expectedErrorMsg string
		description      string
	}{
		{
			name: "plan_command_removed_unit_has_destroy_flag",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "plan",
				Args:       []string{},
			},
			expectError: false,
			description: "Plan command should add '-destroy' flag for removed units (from worktree)",
		},
		{
			name: "apply_command_removed_unit_has_destroy_flag",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "apply",
				Args:       []string{},
			},
			expectError: false,
			description: "Apply command should add '-destroy' flag for removed units (from worktree)",
		},
		{
			name: "plan_command_added_unit_no_destroy_flag",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "plan",
				Args:       []string{},
			},
			expectError: false,
			description: "Plan command should NOT add '-destroy' flag for added units (to worktree)",
		},
		{
			name: "apply_command_added_unit_no_destroy_flag",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "apply",
				Args:       []string{},
			},
			expectError: false,
			description: "Apply command should NOT add '-destroy' flag for added units (to worktree)",
		},
		{
			name: "plan_command_modified_unit_no_destroy_flag",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "plan",
				Args:       []string{},
			},
			expectError: false,
			description: "Plan command should NOT add '-destroy' flag for modified units (to worktree)",
		},
		{
			name: "apply_command_modified_unit_no_destroy_flag",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "apply",
				Args:       []string{},
			},
			expectError: false,
			description: "Apply command should NOT add '-destroy' flag for modified units (to worktree)",
		},
		{
			name: "plan_command_with_destroy_throws_error",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "plan",
				Args:       []string{"-destroy"},
			},
			expectError: true,
			description: "Plan command with '-destroy' already present should throw an error, as it's ambiguous whether to destroy or plan",
		},
		{
			name: "empty_command_allowed",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "",
				Args:       []string{},
			},
			expectError: false,
			description: "Empty command and args should be allowed (discovery commands like find/list)",
		},
		{
			name: "unsupported_command_returns_error",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "destroy",
				Args:       []string{},
			},
			expectError:      true,
			expectedErrorMsg: "Git-based filtering is not supported with the command 'destroy'",
			description:      "Unsupported command should return error",
		},
		{
			name: "plan_with_other_arbitrary_args_allowed",
			discoveryContext: &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        "plan",
				Args:       []string{"-out", "plan.out"},
			},
			expectError: false,
			description: "Plan command with other arbitrary args should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.name != "empty_command_allowed" {
				t.Skip("Empty command and args are allowed for discovery commands like find/list")
			}

			w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, gitExpressions)
			require.NoError(t, err)

			originalDiscovery := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(tt.discoveryContext).
				WithWorktrees(w)

			// Create worktree discovery with the test discovery context
			worktreeDiscovery := discovery.NewWorktreeDiscovery(
				gitExpressions,
			).
				WithOriginalDiscovery(originalDiscovery)

			t.Cleanup(func() {
				cleanupErr := w.Cleanup(context.Background(), l)
				require.NoError(t, cleanupErr)
			})

			// Perform discovery
			components, err := worktreeDiscovery.Discover(t.Context(), l, opts, w)

			if tt.expectError {
				require.Error(t, err, "Expected error for: %s", tt.description)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg, "Error message should contain expected text")
				}
				return
			}

			require.NoError(t, err, "Should not error for: %s", tt.description)

			// Verify worktrees were created
			assert.NotEmpty(t, w.RefsToPaths, "Worktrees should be created")
			assert.Contains(t, w.RefsToPaths, "HEAD~1", "Worktree should exist for initial commit")
			assert.Contains(t, w.RefsToPaths, "HEAD", "Worktree should exist for current commit")

			fromWorktree := w.RefsToPaths["HEAD~1"]
			toWorktree := w.RefsToPaths["HEAD"]

			// Verify units were discovered
			units := components.Filter(component.UnitKind)
			unitPaths := units.Paths()

			expectedUnitToBeCreated := filepath.Join(toWorktree, "unit-to-be-created")
			expectedUnitToBeModified := filepath.Join(toWorktree, "unit-to-be-modified")
			expectedUnitToBeRemoved := filepath.Join(fromWorktree, "unit-to-be-removed")

			// Find components by path and verify their discovery context args
			for _, unit := range units {
				ctx := unit.DiscoveryContext()
				require.NotNil(t, ctx, "Component should have discovery context")

				unitPath := unit.Path()

				// Check removed unit (discovered in "from" worktree)
				if unitPath == expectedUnitToBeRemoved {
					if tt.discoveryContext.Cmd == "plan" || tt.discoveryContext.Cmd == "apply" {
						// Removed units should have -destroy flag added
						assert.Contains(t, ctx.Args, "-destroy",
							"Removed unit discovered in 'from' worktree should have '-destroy' flag for %s command", tt.discoveryContext.Cmd)
					}
				}

				// Check added unit (discovered in "to" worktree)
				if unitPath == expectedUnitToBeCreated {
					if tt.discoveryContext.Cmd == "plan" || tt.discoveryContext.Cmd == "apply" {
						// Added units should NOT have -destroy flag
						assert.NotContains(t, ctx.Args, "-destroy",
							"Added unit discovered in 'to' worktree should NOT have '-destroy' flag for %s command", tt.discoveryContext.Cmd)
					}
				}

				// Check modified unit (discovered in "to" worktree)
				if unitPath == expectedUnitToBeModified {
					if tt.discoveryContext.Cmd == "plan" || tt.discoveryContext.Cmd == "apply" {
						// Modified units should NOT have -destroy flag
						assert.NotContains(t, ctx.Args, "-destroy",
							"Modified unit discovered in 'to' worktree should NOT have '-destroy' flag for %s command", tt.discoveryContext.Cmd)
					}
				}
			}

			// Verify expected units were discovered
			assert.Contains(t, unitPaths, expectedUnitToBeCreated, "Unit should be discovered as it was created between commits")
			assert.Contains(t, unitPaths, expectedUnitToBeModified, "Unit should be discovered as it was modified between commits")
			assert.Contains(t, unitPaths, expectedUnitToBeRemoved, "Unit should be discovered as it was removed between commits")
		})
	}
}
