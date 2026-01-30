package v2_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/gruntwork-io/terragrunt/internal/component"
	v2 "github.com/gruntwork-io/terragrunt/internal/discovery/v2"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/stacks/generate"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupGitRepo creates a git repository with initial structure for integration tests.
func setupGitRepo(t *testing.T) (string, *git.GitRunner) {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	t.Cleanup(func() {
		if err := runner.GoCloseStorage(); err != nil {
			t.Logf("Error closing storage: %s", err)
		}
	})

	return tmpDir, runner
}

// commitChanges stages all changes and commits with the given message.
func commitChanges(t *testing.T, runner *git.GitRunner, message string) {
	t.Helper()

	err := runner.GoAdd(".")
	require.NoError(t, err)

	err = runner.GoCommit(message, &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)
}

// createUnit creates a unit directory with terragrunt.hcl.
func createUnit(t *testing.T, baseDir, unitName, content string) string {
	t.Helper()

	unitDir := filepath.Join(baseDir, unitName)
	err := os.MkdirAll(unitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(content), 0644)
	require.NoError(t, err)

	return unitDir
}

// runWorktreeDiscovery runs v2 discovery with worktree phase enabled.
func runWorktreeDiscovery(
	t *testing.T,
	tmpDir string,
	gitExpressions filter.GitExpressions,
	cmd string,
	args []string,
) (component.Components, *worktrees.Worktrees) {
	t.Helper()

	l := logger.CreateLogger()

	w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, gitExpressions)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	discoveryContext := &component.DiscoveryContext{
		WorkingDir: tmpDir,
		Cmd:        cmd,
		Args:       args,
	}

	// Build filters from git expressions
	filters := make(filter.Filters, 0, len(gitExpressions))
	for _, gitExpr := range gitExpressions {
		f := filter.NewFilter(gitExpr, gitExpr.String())
		filters = append(filters, f)
	}

	discovery := v2.New(tmpDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w).
		WithFilters(filters)

	components, err := discovery.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	return components, w
}

// TestWorktreePhase_Integration_UnitLifecycle tests the full worktree discovery flow
// for created, modified, removed, and untouched units.
func TestWorktreePhase_Integration_UnitLifecycle(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create initial units
	createUnit(t, tmpDir, "unit-to-be-modified", `# Unit to be modified`)
	createUnit(t, tmpDir, "unit-to-be-removed", `# Unit to be removed`)
	createUnit(t, tmpDir, "unit-to-be-untouched", `# Unit to be untouched`)

	commitChanges(t, runner, "Initial commit")

	// Modify the unit
	err := os.WriteFile(filepath.Join(tmpDir, "unit-to-be-modified", "terragrunt.hcl"), []byte(`# Unit modified`), 0644)
	require.NoError(t, err)

	// Remove the unit
	err = os.RemoveAll(filepath.Join(tmpDir, "unit-to-be-removed"))
	require.NoError(t, err)

	// Add a new unit
	createUnit(t, tmpDir, "unit-to-be-created", `# Unit created`)

	// Do nothing to the untouched unit

	commitChanges(t, runner, "Create, modify, and remove units")

	// Run worktree discovery
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}
	components, w := runWorktreeDiscovery(t, tmpDir, gitExpressions, "", nil)

	// Verify worktrees were created
	assert.NotEmpty(t, w.WorktreePairs, "Worktrees should be created")
	assert.Contains(t, w.WorktreePairs, "[HEAD~1...HEAD]", "Worktree should exist")

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	fromWorktree := worktreePair.FromWorktree.Path
	toWorktree := worktreePair.ToWorktree.Path

	// Verify units were discovered
	units := components.Filter(component.UnitKind)
	unitPaths := units.Paths()

	expectedUnitToBeCreated := filepath.Join(toWorktree, "unit-to-be-created")
	expectedUnitToBeModified := filepath.Join(toWorktree, "unit-to-be-modified")
	expectedUnitToBeRemoved := filepath.Join(fromWorktree, "unit-to-be-removed")
	expectedUnitToBeUntouched := filepath.Join(toWorktree, "unit-to-be-untouched")

	assert.Contains(t, unitPaths, expectedUnitToBeCreated, "Unit should be discovered as it was created")
	assert.DirExists(t, expectedUnitToBeCreated)

	assert.Contains(t, unitPaths, expectedUnitToBeModified, "Unit should be discovered as it was modified")
	assert.DirExists(t, expectedUnitToBeModified)

	assert.Contains(t, unitPaths, expectedUnitToBeRemoved, "Unit should be discovered as it was removed")
	assert.DirExists(t, expectedUnitToBeRemoved)

	assert.NotContains(t, unitPaths, expectedUnitToBeUntouched, "Unit should not be discovered as it was untouched")
	assert.DirExists(t, expectedUnitToBeUntouched)
}

