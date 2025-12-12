package discovery_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorktreeDiscovery(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

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
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

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
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Create, modify, and remove units", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create options
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Create a Git filter expression
	gitFilter := filter.NewGitExpression("HEAD~1", "HEAD")
	gitExpressions := filter.GitExpressions{gitFilter}

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
	assert.NotEmpty(t, w.WorktreePairs, "Worktrees should be created")
	assert.Contains(t, w.WorktreePairs, "[HEAD~1...HEAD]", "Worktree should exist for initial commit")

	// Verify units were discovered
	units := components.Filter(component.UnitKind)
	unitPaths := units.Paths()

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	require.NotEmpty(t, worktreePair)

	fromWorktree := worktreePair.FromWorktree.Path
	toWorktree := worktreePair.ToWorktree.Path

	// All paths are worktree paths - no translation needed
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

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	t.Cleanup(func() {
		err = runner.GoCloseStorage()
		require.NoError(t, err)
	})

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
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

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
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Create, modify, and remove units", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create options
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Create a Git filter expression
	gitFilter := filter.NewGitExpression("HEAD~1", "HEAD")
	gitExpressions := filter.GitExpressions{gitFilter}

	l := logger.CreateLogger()

	tests := []struct {
		discoveryContext *component.DiscoveryContext
		name             string
		expectedErrorMsg string
		description      string
		expectError      bool
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
			assert.NotEmpty(t, w.WorktreePairs, "Worktrees should be created")
			assert.Contains(t, w.WorktreePairs, "[HEAD~1...HEAD]", "Worktree should exist for initial commit")

			worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
			require.NotEmpty(t, worktreePair)

			fromWorktree := worktreePair.FromWorktree.Path
			toWorktree := worktreePair.ToWorktree.Path

			// Verify units were discovered
			units := components.Filter(component.UnitKind)
			unitPaths := units.Paths()

			// All paths are worktree paths - no translation needed
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

func TestWorktreeDiscovery_EmptyFilters(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	// Create initial commit with no terragrunt.hcl files
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create a second commit that only changes non-terragrunt.hcl files
	readmePath := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(readmePath, []byte("# Test"), 0644)
	require.NoError(t, err)

	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Update README", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create options
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Create a Git filter expression
	gitFilter := filter.NewGitExpression("HEAD~1", "HEAD")
	gitExpressions := filter.GitExpressions{gitFilter}

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

	// Verify that no components were discovered when filters are empty
	// (because the diffs don't contain any terragrunt.hcl files)
	assert.Empty(t, components, "No components should be discovered when Git filter expands to empty filters")
}

func TestWorktreeDiscovery_EmptyDiffs(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	// Create initial commit
	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create a second commit with no changes (empty commit)
	err = runner.GoCommit("Empty commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create options
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Create a Git filter expression
	gitFilter := filter.NewGitExpression("HEAD~1", "HEAD")
	gitExpressions := filter.GitExpressions{gitFilter}

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

	// Verify that no components were discovered when there are no diffs
	assert.Empty(t, components, "No components should be discovered when there are no diffs between references")
}

func TestWorktreeDiscovery_Stacks(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	// Create a catalog of units

	// The legacy unit is the one that will be migrated from.
	legacyUnitDir := filepath.Join(tmpDir, "catalog", "units", "legacy")
	err = os.MkdirAll(legacyUnitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "terragrunt.hcl"), []byte(`# Legacy unit`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0644)
	require.NoError(t, err)

	// The modern unit is one that will be migrated to from the legacy unit.
	modernUnitDir := filepath.Join(tmpDir, "catalog", "units", "modern")
	err = os.MkdirAll(modernUnitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(modernUnitDir, "terragrunt.hcl"), []byte(`# Modern unit`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(modernUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0644)
	require.NoError(t, err)

	// These let us simulate editing a stack file to use new unit definitions.

	// Commit creation of the foo unit
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Create foo unit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create three stacks that reference the active and passive units, three times.
	//
	// Afterwards we'll:
	//
	// - add a new stack.
	// - modify the first stack to perform an addition, removal, modification and no-op of units.
	// - remove the second stack.
	// - leave the third stack untouched.

	stackFileContents := `unit "unit_to_be_modified" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit_to_be_modified"
}

unit "unit_to_be_removed" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit_to_be_removed"
}

unit "unit_to_be_untouched" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit_to_be_untouched"
}
`

	stackToBeModifiedDir := filepath.Join(tmpDir, "live", "stack-to-be-modified")
	err = os.MkdirAll(stackToBeModifiedDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0644)
	require.NoError(t, err)

	stackToBeRemovedDir := filepath.Join(tmpDir, "live", "stack-to-be-removed")
	err = os.MkdirAll(stackToBeRemovedDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeRemovedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0644)
	require.NoError(t, err)

	stackToBeUntouchedDir := filepath.Join(tmpDir, "live", "stack-to-be-untouched")
	err = os.MkdirAll(stackToBeUntouchedDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeUntouchedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0644)
	require.NoError(t, err)

	// Commit creation of the stacks
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Create stacks", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Add a new stack
	stackToBeAddedDir := filepath.Join(tmpDir, "live", "stack-to-be-added")
	err = os.MkdirAll(stackToBeAddedDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeAddedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0644)
	require.NoError(t, err)

	// Modify the first stack to switch from the legacy unit to the modern unit
	err = os.WriteFile(filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"), []byte(`unit "unit_to_be_added" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit_to_be_added"
}

unit "unit_to_be_modified" {
	source = "${get_repo_root()}/catalog/units/modern"
	path   = "unit_to_be_modified"
}

unit "unit_to_be_untouched" {
	source = "${get_repo_root()}/catalog/units/legacy"
	path   = "unit_to_be_untouched"
}
`), 0644)
	require.NoError(t, err)

	// Remove the second stack
	err = os.RemoveAll(stackToBeRemovedDir)
	require.NoError(t, err)

	// Leave the third stack untouched

	// Commit the changes
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Modify and remove stacks", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create options
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir
	opts.FilterQueries = []string{"[HEAD~1...HEAD]"}
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.FilterFlag)
	require.NoError(t, err)

	// Create a Git filter expression
	gitFilter := filter.NewGitExpression("HEAD~1", "HEAD")
	gitExpressions := filter.GitExpressions{gitFilter}

	l := logger.CreateLogger()

	w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, gitExpressions)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.Background(), l)
		require.NoError(t, cleanupErr)
	})

	// Generate stacks using the test's worktrees
	err = generate.GenerateStacks(t.Context(), l, opts, w)
	require.NoError(t, err)

	// Create original discovery
	originalDiscovery := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
			Cmd:        "plan",
		}).
		WithWorktrees(w)

	// Create worktree discovery
	worktreeDiscovery := discovery.NewWorktreeDiscovery(
		gitExpressions,
	).
		WithOriginalDiscovery(originalDiscovery)

	// Perform discovery
	components, err := worktreeDiscovery.Discover(t.Context(), l, opts, w)
	require.NoError(t, err)

	// Verify that the stacks were discovered
	assert.NotEmpty(t, components)
	assert.Len(t, components, 12)

	// Get relative paths from tmpDir
	stackToBeAddedRel, err := filepath.Rel(tmpDir, stackToBeAddedDir)
	require.NoError(t, err)
	stackToBeModifiedRel, err := filepath.Rel(tmpDir, stackToBeModifiedDir)
	require.NoError(t, err)
	stackToBeRemovedRel, err := filepath.Rel(tmpDir, stackToBeRemovedDir)
	require.NoError(t, err)

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	require.NotEmpty(t, worktreePair)

	fromWorktree := worktreePair.FromWorktree.Path
	toWorktree := worktreePair.ToWorktree.Path

	// All paths are worktree paths - no translation needed
	assert.ElementsMatch(t, components, component.Components{
		// Stacks
		component.NewStack(filepath.Join(toWorktree, stackToBeAddedRel)).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: toWorktree,
				Ref:        "HEAD",
				Cmd:        "plan",
			},
		),
		component.NewStack(filepath.Join(toWorktree, stackToBeModifiedRel)).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: toWorktree,
				Ref:        "HEAD",
				Cmd:        "plan",
			},
		),
		component.NewStack(filepath.Join(fromWorktree, stackToBeRemovedRel)).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: fromWorktree,
				Ref:        "HEAD~1",
				Cmd:        "plan",
				Args:       []string{"-destroy"},
			},
		),
		// Units from stack-to-be-added (HEAD) - worktree paths
		component.NewUnit(filepath.Join(toWorktree, stackToBeAddedRel, ".terragrunt-stack", "unit_to_be_modified")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: toWorktree,
				Ref:        "HEAD",
				Cmd:        "plan",
			},
		),
		component.NewUnit(filepath.Join(toWorktree, stackToBeAddedRel, ".terragrunt-stack", "unit_to_be_removed")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: toWorktree,
				Ref:        "HEAD",
				Cmd:        "plan",
			},
		),
		component.NewUnit(filepath.Join(toWorktree, stackToBeAddedRel, ".terragrunt-stack", "unit_to_be_untouched")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: toWorktree,
				Ref:        "HEAD",
				Cmd:        "plan",
			},
		),
		// Units from stack-to-be-modified (HEAD) - worktree paths
		// For changed stacks, we only discover units that are added, removed, or changed (different SHA)
		// unit_to_be_added: only in HEAD (added)
		component.NewUnit(filepath.Join(toWorktree, stackToBeModifiedRel, ".terragrunt-stack", "unit_to_be_added")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: toWorktree,
				Ref:        "HEAD",
				Cmd:        "plan",
			},
		),
		// unit_to_be_modified: in both but changed (legacy -> modern), so we use HEAD version
		component.NewUnit(filepath.Join(toWorktree, stackToBeModifiedRel, ".terragrunt-stack", "unit_to_be_modified")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: toWorktree,
				Ref:        "HEAD",
				Cmd:        "plan",
			},
		),
		// Units from stack-to-be-modified (HEAD~1) - fromWorktree paths
		// unit_to_be_removed: only in HEAD~1 (removed)
		component.NewUnit(filepath.Join(fromWorktree, stackToBeModifiedRel, ".terragrunt-stack", "unit_to_be_removed")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: fromWorktree,
				Ref:        "HEAD~1",
				Cmd:        "plan",
				Args:       []string{"-destroy"},
			},
		),
		// Units from stack-to-be-removed (HEAD~1) - fromWorktree paths
		component.NewUnit(filepath.Join(fromWorktree, stackToBeRemovedRel, ".terragrunt-stack", "unit_to_be_modified")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: fromWorktree,
				Ref:        "HEAD~1",
				Cmd:        "plan",
				Args:       []string{"-destroy"},
			},
		),
		component.NewUnit(filepath.Join(fromWorktree, stackToBeRemovedRel, ".terragrunt-stack", "unit_to_be_removed")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: fromWorktree,
				Ref:        "HEAD~1",
				Cmd:        "plan",
				Args:       []string{"-destroy"},
			},
		),
		component.NewUnit(filepath.Join(fromWorktree, stackToBeRemovedRel, ".terragrunt-stack", "unit_to_be_untouched")).WithDiscoveryContext(
			&component.DiscoveryContext{
				WorkingDir: fromWorktree,
				Ref:        "HEAD~1",
				Cmd:        "plan",
				Args:       []string{"-destroy"},
			},
		),
	})
}

// TestWorktreeDiscoveryDetectsFileRename verifies that renaming a file within a unit
// (without changing its content) is detected as a change by the worktree discovery.
// This tests that the SHA256 computation includes file paths, not just content.
func TestWorktreeDiscoveryDetectsFileRename(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	// Create a unit with a file
	unitDir := filepath.Join(tmpDir, "unit")
	err = os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# Unit config`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "original.tf"), []byte(`# Same content before and after rename`), 0644)
	require.NoError(t, err)

	// Commit
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit with original.tf", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Rename the file (same content, different name)
	err = os.Rename(
		filepath.Join(unitDir, "original.tf"),
		filepath.Join(unitDir, "renamed.tf"),
	)
	require.NoError(t, err)

	// Commit the rename
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Rename original.tf to renamed.tf", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Set up worktree discovery
	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir

	gitExpressions := filter.GitExpressions{
		&filter.GitExpression{
			FromRef: "HEAD~1",
			ToRef:   "HEAD",
		},
	}

	w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, gitExpressions)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.Background(), l))
	})

	originalDiscovery := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
			Cmd:        "plan",
		}).
		WithWorktrees(w)

	worktreeDiscovery := discovery.NewWorktreeDiscovery(gitExpressions).
		WithOriginalDiscovery(originalDiscovery)

	components, err := worktreeDiscovery.Discover(t.Context(), l, opts, w)
	require.NoError(t, err)

	// The unit should be detected as changed because the file was renamed
	// (even though content is identical)
	assert.NotEmpty(t, components, "unit with renamed file should be detected as changed")

	// Verify we have the unit from HEAD (the version with renamed.tf)
	foundUnit := false

	for _, c := range components {
		if _, ok := c.(*component.Unit); ok {
			foundUnit = true
			break
		}
	}

	assert.True(t, foundUnit, "should discover the unit with renamed file")
}

// TestWorktreeDiscoveryDetectsFileMove verifies that moving a file to a subdirectory
// within a unit (without changing its content) is detected as a change.
func TestWorktreeDiscoveryDetectsFileMove(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	// Create a unit with a file in root
	unitDir := filepath.Join(tmpDir, "unit")
	err = os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# Unit config`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "module.tf"), []byte(`# Module content`), 0644)
	require.NoError(t, err)

	// Commit
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Initial commit with module.tf in root", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Move file to subdirectory (same content, different path)
	subDir := filepath.Join(unitDir, "modules")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	err = os.Rename(
		filepath.Join(unitDir, "module.tf"),
		filepath.Join(subDir, "module.tf"),
	)
	require.NoError(t, err)

	// Commit the move
	err = runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit("Move module.tf to modules/ subdirectory", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Set up worktree discovery
	l := logger.CreateLogger()
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir

	gitExpressions := filter.GitExpressions{
		&filter.GitExpression{
			FromRef: "HEAD~1",
			ToRef:   "HEAD",
		},
	}

	w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, gitExpressions)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.Background(), l))
	})

	originalDiscovery := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
			Cmd:        "plan",
		}).
		WithWorktrees(w)

	worktreeDiscovery := discovery.NewWorktreeDiscovery(gitExpressions).
		WithOriginalDiscovery(originalDiscovery)

	components, err := worktreeDiscovery.Discover(t.Context(), l, opts, w)
	require.NoError(t, err)

	// The unit should be detected as changed because the file was moved
	// (even though content is identical)
	assert.NotEmpty(t, components, "unit with moved file should be detected as changed")

	// Verify we have the unit from HEAD
	foundUnit := false

	for _, c := range components {
		if _, ok := c.(*component.Unit); ok {
			foundUnit = true
			break
		}
	}

	assert.True(t, foundUnit, "should discover the unit with moved file")
}