// TestWorktreePhase_Integration_CommandArgs tests command argument handling for worktrees.
func TestWorktreePhase_Integration_CommandArgs(t *testing.T) {
	t.Parallel()

	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	tests := []struct {
		name             string
		cmd              string
		expectedErrorMsg string
		description      string
		args             []string
		expectError      bool
	}{
		{
			name:        "plan_command_removed_unit_has_destroy_flag",
			cmd:         "plan",
			args:        []string{},
			expectError: false,
			description: "Plan command should add '-destroy' flag for removed units",
		},
		{
			name:        "apply_command_removed_unit_has_destroy_flag",
			cmd:         "apply",
			args:        []string{},
			expectError: false,
			description: "Apply command should add '-destroy' flag for removed units",
		},
		{
			name:        "plan_command_with_destroy_throws_error",
			cmd:         "plan",
			args:        []string{"-destroy"},
			expectError: true,
			description: "Plan command with '-destroy' already present should error",
		},
		{
			name:        "empty_command_allowed",
			cmd:         "",
			args:        []string{},
			expectError: false,
			description: "Empty command should be allowed for discovery commands",
		},
		{
			name:             "unsupported_command_returns_error",
			cmd:              "destroy",
			args:             []string{},
			expectError:      true,
			expectedErrorMsg: "Git-based filtering is not supported with the command 'destroy'",
			description:      "Unsupported command should return error",
		},
		{
			name:        "plan_with_other_args_allowed",
			cmd:         "plan",
			args:        []string{"-out", "plan.out"},
			expectError: false,
			description: "Plan command with other args should be allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Each subtest creates its own git repository
			tmpDir, runner := setupGitRepo(t)

			// Create initial units
			createUnit(t, tmpDir, "unit-to-be-modified", `# Unit to be modified`)
			createUnit(t, tmpDir, "unit-to-be-removed", `# Unit to be removed`)

			commitChanges(t, runner, "Initial commit")

			// Modify the unit
			err := os.WriteFile(filepath.Join(tmpDir, "unit-to-be-modified", "terragrunt.hcl"), []byte(`# Modified`), 0644)
			require.NoError(t, err)

			// Remove the unit
			err = os.RemoveAll(filepath.Join(tmpDir, "unit-to-be-removed"))
			require.NoError(t, err)

			// Add a new unit
			createUnit(t, tmpDir, "unit-to-be-created", `# Created`)

			commitChanges(t, runner, "Update units")

			// Set up discovery
			l := logger.CreateLogger()

			w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, gitExpressions)
			require.NoError(t, err)

			t.Cleanup(func() {
				cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
				require.NoError(t, cleanupErr)
			})

			opts := options.NewTerragruntOptions()
			opts.WorkingDir = tmpDir
			opts.RootWorkingDir = tmpDir

			discoveryContext := &component.DiscoveryContext{
				WorkingDir: tmpDir,
				Cmd:        tt.cmd,
				Args:       tt.args,
			}

			discovery := v2.New(tmpDir).
				WithDiscoveryContext(discoveryContext).
				WithWorktrees(w)

			filters := filter.Filters{}

			for _, gitExpr := range gitExpressions {
				f := filter.NewFilter(gitExpr, gitExpr.String())
				filters = append(filters, f)
			}

			discovery = discovery.WithFilters(filters)

			components, err := discovery.Discover(t.Context(), l, opts)

			if tt.expectError {
				require.Error(t, err, "Expected error for: %s", tt.description)

				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}

				return
			}

			require.NoError(t, err, "Should not error for: %s", tt.description)

			// Verify worktrees were created
			assert.NotEmpty(t, w.WorktreePairs, "Worktrees should be created")

			worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
			fromWorktree := worktreePair.FromWorktree.Path
			toWorktree := worktreePair.ToWorktree.Path

			// Verify units were discovered
			units := components.Filter(component.UnitKind)

			expectedUnitToBeCreated := filepath.Join(toWorktree, "unit-to-be-created")
			expectedUnitToBeModified := filepath.Join(toWorktree, "unit-to-be-modified")
			expectedUnitToBeRemoved := filepath.Join(fromWorktree, "unit-to-be-removed")

			// Verify discovery context args for each unit
			for _, unit := range units {
				ctx := unit.DiscoveryContext()
				require.NotNil(t, ctx, "Component should have discovery context")

				unitPath := unit.Path()

				// Check removed unit (discovered in "from" worktree)
				if unitPath == expectedUnitToBeRemoved {
					if tt.cmd == "plan" || tt.cmd == "apply" {
						assert.Contains(t, ctx.Args, "-destroy",
							"Removed unit should have '-destroy' flag for %s command", tt.cmd)
					}
				}

				// Check added unit (discovered in "to" worktree)
				if unitPath == expectedUnitToBeCreated {
					if tt.cmd == "plan" || tt.cmd == "apply" {
						assert.NotContains(t, ctx.Args, "-destroy",
							"Added unit should NOT have '-destroy' flag for %s command", tt.cmd)
					}
				}

				// Check modified unit (discovered in "to" worktree)
				if unitPath == expectedUnitToBeModified {
					if tt.cmd == "plan" || tt.cmd == "apply" {
						assert.NotContains(t, ctx.Args, "-destroy",
							"Modified unit should NOT have '-destroy' flag for %s command", tt.cmd)
					}
				}
			}
		})
	}
}

// TestWorktreePhase_Integration_EmptyFilters tests that discovery produces no results
// when git diff contains no terragrunt files.
func TestWorktreePhase_Integration_EmptyFilters(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create initial empty commit
	err := runner.GoCommit("Initial commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create a second commit with only non-terragrunt files
	readmePath := filepath.Join(tmpDir, "README.md")
	err = os.WriteFile(readmePath, []byte("# Test"), 0644)
	require.NoError(t, err)

	commitChanges(t, runner, "Update README")

	// Run worktree discovery
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}
	components, _ := runWorktreeDiscovery(t, tmpDir, gitExpressions, "", nil)

	// Verify that no components were discovered
	assert.Empty(t, components, "No components should be discovered when filters are empty")
}

// TestWorktreePhase_Integration_EmptyDiffs tests that discovery produces no results
// when there are no changes between commits.
func TestWorktreePhase_Integration_EmptyDiffs(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create initial empty commit
	err := runner.GoCommit("Initial commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Create a second empty commit
	err = runner.GoCommit("Empty commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	// Run worktree discovery
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}
	components, _ := runWorktreeDiscovery(t, tmpDir, gitExpressions, "", nil)

	// Verify that no components were discovered
	assert.Empty(t, components, "No components should be discovered when there are no diffs")
}

// TestWorktreePhase_Integration_Stacks tests stack discovery with generated units.
func TestWorktreePhase_Integration_Stacks(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a catalog of units
	legacyUnitDir := filepath.Join(tmpDir, "catalog", "units", "legacy")
	err := os.MkdirAll(legacyUnitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "terragrunt.hcl"), []byte(`# Legacy unit`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0644)
	require.NoError(t, err)

	modernUnitDir := filepath.Join(tmpDir, "catalog", "units", "modern")
	err = os.MkdirAll(modernUnitDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(modernUnitDir, "terragrunt.hcl"), []byte(`# Modern unit`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(modernUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create catalog units")

	// Create stacks
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

	commitChanges(t, runner, "Create stacks")

	// Add a new stack
	stackToBeAddedDir := filepath.Join(tmpDir, "live", "stack-to-be-added")
	err = os.MkdirAll(stackToBeAddedDir, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeAddedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0644)
	require.NoError(t, err)

	// Modify the first stack
	modifiedStackContents := `unit "unit_to_be_added" {
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
`
	err = os.WriteFile(filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"), []byte(modifiedStackContents), 0644)
	require.NoError(t, err)

	// Remove the second stack
	err = os.RemoveAll(stackToBeRemovedDir)
	require.NoError(t, err)

	commitChanges(t, runner, "Modify and remove stacks")

	// Set up discovery with worktrees
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, gitExpressions)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	// Generate stacks in worktrees
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir
	opts.FilterQueries = []string{"[HEAD~1...HEAD]"}
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.FilterFlag)
	require.NoError(t, err)

	err = generate.GenerateStacks(t.Context(), l, opts, w)
	require.NoError(t, err)

	// Run discovery
	discoveryContext := &component.DiscoveryContext{
		WorkingDir: tmpDir,
		Cmd:        "plan",
	}

	discovery := v2.New(tmpDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w)

	filters := filter.Filters{}

	for _, gitExpr := range gitExpressions {
		f := filter.NewFilter(gitExpr, gitExpr.String())
		filters = append(filters, f)
	}

	discovery = discovery.WithFilters(filters)

	components, err := discovery.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Verify that components were discovered
	assert.NotEmpty(t, components)

	// Get worktree paths
	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	require.NotEmpty(t, worktreePair)

	fromWorktree := worktreePair.FromWorktree.Path
	toWorktree := worktreePair.ToWorktree.Path

	// Get relative paths
	stackToBeAddedRel, err := filepath.Rel(tmpDir, stackToBeAddedDir)
	require.NoError(t, err)
	stackToBeRemovedRel, err := filepath.Rel(tmpDir, stackToBeRemovedDir)
	require.NoError(t, err)

	// Verify added stack and its units are in toWorktree
	addedStackPath := filepath.Join(toWorktree, stackToBeAddedRel)
	foundAddedStack := false

	for _, c := range components {
		if c.Path() == addedStackPath {
			foundAddedStack = true
			dc := c.DiscoveryContext()
			assert.NotNil(t, dc)
			assert.Equal(t, "HEAD", dc.Ref)

			break
		}
	}

	assert.True(t, foundAddedStack, "Added stack should be discovered")

	// Verify removed stack is in fromWorktree
	removedStackPath := filepath.Join(fromWorktree, stackToBeRemovedRel)
	foundRemovedStack := false

	for _, c := range components {
		if c.Path() == removedStackPath {
			foundRemovedStack = true
			dc := c.DiscoveryContext()
			assert.NotNil(t, dc)
			assert.Equal(t, "HEAD~1", dc.Ref)
			assert.Contains(t, dc.Args, "-destroy", "Removed stack should have -destroy flag")

			break
		}
	}

	assert.True(t, foundRemovedStack, "Removed stack should be discovered")
}

// TestWorktreePhase_Integration_FileRename tests that file renames are detected.
func TestWorktreePhase_Integration_FileRename(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a unit with a file
	unitDir := createUnit(t, tmpDir, "unit", `# Unit config`)

	err := os.WriteFile(filepath.Join(unitDir, "original.tf"), []byte(`# Same content before and after rename`), 0644)
	require.NoError(t, err)

	commitChanges(t, runner, "Initial commit with original.tf")

	// Rename the file (same content, different name)
	err = os.Rename(
		filepath.Join(unitDir, "original.tf"),
		filepath.Join(unitDir, "renamed.tf"),
	)
	require.NoError(t, err)

	commitChanges(t, runner, "Rename original.tf to renamed.tf")

	// Run worktree discovery
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}
	components, w := runWorktreeDiscovery(t, tmpDir, gitExpressions, "plan", nil)

	// The unit should be detected as changed because the file was renamed
	assert.NotEmpty(t, components, "Unit with renamed file should be detected as changed")

	// Verify we have the unit
	toWorktree := w.WorktreePairs["[HEAD~1...HEAD]"].ToWorktree.Path
	expectedUnitPath := filepath.Join(toWorktree, "unit")

	unitPaths := components.Paths()
	assert.Contains(t, unitPaths, expectedUnitPath, "Should discover the unit with renamed file")
}

// TestWorktreePhase_Integration_FileMove tests that file moves are detected.
func TestWorktreePhase_Integration_FileMove(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a unit with a file in root
	unitDir := createUnit(t, tmpDir, "unit", `# Unit config`)

	err := os.WriteFile(filepath.Join(unitDir, "module.tf"), []byte(`# Module content`), 0644)
	require.NoError(t, err)

	commitChanges(t, runner, "Initial commit with module.tf in root")

	// Move file to subdirectory (same content, different path)
	subDir := filepath.Join(unitDir, "modules")
	err = os.MkdirAll(subDir, 0755)
	require.NoError(t, err)

	err = os.Rename(
		filepath.Join(unitDir, "module.tf"),
		filepath.Join(subDir, "module.tf"),
	)
	require.NoError(t, err)

	commitChanges(t, runner, "Move module.tf to modules/ subdirectory")

	// Run worktree discovery
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}
	components, _ := runWorktreeDiscovery(t, tmpDir, gitExpressions, "plan", nil)

	// The unit should be detected as changed because the file was moved
	assert.NotEmpty(t, components, "Unit with moved file should be detected as changed")

	// Verify we have the unit
	foundUnit := false

	for _, c := range components {
		if _, ok := c.(*component.Unit); ok {
			foundUnit = true
			break
		}
	}

	assert.True(t, foundUnit, "Should discover the unit with moved file")
}

// TestWorktreePhase_Integration_NestedUnits tests discovery of nested units.
func TestWorktreePhase_Integration_NestedUnits(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create nested unit structure
	createUnit(t, tmpDir, "apps/frontend", `# Frontend unit`)
	createUnit(t, tmpDir, "apps/backend", `# Backend unit`)
	createUnit(t, tmpDir, "apps/backend/db", `# Database unit`)

	commitChanges(t, runner, "Initial commit")

	// Modify the nested unit
	err := os.WriteFile(
		filepath.Join(tmpDir, "apps/backend/db", "terragrunt.hcl"),
		[]byte(`# Modified database unit`),
		0644,
	)
	require.NoError(t, err)

	commitChanges(t, runner, "Modify nested unit")

	// Run worktree discovery
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}
	components, w := runWorktreeDiscovery(t, tmpDir, gitExpressions, "", nil)

	// Verify the nested unit was discovered
	units := components.Filter(component.UnitKind)
	assert.Len(t, units, 1, "Only the modified nested unit should be discovered")

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	toWorktree := worktreePair.ToWorktree.Path
	expectedPath := filepath.Join(toWorktree, "apps/backend/db")

	unitPaths := units.Paths()
	assert.Contains(t, unitPaths, expectedPath, "Nested unit should be discovered")
}

// TestWorktreePhase_Integration_MultipleGitExpressions tests discovery with multiple git expressions.
func TestWorktreePhase_Integration_MultipleGitExpressions(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create initial unit
	createUnit(t, tmpDir, "unit-a", `# Unit A`)

	commitChanges(t, runner, "Initial commit")

	// Create second unit
	createUnit(t, tmpDir, "unit-b", `# Unit B`)

	commitChanges(t, runner, "Add unit B")

	// Create third unit
	createUnit(t, tmpDir, "unit-c", `# Unit C`)

	commitChanges(t, runner, "Add unit C")

	// Run worktree discovery with expression covering last commit
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}
	components, w := runWorktreeDiscovery(t, tmpDir, gitExpressions, "", nil)

	// Should only discover unit-c (added in last commit)
	units := components.Filter(component.UnitKind)
	assert.Len(t, units, 1, "Only unit-c should be discovered")

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	toWorktree := worktreePair.ToWorktree.Path
	expectedPath := filepath.Join(toWorktree, "unit-c")

	unitPaths := units.Paths()
	assert.Contains(t, unitPaths, expectedPath, "Unit C should be discovered")
}

// TestWorktreePhase_Integration_GitFilterCombinedWithOtherFilters tests git filters combined
// with other filter types (path, name, type, negation).
func TestWorktreePhase_Integration_GitFilterCombinedWithOtherFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		filterQueries   func(fromRef, toRef string) []string
		wantUnits       func(fromDir, toDir string) []string
		wantStacks      []string
		expectedChanged []string // which units are expected to be changed in the test setup
	}{
		{
			name: "Git filter combined with path filter",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "..." + toRef + "] | ./app"}
			},
			wantUnits: func(_, toDir string) []string {
				return []string{filepath.Join(toDir, "app")}
			},
			wantStacks:      []string{},
			expectedChanged: []string{"app", "new"},
		},
		{
			name: "Git filter combined with name filter",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "..." + toRef + "] | name=new"}
			},
			wantUnits: func(_, toDir string) []string {
				return []string{filepath.Join(toDir, "new")}
			},
			wantStacks:      []string{},
			expectedChanged: []string{"app", "new"},
		},
		{
			name: "Git filter with negation",
			filterQueries: func(fromRef, toRef string) []string {
				return []string{"[" + fromRef + "..." + toRef + "] | !name=new"}
			},
			wantUnits: func(fromDir, toDir string) []string {
				return []string{
					filepath.Join(fromDir, "cache"),
					filepath.Join(toDir, "app"),
				}
			},
			wantStacks:      []string{},
			expectedChanged: []string{"app", "new", "cache"},
		},
		{
			name: "Git filter - single reference (compared to HEAD)",
			filterQueries: func(fromRef, _ string) []string {
				return []string{"[" + fromRef + "]"}
			},
			wantUnits: func(fromDir, toDir string) []string {
				return []string{
					filepath.Join(fromDir, "cache"),
					filepath.Join(toDir, "app"),
					filepath.Join(toDir, "new"),
				}
			},
			wantStacks:      []string{},
			expectedChanged: []string{"app", "new", "cache"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupGitRepo(t)

			// Create initial components
			createUnit(t, tmpDir, "app", `# App unit`)
			createUnit(t, tmpDir, "db", `# DB unit`)
			createUnit(t, tmpDir, "cache", `# Cache unit`)

			commitChanges(t, runner, "Initial commit")

			// Modify app component
			err := os.WriteFile(filepath.Join(tmpDir, "app", "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0644)
			require.NoError(t, err)

			// Add new component
			createUnit(t, tmpDir, "new", `# New unit`)

			// Remove cache component
			err = os.RemoveAll(filepath.Join(tmpDir, "cache"))
			require.NoError(t, err)

			commitChanges(t, runner, "Changes: modified app, added new, removed cache")

			// Parse filter queries
			l := logger.CreateLogger()
			filterQueries := tt.filterQueries("HEAD~1", "HEAD")
			filters, err := filter.ParseFilterQueries(l, filterQueries)
			require.NoError(t, err)

			// Create worktrees
			w, err := worktrees.NewWorktrees(t.Context(), l, tmpDir, filters.UniqueGitFilters())
			require.NoError(t, err)

			t.Cleanup(func() {
				cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
				require.NoError(t, cleanupErr)
			})

			opts := options.NewTerragruntOptions()
			opts.WorkingDir = tmpDir
			opts.RootWorkingDir = tmpDir

			discoveryContext := &component.DiscoveryContext{
				WorkingDir: tmpDir,
			}

			discovery := v2.New(tmpDir).
				WithDiscoveryContext(discoveryContext).
				WithWorktrees(w).
				WithFilters(filters)

			components, err := discovery.Discover(t.Context(), l, opts)
			require.NoError(t, err)

			// Filter results by type
			units := components.Filter(component.UnitKind).Paths()
			stacks := components.Filter(component.StackKind).Paths()

			worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
			require.NotEmpty(t, worktreePair)

			wantUnits := tt.wantUnits(worktreePair.FromWorktree.Path, worktreePair.ToWorktree.Path)

			// Verify results
			assert.ElementsMatch(t, wantUnits, units, "Units mismatch for test: %s", tt.name)
			assert.ElementsMatch(t, tt.wantStacks, stacks, "Stacks mismatch for test: %s", tt.name)
		})
	}
}

// TestWorktreePhase_Integration_FromSubdirectory tests that git filter discovery works correctly
// when running from a subdirectory of the git root. This is a regression test for the bug where
// paths were incorrectly duplicated (e.g., "basic/basic/basic-2" instead of "basic/basic-2").
func TestWorktreePhase_Integration_FromSubdirectory(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create subdirectory structure: basic/basic-1, basic/basic-2
	basicDir := filepath.Join(tmpDir, "basic")
	basic1Dir := filepath.Join(basicDir, "basic-1")
	basic2Dir := filepath.Join(basicDir, "basic-2")

	// Also create a component outside the subdirectory
	otherDir := filepath.Join(tmpDir, "other")

	testDirs := []string{basic1Dir, basic2Dir, otherDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Create initial files
	initialFiles := map[string]string{
		filepath.Join(basic1Dir, "terragrunt.hcl"): ``,
		filepath.Join(basic2Dir, "terragrunt.hcl"): ``,
		filepath.Join(otherDir, "terragrunt.hcl"):  ``,
	}

	for path, content := range initialFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	commitChanges(t, runner, "Initial commit")

	// Modify basic-2 component
	err := os.WriteFile(filepath.Join(basic2Dir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0644)
	require.NoError(t, err)

	commitChanges(t, runner, "Modified basic-2")

	// Now run discovery FROM THE SUBDIRECTORY (basic)
	l := logger.CreateLogger()

	// Parse filter with Git reference
	filters, err := filter.ParseFilterQueries(l, []string{"[HEAD~1]"})
	require.NoError(t, err)

	// Create worktrees from the subdirectory
	w, err := worktrees.NewWorktrees(t.Context(), l, basicDir, filters.UniqueGitFilters())
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = basicDir
	opts.RootWorkingDir = basicDir

	discoveryContext := &component.DiscoveryContext{
		WorkingDir: basicDir,
	}

	discovery := v2.New(basicDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w).
		WithFilters(filters)

	components, err := discovery.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Filter results by type
	units := components.Filter(component.UnitKind).Paths()

	// With worktree-based execution, discovery runs directly in the worktree path
	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	require.NotEmpty(t, worktreePair)

	expectedPath := filepath.Join(worktreePair.ToWorktree.Path, "basic", "basic-2")
	assert.ElementsMatch(t, []string{expectedPath}, units,
		"Should discover basic-2 with correct path when running from subdirectory")

	// Verify the path doesn't have duplicated directory names
	for _, unitPath := range units {
		assert.NotContains(t, unitPath, "basic"+string(filepath.Separator)+"basic"+string(filepath.Separator)+"basic-",
			"Path should not have duplicated directory names")
	}
}

// setupMultiCommitTestRepo creates a git repository with 4 commits for testing
// git filter discovery from a subdirectory. Returns the basicDir (subdirectory).
func setupMultiCommitTestRepo(t *testing.T) string {
	t.Helper()

	tmpDir, runner := setupGitRepo(t)

	// Create subdirectory structure: basic/basic-1, basic/basic-2, basic/basic-3
	basicDir := filepath.Join(tmpDir, "basic")
	basic1Dir := filepath.Join(basicDir, "basic-1")
	basic2Dir := filepath.Join(basicDir, "basic-2")
	basic3Dir := filepath.Join(basicDir, "basic-3")

	// Also create components outside the subdirectory
	otherDir := filepath.Join(tmpDir, "other")
	anotherDir := filepath.Join(tmpDir, "another")

	testDirs := []string{basic1Dir, basic2Dir, basic3Dir, otherDir, anotherDir}
	for _, dir := range testDirs {
		err := os.MkdirAll(dir, 0755)
		require.NoError(t, err)
	}

	// Commit 1: Initial state with all components
	initialFiles := map[string]string{
		filepath.Join(basic1Dir, "terragrunt.hcl"):  ``,
		filepath.Join(basic2Dir, "terragrunt.hcl"):  ``,
		filepath.Join(basic3Dir, "terragrunt.hcl"):  ``,
		filepath.Join(otherDir, "terragrunt.hcl"):   ``,
		filepath.Join(anotherDir, "terragrunt.hcl"): ``,
	}

	for path, content := range initialFiles {
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)
	}

	commitChanges(t, runner, "Initial commit")

	// Commit 2: Modify basic-1 and other (outside subdirectory)
	err := os.WriteFile(filepath.Join(basic1Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v1"
}
`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(otherDir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0644)
	require.NoError(t, err)

	commitChanges(t, runner, "Commit 2: modify basic-1 and other")

	// Commit 3: Modify basic-2 and another (outside subdirectory)
	err = os.WriteFile(filepath.Join(basic2Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v2"
}
`), 0644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(anotherDir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0644)
	require.NoError(t, err)

	commitChanges(t, runner, "Commit 3: modify basic-2 and another")

	// Commit 4: Modify basic-3
	err = os.WriteFile(filepath.Join(basic3Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v3"
}
`), 0644)
	require.NoError(t, err)

	commitChanges(t, runner, "Commit 4: modify basic-3")

	return basicDir
}

// TestWorktreePhase_Integration_FromSubdirectory_MultipleCommits tests git filter discovery
// initiated from a subdirectory when comparing against multiple commits back (HEAD~2, HEAD~3).
func TestWorktreePhase_Integration_FromSubdirectory_MultipleCommits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		expectedUnitsFunc func(toWorktreePath string) []string
		name              string
		gitRef            string
	}{
		{
			name:   "HEAD~1 from subdirectory - only basic-3",
			gitRef: "HEAD~1",
			expectedUnitsFunc: func(toWorktreePath string) []string {
				return []string{filepath.Join(toWorktreePath, "basic", "basic-3")}
			},
		},
		{
			name:   "HEAD~2 from subdirectory - basic-2 and basic-3, plus another",
			gitRef: "HEAD~2",
			expectedUnitsFunc: func(toWorktreePath string) []string {
				// With worktree-root discovery, we find all changed units including 'another'
				return []string{
					filepath.Join(toWorktreePath, "another"),
					filepath.Join(toWorktreePath, "basic", "basic-2"),
					filepath.Join(toWorktreePath, "basic", "basic-3"),
				}
			},
		},
		{
			name:   "HEAD~3 from subdirectory - basic-1, basic-2, basic-3, plus other and another",
			gitRef: "HEAD~3",
			expectedUnitsFunc: func(toWorktreePath string) []string {
				// With worktree-root discovery, we find all changed units
				return []string{
					filepath.Join(toWorktreePath, "other"),
					filepath.Join(toWorktreePath, "another"),
					filepath.Join(toWorktreePath, "basic", "basic-1"),
					filepath.Join(toWorktreePath, "basic", "basic-2"),
					filepath.Join(toWorktreePath, "basic", "basic-3"),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Each subtest creates its own git repository
			basicDir := setupMultiCommitTestRepo(t)

			l := logger.CreateLogger()

			// Parse filter with Git reference
			filters, err := filter.ParseFilterQueries(l, []string{"[" + tt.gitRef + "]"})
			require.NoError(t, err)

			// Create worktrees from the subdirectory
			w, err := worktrees.NewWorktrees(t.Context(), l, basicDir, filters.UniqueGitFilters())
			require.NoError(t, err)

			t.Cleanup(func() {
				cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
				require.NoError(t, cleanupErr)
			})

			opts := options.NewTerragruntOptions()
			opts.WorkingDir = basicDir
			opts.RootWorkingDir = basicDir

			discoveryContext := &component.DiscoveryContext{
				WorkingDir: basicDir,
			}

			discovery := v2.New(basicDir).
				WithDiscoveryContext(discoveryContext).
				WithWorktrees(w).
				WithFilters(filters)

			components, err := discovery.Discover(t.Context(), l, opts)
			require.NoError(t, err)

			// Filter results by type
			units := components.Filter(component.UnitKind).Paths()

			// Get worktree pair for expected path calculation
			worktreePair := w.WorktreePairs["["+tt.gitRef+"...HEAD]"]
			require.NotEmpty(t, worktreePair)

			// Verify correct units are discovered
			expectedUnits := tt.expectedUnitsFunc(worktreePair.ToWorktree.Path)
			assert.ElementsMatch(t, expectedUnits, units,
				"Should discover correct units when running from subdirectory with %s", tt.gitRef)

			// Verify no path duplication
			for _, unitPath := range units {
				assert.NotContains(t, unitPath,
					"basic"+string(filepath.Separator)+"basic"+string(filepath.Separator)+"basic-",
					"Path should not have duplicated directory names")
			}
		})
	}
}
