package discovery_test

import (
	"context"
	"fmt"
	"io/fs"
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
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	err := os.WriteFile(filepath.Join(tmpDir, "unit-to-be-modified", "terragrunt.hcl"), []byte(`# Unit modified`), 0o644)
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
			err := os.WriteFile(filepath.Join(tmpDir, "unit-to-be-modified", "terragrunt.hcl"), []byte(`# Modified`), 0o644)
			require.NoError(t, err)

			// Remove the unit
			err = os.RemoveAll(filepath.Join(tmpDir, "unit-to-be-removed"))
			require.NoError(t, err)

			// Add a new unit
			createUnit(t, tmpDir, "unit-to-be-created", `# Created`)

			commitChanges(t, runner, "Update units")

			// Set up discovery
			l := logger.CreateLogger()

			w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: gitExpressions})
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

			discovery := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(discoveryContext).
				WithWorktrees(w)

			filters := make(filter.Filters, 0, len(gitExpressions))

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
	err = os.WriteFile(readmePath, []byte("# Test"), 0o644)
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
	err := os.MkdirAll(legacyUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "terragrunt.hcl"), []byte(`# Legacy unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0o644)
	require.NoError(t, err)

	modernUnitDir := filepath.Join(tmpDir, "catalog", "units", "modern")
	err = os.MkdirAll(modernUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(modernUnitDir, "terragrunt.hcl"), []byte(`# Modern unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(modernUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0o644)
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
	err = os.MkdirAll(stackToBeModifiedDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0o644)
	require.NoError(t, err)

	stackToBeRemovedDir := filepath.Join(tmpDir, "live", "stack-to-be-removed")
	err = os.MkdirAll(stackToBeRemovedDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeRemovedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0o644)
	require.NoError(t, err)

	stackToBeUntouchedDir := filepath.Join(tmpDir, "live", "stack-to-be-untouched")
	err = os.MkdirAll(stackToBeUntouchedDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeUntouchedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create stacks")

	// Add a new stack
	stackToBeAddedDir := filepath.Join(tmpDir, "live", "stack-to-be-added")
	err = os.MkdirAll(stackToBeAddedDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackToBeAddedDir, "terragrunt.stack.hcl"), []byte(stackFileContents), 0o644)
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
	err = os.WriteFile(filepath.Join(stackToBeModifiedDir, "terragrunt.stack.hcl"), []byte(modifiedStackContents), 0o644)
	require.NoError(t, err)

	// Remove the second stack
	err = os.RemoveAll(stackToBeRemovedDir)
	require.NoError(t, err)

	commitChanges(t, runner, "Modify and remove stacks")

	// Set up discovery with worktrees
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: gitExpressions})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	// Generate stacks in worktrees
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir
	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
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

	discovery := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w)

	filters := make(filter.Filters, 0, len(gitExpressions))

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

	err := os.WriteFile(filepath.Join(unitDir, "original.tf"), []byte(`# Same content before and after rename`), 0o644)
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

	err := os.WriteFile(filepath.Join(unitDir, "module.tf"), []byte(`# Module content`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Initial commit with module.tf in root")

	// Move file to subdirectory (same content, different path)
	subDir := filepath.Join(unitDir, "modules")
	err = os.MkdirAll(subDir, 0o755)
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
		0o644,
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
`), 0o644)
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
			w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()})
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

			discovery := discovery.NewDiscovery(tmpDir).
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
		err := os.MkdirAll(dir, 0o755)
		require.NoError(t, err)
	}

	// Create initial files
	initialFiles := map[string]string{
		filepath.Join(basic1Dir, "terragrunt.hcl"): ``,
		filepath.Join(basic2Dir, "terragrunt.hcl"): ``,
		filepath.Join(otherDir, "terragrunt.hcl"):  ``,
	}

	for path, content := range initialFiles {
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
	}

	commitChanges(t, runner, "Initial commit")

	// Modify basic-2 component
	err := os.WriteFile(filepath.Join(basic2Dir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Modified basic-2")

	// Now run discovery FROM THE SUBDIRECTORY (basic)
	l := logger.CreateLogger()

	// Parse filter with Git reference
	filters, err := filter.ParseFilterQueries(l, []string{"[HEAD~1]"})
	require.NoError(t, err)

	// Create worktrees from the subdirectory
	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: basicDir, GitExpressions: filters.UniqueGitFilters()})
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

	discovery := discovery.NewDiscovery(basicDir).
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
		err := os.MkdirAll(dir, 0o755)
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
		err := os.WriteFile(path, []byte(content), 0o644)
		require.NoError(t, err)
	}

	commitChanges(t, runner, "Initial commit")

	// Commit 2: Modify basic-1 and other (outside subdirectory)
	err := os.WriteFile(filepath.Join(basic1Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v1"
}
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(otherDir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Commit 2: modify basic-1 and other")

	// Commit 3: Modify basic-2 and another (outside subdirectory)
	err = os.WriteFile(filepath.Join(basic2Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v2"
}
`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(anotherDir, "terragrunt.hcl"), []byte(`
locals {
	modified = true
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Commit 3: modify basic-2 and another")

	// Commit 4: Modify basic-3
	err = os.WriteFile(filepath.Join(basic3Dir, "terragrunt.hcl"), []byte(`
locals {
	version = "v3"
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Commit 4: modify basic-3")

	return basicDir
}

// TestWorktreePhase_Integration_NegatedGitGraphExpressions tests that negated Git+Graph expressions
// work correctly. These are expressions where the negation wraps the Git expression:
// - `![HEAD~1...HEAD]...` - Exclude changed components AND their dependencies
// - `!...[HEAD~1...HEAD]` - Exclude changed components AND their dependents
//
// In worktree-based discovery, only changed components are initially discovered.
// When a negated git+graph filter is used in combination with a positive git filter,
// the filter semantics cause components matching the negation to be excluded.
//
// This validates the complete pipeline: worktree → graph → final evaluation with negation.
func TestWorktreePhase_Integration_NegatedGitGraphExpressions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		filterQueries   func(fromRef, toRef string) []string
		wantUnits       func(fromDir, toDir string) []string
		description     string
		expectedChanged []string // which units are expected to be changed in the test setup
	}{
		{
			name: "simple negated git expression excludes all changed",
			filterQueries: func(fromRef, toRef string) []string {
				// ![HEAD~1...HEAD] = Negated git expression excludes changed components
				// When combined with positive filter, negation takes precedence
				return []string{
					"[" + fromRef + "..." + toRef + "]",  // Include all changed
					"![" + fromRef + "..." + toRef + "]", // Exclude changed
				}
			},
			wantUnits: func(_, _ string) []string {
				// Both filters apply: positive includes, negative excludes
				// Components matching negation are excluded from final result
				return []string{}
			},
			description:     "Positive and negative git filters - negation excludes all",
			expectedChanged: []string{"app", "db"},
		},
		{
			name: "negated git with dependency traversal in intersection",
			filterQueries: func(fromRef, toRef string) []string {
				// Use intersection to apply negated graph filter to git results
				// [HEAD~1...HEAD] | ![HEAD~1...HEAD]... = changed AND NOT (changed with deps)
				return []string{"[" + fromRef + "..." + toRef + "] | ![" + fromRef + "..." + toRef + "]..."}
			},
			wantUnits: func(_, _ string) []string {
				// Intersection: component must match [changed] AND match ![changed]...
				// For any changed component, ![changed]... is false (negation of true)
				// So intersection is always empty
				return []string{}
			},
			description:     "Git filter intersected with negated git+deps - empty result",
			expectedChanged: []string{"app"},
		},
		{
			name: "negated git with dependent traversal in intersection",
			filterQueries: func(fromRef, toRef string) []string {
				// Use intersection: [HEAD~1...HEAD] | !...[HEAD~1...HEAD]
				return []string{"[" + fromRef + "..." + toRef + "] | !...[" + fromRef + "..." + toRef + "]"}
			},
			wantUnits: func(_, _ string) []string {
				// Intersection: component must match [changed] AND match !...[changed]
				// For any changed component, !...[changed] is false
				// So intersection is always empty
				return []string{}
			},
			description:     "Git filter intersected with negated git+dependents - empty result",
			expectedChanged: []string{"vpc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tmpDir, runner := setupGitRepo(t)

			// Create dependency chain: app -> db -> vpc
			// Plus an unrelated component for verification
			vpcDir := filepath.Join(tmpDir, "vpc")
			dbDir := filepath.Join(tmpDir, "db")
			appDir := filepath.Join(tmpDir, "app")
			unrelatedDir := filepath.Join(tmpDir, "unrelated")

			testDirs := []string{vpcDir, dbDir, appDir, unrelatedDir}
			for _, dir := range testDirs {
				err := os.MkdirAll(dir, 0o755)
				require.NoError(t, err)
			}

			// Create initial files with dependencies
			testFiles := map[string]string{
				filepath.Join(appDir, "terragrunt.hcl"): `
dependency "db" {
	config_path = "../db"
}
`,
				filepath.Join(dbDir, "terragrunt.hcl"): `
dependency "vpc" {
	config_path = "../vpc"
}
`,
				filepath.Join(vpcDir, "terragrunt.hcl"):       ``,
				filepath.Join(unrelatedDir, "terragrunt.hcl"): ``,
			}

			for path, content := range testFiles {
				err := os.WriteFile(path, []byte(content), 0o644)
				require.NoError(t, err)
			}

			commitChanges(t, runner, "Initial commit")

			// Modify the expected changed components based on test case
			for _, changed := range tt.expectedChanged {
				changedPath := filepath.Join(tmpDir, changed, "terragrunt.hcl")
				currentContent, err := os.ReadFile(changedPath)
				require.NoError(t, err)

				newContent := string(currentContent) + `
locals {
	modified = true
}
`
				err = os.WriteFile(changedPath, []byte(newContent), 0o644)
				require.NoError(t, err)
			}

			commitChanges(t, runner, "Modify components: "+tt.description)

			// Parse filter queries
			l := logger.CreateLogger()
			filterQueries := tt.filterQueries("HEAD~1", "HEAD")
			filters, err := filter.ParseFilterQueries(l, filterQueries)
			require.NoError(t, err)

			// Create worktrees
			w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()})
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

			discovery := discovery.NewDiscovery(tmpDir).
				WithDiscoveryContext(discoveryContext).
				WithWorktrees(w).
				WithFilters(filters)

			components, err := discovery.Discover(t.Context(), l, opts)
			require.NoError(t, err)

			// Filter results by type
			units := components.Filter(component.UnitKind).Paths()

			worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
			require.NotEmpty(t, worktreePair)

			wantUnits := tt.wantUnits(worktreePair.FromWorktree.Path, worktreePair.ToWorktree.Path)

			// Verify results
			assert.ElementsMatch(t, wantUnits, units, "Units mismatch for test: %s\nDescription: %s", tt.name, tt.description)
		})
	}
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
			w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: basicDir, GitExpressions: filters.UniqueGitFilters()})
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

			discovery := discovery.NewDiscovery(basicDir).
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

// setupGitRepo creates a git repository with initial structure for integration tests.
func setupGitRepo(t *testing.T) (string, *git.GitRunner) {
	t.Helper()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	runner, err := git.NewGitRunner(vexec.NewOSExec())
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
	err := os.MkdirAll(unitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(content), 0o644)
	require.NoError(t, err)

	return unitDir
}

// TestWorktreePhase_Integration_StackReadingChanges tests that changes to files referenced
// via read_terragrunt_config() in a stack file trigger stack change detection, while changes
// to unreferenced files in the same directory do not.
func TestWorktreePhase_Integration_StackReadingChanges(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a catalog unit
	legacyUnitDir := filepath.Join(tmpDir, "catalog", "units", "legacy")
	err := os.MkdirAll(legacyUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "terragrunt.hcl"), []byte(`# Legacy unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create catalog units")

	// Create a stack that references a sidecar file via read_terragrunt_config
	stackWithRefDir := filepath.Join(tmpDir, "live", "stack-with-ref")
	err = os.MkdirAll(stackWithRefDir, 0o755)
	require.NoError(t, err)

	// Sidecar file referenced by the stack
	err = os.WriteFile(filepath.Join(stackWithRefDir, "config.hcl"), []byte(`inputs = { version = "v1" }`), 0o644)
	require.NoError(t, err)

	stackWithRefContent := `
locals {
  config = read_terragrunt_config("config.hcl")
}

unit "app" {
  source = "${get_repo_root()}/catalog/units/legacy"
  path   = "app"
}
`
	err = os.WriteFile(filepath.Join(stackWithRefDir, "terragrunt.stack.hcl"), []byte(stackWithRefContent), 0o644)
	require.NoError(t, err)

	// Create a stack WITHOUT read_terragrunt_config but with a file in same dir
	stackNoRefDir := filepath.Join(tmpDir, "live", "stack-no-ref")
	err = os.MkdirAll(stackNoRefDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackNoRefDir, "unrelated.hcl"), []byte(`# not referenced`), 0o644)
	require.NoError(t, err)

	stackNoRefContent := `
unit "app" {
  source = "${get_repo_root()}/catalog/units/legacy"
  path   = "app"
}
`
	err = os.WriteFile(filepath.Join(stackNoRefDir, "terragrunt.stack.hcl"), []byte(stackNoRefContent), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create stacks with and without read_terragrunt_config")

	// Change only the sidecar files (not the stack files)
	err = os.WriteFile(filepath.Join(stackWithRefDir, "config.hcl"), []byte(`inputs = { version = "v2" }`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackNoRefDir, "unrelated.hcl"), []byte(`# still not referenced but modified`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Update sidecar files only")

	// Set up discovery with worktrees
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: gitExpressions})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	// Generate stacks in worktrees
	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
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

	disc := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w)

	filters := make(filter.Filters, 0, len(gitExpressions))

	for _, gitExpr := range gitExpressions {
		f := filter.NewFilter(gitExpr, gitExpr.String())
		filters = append(filters, f)
	}

	disc = disc.WithFilters(filters)

	components, err := disc.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Get worktree paths
	require.Contains(t, w.WorktreePairs, "[HEAD~1...HEAD]", "Worktree pair should exist")

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	toWorktree := worktreePair.ToWorktree.Path

	// Collect component paths for debugging on failure
	componentPaths := make([]string, 0, len(components))
	for _, c := range components {
		componentPaths = append(componentPaths, c.Path())
	}

	// Verify: stack-with-ref should be discovered (config.hcl is referenced via read_terragrunt_config)
	stackWithRefRel, err := filepath.Rel(tmpDir, stackWithRefDir)
	require.NoError(t, err)

	expectedStackWithRef := filepath.Join(toWorktree, stackWithRefRel)
	foundStackWithRef := false

	for _, c := range components {
		if c.Path() == expectedStackWithRef {
			foundStackWithRef = true

			break
		}
	}

	assert.True(t, foundStackWithRef,
		"Stack with read_terragrunt_config reference should be discovered when sidecar changes; got: %v", componentPaths)

	// Verify: stack-no-ref should NOT be discovered (unrelated.hcl is not referenced)
	stackNoRefRel, err := filepath.Rel(tmpDir, stackNoRefDir)
	require.NoError(t, err)

	expectedStackNoRef := filepath.Join(toWorktree, stackNoRefRel)
	foundStackNoRef := false

	for _, c := range components {
		if c.Path() == expectedStackNoRef {
			foundStackNoRef = true

			break
		}
	}

	assert.False(t, foundStackNoRef,
		"Stack without read_terragrunt_config reference should NOT be discovered; got: %v", componentPaths)
}

// TestWorktreePhase_Integration_StackReadingDedup tests that when both the stack file itself
// and a sidecar file referenced via read_terragrunt_config() change in the same commit,
// the stack is discovered exactly once (no duplication from buildHandledStackDirs).
func TestWorktreePhase_Integration_StackReadingDedup(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a catalog unit
	legacyUnitDir := filepath.Join(tmpDir, "catalog", "units", "legacy")
	err := os.MkdirAll(legacyUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "terragrunt.hcl"), []byte(`# Legacy unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create catalog units")

	// Create a stack with read_terragrunt_config + sidecar
	stackDir := filepath.Join(tmpDir, "live", "dedup-stack")
	err = os.MkdirAll(stackDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackDir, "config.hcl"), []byte(`inputs = { version = "v1" }`), 0o644)
	require.NoError(t, err)

	stackContent := `
locals {
  config = read_terragrunt_config("config.hcl")
}

unit "app" {
  source = "${get_repo_root()}/catalog/units/legacy"
  path   = "app"
}
`
	err = os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create stack with read_terragrunt_config")

	// Change BOTH the stack file AND the sidecar file in the same commit
	updatedStackContent := `
locals {
  config = read_terragrunt_config("config.hcl")
}

unit "app" {
  source = "${get_repo_root()}/catalog/units/legacy"
  path   = "app-v2"
}
`
	err = os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(updatedStackContent), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackDir, "config.hcl"), []byte(`inputs = { version = "v2" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Update both stack file and sidecar")

	// Set up discovery
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: gitExpressions})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
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

	disc := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w)

	filters := make(filter.Filters, 0, len(gitExpressions))

	for _, gitExpr := range gitExpressions {
		f := filter.NewFilter(gitExpr, gitExpr.String())
		filters = append(filters, f)
	}

	disc = disc.WithFilters(filters)

	components, err := disc.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Get worktree pair path
	require.Contains(t, w.WorktreePairs, "[HEAD~1...HEAD]", "Worktree pair should exist")

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	toWorktree := worktreePair.ToWorktree.Path

	// Collect component paths
	componentPaths := make([]string, 0, len(components))
	for _, c := range components {
		componentPaths = append(componentPaths, c.Path())
	}

	// The stack should be discovered (stack file changed)
	stackRel, err := filepath.Rel(tmpDir, stackDir)
	require.NoError(t, err)

	expectedStackPath := filepath.Join(toWorktree, stackRel)

	// Verify no duplicate paths — dedup via buildHandledStackDirs should prevent
	// findStacksAffectedByReading from adding the stack again
	seen := make(map[string]int, len(components))

	for _, c := range components {
		seen[c.Path()]++
	}

	for p, count := range seen {
		assert.Equal(t, 1, count,
			"Component path %s appears %d times (expected 1); all: %v", p, count, componentPaths)
	}

	// Verify the stack itself is discovered
	_, foundStack := seen[expectedStackPath]
	assert.True(t, foundStack,
		"Stack %s should be discovered when both stack file and sidecar change; got: %v", expectedStackPath, componentPaths)
}

// TestWorktreePhase_Integration_StackReadingNestedPath tests that stacks referencing sidecar
// files at nested or sibling paths (e.g., read_terragrunt_config("../../env/config.hcl"))
// are correctly discovered when those files change.
func TestWorktreePhase_Integration_StackReadingNestedPath(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a catalog unit
	legacyUnitDir := filepath.Join(tmpDir, "catalog", "units", "legacy")
	err := os.MkdirAll(legacyUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "terragrunt.hcl"), []byte(`# Legacy unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(legacyUnitDir, "main.tf"), []byte(`# Intentionally empty`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create catalog units")

	// Create a sidecar config in a DIFFERENT directory tree than the stack
	envDir := filepath.Join(tmpDir, "env")
	err = os.MkdirAll(envDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(envDir, "config.hcl"), []byte(`inputs = { version = "v1" }`), 0o644)
	require.NoError(t, err)

	// Create a stack that references the sidecar via a nested/sibling path
	stackDir := filepath.Join(tmpDir, "live", "my-stack")
	err = os.MkdirAll(stackDir, 0o755)
	require.NoError(t, err)

	stackContent := `
locals {
  config = read_terragrunt_config("../../env/config.hcl")
}

unit "app" {
  source = "${get_repo_root()}/catalog/units/legacy"
  path   = "app"
}
`
	err = os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	// Create a stack WITHOUT a cross-directory reference (control)
	stackNoRefDir := filepath.Join(tmpDir, "live", "no-ref-stack")
	err = os.MkdirAll(stackNoRefDir, 0o755)
	require.NoError(t, err)

	stackNoRefContent := `
unit "app" {
  source = "${get_repo_root()}/catalog/units/legacy"
  path   = "app"
}
`
	err = os.WriteFile(filepath.Join(stackNoRefDir, "terragrunt.stack.hcl"), []byte(stackNoRefContent), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create stacks and env config")

	// Change ONLY the sidecar file in the separate directory
	err = os.WriteFile(filepath.Join(envDir, "config.hcl"), []byte(`inputs = { version = "v2" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Update env config only")

	// Set up discovery
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: gitExpressions})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
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

	disc := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w)

	filters := make(filter.Filters, 0, len(gitExpressions))

	for _, gitExpr := range gitExpressions {
		f := filter.NewFilter(gitExpr, gitExpr.String())
		filters = append(filters, f)
	}

	disc = disc.WithFilters(filters)

	components, err := disc.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Get worktree paths
	require.Contains(t, w.WorktreePairs, "[HEAD~1...HEAD]", "Worktree pair should exist")

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	toWorktree := worktreePair.ToWorktree.Path

	// Collect component paths for debugging
	componentPaths := make([]string, 0, len(components))
	for _, c := range components {
		componentPaths = append(componentPaths, c.Path())
	}

	// Verify: stack with cross-directory read_terragrunt_config reference IS discovered
	stackRel, err := filepath.Rel(tmpDir, stackDir)
	require.NoError(t, err)

	expectedStack := filepath.Join(toWorktree, stackRel)
	foundStack := false

	for _, c := range components {
		if c.Path() == expectedStack {
			foundStack = true

			break
		}
	}

	assert.True(t, foundStack,
		"Stack with nested read_terragrunt_config reference should be discovered when sidecar changes; got: %v", componentPaths)

	// Verify: stack WITHOUT the reference should NOT be discovered
	stackNoRefRel, err := filepath.Rel(tmpDir, stackNoRefDir)
	require.NoError(t, err)

	expectedNoRef := filepath.Join(toWorktree, stackNoRefRel)
	foundNoRef := false

	for _, c := range components {
		if c.Path() == expectedNoRef {
			foundNoRef = true

			break
		}
	}

	assert.False(t, foundNoRef,
		"Stack without read_terragrunt_config reference should NOT be discovered; got: %v", componentPaths)
}

// TestWorktreePhase_Integration_StackNotGeneratedForUnitChanges verifies that when only
// unit files change (no read files involved), stacks in worktrees are not generated.
// This ensures we don't do unnecessary work when the change doesn't affect any stack.
func TestWorktreePhase_Integration_StackNotGeneratedForUnitChanges(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a catalog unit
	catalogUnitDir := filepath.Join(tmpDir, "catalog", "units", "myapp")
	err := os.MkdirAll(catalogUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogUnitDir, "terragrunt.hcl"), []byte(`# catalog unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogUnitDir, "main.tf"), []byte(`output "example" { value = "ok" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create catalog unit")

	// Create a stack (no read_terragrunt_config)
	stackDir := filepath.Join(tmpDir, "live", "app-stack")
	err = os.MkdirAll(stackDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(`
unit "myapp" {
  source = "${get_repo_root()}/catalog/units/myapp"
  path   = "myapp"
}
`), 0o644)
	require.NoError(t, err)

	// Create a standalone unit
	unitDir := filepath.Join(tmpDir, "live", "standalone")
	err = os.MkdirAll(unitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# standalone unit`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create stack and standalone unit")

	// Change ONLY the standalone unit (not anything the stack reads)
	err = os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte(`# standalone unit modified`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Modify standalone unit only")

	// Set up worktrees and run generation
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: gitExpressions,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.FilterFlag)
	require.NoError(t, err)

	// Generate stacks — using tmpDir as working directory so that only
	// worktreeStacksToGenerate can cause generation inside worktrees.
	err = generate.GenerateStacks(t.Context(), l, opts, w)
	require.NoError(t, err)

	// Verify: no .terragrunt-stack directories should exist in the worktrees,
	// because the only change was to a standalone unit (no reading filters).
	for _, pair := range w.WorktreePairs {
		for _, wt := range []worktrees.Worktree{pair.FromWorktree, pair.ToWorktree} {
			stackGenDir := filepath.Join(wt.Path, "live", "app-stack", ".terragrunt-stack")
			_, statErr := os.Stat(stackGenDir)
			require.ErrorIs(t, statErr, fs.ErrNotExist,
				"Stack should not be generated in worktree %s when only a unit changed, but %s exists",
				wt.Ref, stackGenDir)
		}
	}

	// Also verify: no reading-affected stacks were recorded
	assert.Empty(t, w.ReadingAffectedStacks,
		"No reading-affected stacks should be recorded when only a unit changed")
}

// TestWorktreePhase_Integration_StackReadingRespectsExclusion verifies that when a user
// excludes a stack via --filter, that stack is not parsed during worktree stack discovery
// even when reading filters are active. A "land-mine" stack with a run_cmd in locals
// creates a marker file when parsed; the test asserts the marker is never created.
func TestWorktreePhase_Integration_StackReadingRespectsExclusion(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a catalog unit
	catalogUnitDir := filepath.Join(tmpDir, "catalog", "units", "myapp")
	err := os.MkdirAll(catalogUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogUnitDir, "terragrunt.hcl"), []byte(`# catalog unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogUnitDir, "main.tf"), []byte(`output "example" { value = "ok" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create catalog unit")

	// Create a "land-mine" stack that creates a marker file when parsed.
	// If the exclusion filter works, this file should never be created.
	landMineDir := filepath.Join(tmpDir, "live", "land-mine")
	err = os.MkdirAll(landMineDir, 0o755)
	require.NoError(t, err)

	markerFile := filepath.Join(tmpDir, "land-mine-parsed.marker")

	err = os.WriteFile(filepath.Join(landMineDir, "terragrunt.stack.hcl"), []byte(fmt.Sprintf(`
locals {
  marker = run_cmd("--terragrunt-quiet", "bash", "-c", "touch %s")
}

unit "myapp" {
  source = "${get_repo_root()}/catalog/units/myapp"
  path   = "myapp"
}
`, markerFile)), 0o644)
	require.NoError(t, err)

	// Create a normal stack that reads a config file
	normalDir := filepath.Join(tmpDir, "live", "normal")
	err = os.MkdirAll(normalDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(normalDir, "config.hcl"), []byte(`inputs = { example = "v1" }`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(normalDir, "terragrunt.stack.hcl"), []byte(`
locals {
  config = read_terragrunt_config("config.hcl")
}

unit "myapp" {
  source = "${get_repo_root()}/catalog/units/myapp"
  path   = "myapp"
  values = local.config.inputs
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create land-mine and normal stacks")

	// Change the normal stack's read file
	err = os.WriteFile(filepath.Join(normalDir, "config.hcl"), []byte(`inputs = { example = "v2" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Update config file")

	// Set up worktrees
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: gitExpressions,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Use filters that exclude the land-mine stack and include the git expression
	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{
		"[HEAD~1...HEAD]",
		"!./live/land-mine | type=stack",
	})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.FilterFlag)
	require.NoError(t, err)

	// Generate stacks
	err = generate.GenerateStacks(t.Context(), l, opts, w)
	require.NoError(t, err)

	// The marker file should NOT exist — the land-mine stack should not have been parsed.
	_, statErr := os.Stat(markerFile)
	assert.ErrorIs(t, statErr, fs.ErrNotExist,
		"Land-mine stack was parsed (marker file created) despite being excluded by filter")
}

// TestWorktreePhase_Integration_StackReadingExclusionOverridesInclusion verifies that when
// a stack is both explicitly included and excluded by path, the exclusion wins and the
// stack is not parsed, even when a reading filter would otherwise trigger parsing.
func TestWorktreePhase_Integration_StackReadingExclusionOverridesInclusion(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a catalog unit
	catalogUnitDir := filepath.Join(tmpDir, "catalog", "units", "myapp")
	err := os.MkdirAll(catalogUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogUnitDir, "terragrunt.hcl"), []byte(`# catalog unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogUnitDir, "main.tf"), []byte(`output "example" { value = "ok" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create catalog unit")

	// Create a "land-mine" stack that creates a marker file when parsed
	landMineDir := filepath.Join(tmpDir, "live", "land-mine")
	err = os.MkdirAll(landMineDir, 0o755)
	require.NoError(t, err)

	markerFile := filepath.Join(tmpDir, "land-mine-parsed.marker")

	err = os.WriteFile(filepath.Join(landMineDir, "terragrunt.stack.hcl"), []byte(fmt.Sprintf(`
locals {
  marker = run_cmd("--terragrunt-quiet", "bash", "-c", "touch %s")
}

unit "myapp" {
  source = "${get_repo_root()}/catalog/units/myapp"
  path   = "myapp"
}
`, markerFile)), 0o644)
	require.NoError(t, err)

	// Create a normal stack that reads a config file
	normalDir := filepath.Join(tmpDir, "live", "normal")
	err = os.MkdirAll(normalDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(normalDir, "config.hcl"), []byte(`inputs = { example = "v1" }`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(normalDir, "terragrunt.stack.hcl"), []byte(`
locals {
  config = read_terragrunt_config("config.hcl")
}

unit "myapp" {
  source = "${get_repo_root()}/catalog/units/myapp"
  path   = "myapp"
  values = local.config.inputs
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create land-mine and normal stacks")

	// Change the normal stack's read file
	err = os.WriteFile(filepath.Join(normalDir, "config.hcl"), []byte(`inputs = { example = "v2" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Update config file")

	// Set up worktrees
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: gitExpressions,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// The land-mine is both included AND excluded by path (intersected with
	// type=stack), plus a reading filter that would otherwise trigger parsing.
	// The exclusion should win.
	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{
		"[HEAD~1...HEAD]",
		"!./live/land-mine | type=stack",
		"./live/land-mine | type=stack",
		"reading=live/normal/config.hcl",
	})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.FilterFlag)
	require.NoError(t, err)

	err = generate.GenerateStacks(t.Context(), l, opts, w)
	require.NoError(t, err)

	// The marker file should NOT exist — negation should prevent parsing
	// even though the land-mine is also positively included.
	_, statErr := os.Stat(markerFile)
	assert.ErrorIs(t, statErr, fs.ErrNotExist,
		"Land-mine stack was parsed (marker file created) despite being excluded by filter")
}

// runWorktreeDiscovery runs discovery with worktree phase enabled.
func runWorktreeDiscovery(
	t *testing.T,
	tmpDir string,
	gitExpressions filter.GitExpressions,
	cmd string,
	args []string,
) (component.Components, *worktrees.Worktrees) {
	t.Helper()

	l := logger.CreateLogger()

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: gitExpressions})
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

	discovery := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w).
		WithFilters(filters)

	components, err := discovery.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	return components, w
}

// TestWorktreePhase_Integration_StackReadingChanges_Units tests that when a file
// referenced via read_terragrunt_config() changes, the units within the generated stack
// are discovered (not just the stack component itself). This is the actual bug from #5681:
// the stack was discovered but its units were not, resulting in "No units discovered."
func TestWorktreePhase_Integration_StackReadingChanges_Units(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a catalog unit that the stack will reference as a source
	catalogUnitDir := filepath.Join(tmpDir, "catalog", "units", "myapp")
	err := os.MkdirAll(catalogUnitDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogUnitDir, "terragrunt.hcl"), []byte(`# catalog unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogUnitDir, "main.tf"), []byte(`
variable "example" {
  type    = string
  default = "ok"
}

output "example" {
  value = var.example
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create catalog unit")

	// Create a stack that reads an external config file via read_terragrunt_config
	stackDir := filepath.Join(tmpDir, "live", "app-stack")
	err = os.MkdirAll(stackDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackDir, "config.hcl"), []byte(`inputs = { example = "v1" }`), 0o644)
	require.NoError(t, err)

	stackContent := `
locals {
  config = read_terragrunt_config("config.hcl")
}

unit "myapp" {
  source = "${get_repo_root()}/catalog/units/myapp"
  path   = "myapp"
  values = local.config.inputs
}
`
	err = os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(stackContent), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Create stack with read_terragrunt_config")

	// Change ONLY the read file (not the stack file itself)
	err = os.WriteFile(filepath.Join(stackDir, "config.hcl"), []byte(`inputs = { example = "v2" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Update read config file only")

	// Set up worktrees and generate stacks
	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: gitExpressions,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.FilterFlag)
	require.NoError(t, err)

	// Generate stacks in both worktrees
	for _, pair := range w.WorktreePairs {
		fromOpts := opts.Clone()
		fromOpts.WorkingDir = pair.FromWorktree.Path
		fromOpts.RootWorkingDir = pair.FromWorktree.Path
		err = generate.GenerateStacks(t.Context(), l, fromOpts, w)
		require.NoError(t, err)

		toOpts := opts.Clone()
		toOpts.WorkingDir = pair.ToWorktree.Path
		toOpts.RootWorkingDir = pair.ToWorktree.Path
		err = generate.GenerateStacks(t.Context(), l, toOpts, w)
		require.NoError(t, err)
	}

	// Run discovery
	discoveryContext := &component.DiscoveryContext{
		WorkingDir: tmpDir,
		Cmd:        "plan",
	}

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(discoveryContext).
		WithWorktrees(w)

	filters := make(filter.Filters, 0, len(gitExpressions))
	for _, gitExpr := range gitExpressions {
		f := filter.NewFilter(gitExpr, gitExpr.String())
		filters = append(filters, f)
	}

	d = d.WithFilters(filters)

	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	// Collect component paths and kinds for debugging
	componentPaths := make([]string, 0, len(components))
	unitPaths := make([]string, 0)

	for _, c := range components {
		componentPaths = append(componentPaths, c.Path())
		if _, ok := c.(*component.Unit); ok {
			unitPaths = append(unitPaths, c.Path())
		}
	}

	// The critical assertion: at least one UNIT should be discovered.
	// The bug (#5681) is that only the Stack component is discovered, not its units.
	assert.NotEmpty(t, unitPaths,
		"Expected at least one unit to be discovered when a read file changes, "+
			"but got no units. All components: %v", componentPaths)
}

// TestWorktreePhase_Integration_NegatedFiltersAppliedInWorktreeSubDiscoveries tests that
// negated path filters (e.g., from .terragrunt-filters) are applied within worktree
// sub-discoveries, not just during the final filter evaluation.
// This is a regression test for #5821: source catalog units in worktrees were being
// discovered and parsed despite being excluded by a negated path filter, because the
// worktree sub-discoveries did not receive the exclusion filters.
func TestWorktreePhase_Integration_NegatedFiltersAppliedInWorktreeSubDiscoveries(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create two units: one that should be discovered and one that should be excluded.
	createUnit(t, tmpDir, "app", `# App unit`)
	// This catalog unit references values.* — it's a template that only works
	// when generated through a stack. Without the fix, the worktree sub-discovery
	// tries to parse it and fails with "Unknown variable: values".
	createUnit(t, tmpDir, "catalog/units/svc", `
locals {
  environment = values.environment
}
`)

	commitChanges(t, runner, "Initial commit")

	// Modify both units
	err := os.WriteFile(filepath.Join(tmpDir, "app", "terragrunt.hcl"), []byte(`# Modified app`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "catalog", "units", "svc", "terragrunt.hcl"), []byte(`
locals {
  environment = values.environment
  region      = "us-east-1"
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Modify both units")

	l := logger.CreateLogger()

	// Parse filters: git expression + negated path that excludes catalog
	filterQueries := []string{"[HEAD~1...HEAD]", "!./catalog/**"}
	filters, parseErr := filter.ParseFilterQueries(l, filterQueries)
	require.NoError(t, parseErr)

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: filters.UniqueGitFilters(),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{
			WorkingDir: tmpDir,
			Cmd:        "plan",
		}).
		WithWorktrees(w).
		WithRelationships().
		WithFilters(filters)

	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	unitPaths := components.Filter(component.UnitKind).Paths()

	worktreePair := w.WorktreePairs["[HEAD~1...HEAD]"]
	require.NotEmpty(t, worktreePair)

	toWorktree := worktreePair.ToWorktree.Path

	// The app unit should be discovered (it's in the git diff and not excluded)
	assert.Contains(t, unitPaths, filepath.Join(toWorktree, "app"),
		"app unit should be discovered")

	// The catalog unit should NOT be discovered (excluded by !./catalog/**)
	for _, p := range unitPaths {
		assert.NotContains(t, p, "catalog",
			"catalog units should be excluded by the negated filter, but found: %s", p)
	}
}

// TestWorktreePhase_Integration_GitFilterExclusionPreventsParsingInWorktree verifies that
// a negated filter prevents excluded units from being parsed inside worktree sub-discoveries.
// The land-mine unit uses run_cmd("exit 1") which would cause a fatal parse error if evaluated.
func TestWorktreePhase_Integration_GitFilterExclusionPreventsParsingInWorktree(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	createUnit(t, tmpDir, "app", `# App unit`)
	createUnit(t, tmpDir, "land-mine", `
locals {
  boom = run_cmd("--terragrunt-quiet", "bash", "-c", "exit 1")
}
`)

	commitChanges(t, runner, "Initial commit")

	err := os.WriteFile(filepath.Join(tmpDir, "app", "terragrunt.hcl"), []byte(`# Modified app`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(tmpDir, "land-mine", "terragrunt.hcl"), []byte(`
locals {
  boom = run_cmd("--terragrunt-quiet", "bash", "-c", "exit 1")
  modified = true
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Modify both units")

	l := logger.CreateLogger()
	filters, parseErr := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]", "!./land-mine"})
	require.NoError(t, parseErr)

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: filters.UniqueGitFilters(),
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	d := discovery.NewDiscovery(tmpDir).
		WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: tmpDir, Cmd: "plan"}).
		WithWorktrees(w).
		WithRelationships().
		WithFilters(filters)

	// If the land-mine unit is parsed, run_cmd("exit 1") causes a fatal error.
	components, err := d.Discover(t.Context(), l, opts)
	require.NoError(t, err)

	unitPaths := components.Filter(component.UnitKind).Paths()

	toWorktree := w.WorktreePairs["[HEAD~1...HEAD]"].ToWorktree.Path
	assert.Contains(t, unitPaths, filepath.Join(toWorktree, "app"), "app should be discovered")

	for _, p := range unitPaths {
		assert.NotContains(t, p, "land-mine", "land-mine should not be discovered: %s", p)
	}
}

// TestWorktreePhase_Integration_StackDiscoveryDoesNotParseUnits verifies that
// discoverStacks (via GenerateStacks) does not parse non-stack components even when
// reading filters trigger the parse phase. The land-mine unit uses run_cmd("exit 1")
// which would cause a fatal error if parsed.
func TestWorktreePhase_Integration_StackDiscoveryDoesNotParseUnits(t *testing.T) {
	t.Parallel()

	tmpDir, runner := setupGitRepo(t)

	// Create a land-mine unit at the repo root
	createUnit(t, tmpDir, "land-mine", `
locals {
  boom = run_cmd("--terragrunt-quiet", "bash", "-c", "exit 1")
}
`)

	// Create a catalog unit for the stack to source
	catalogDir := filepath.Join(tmpDir, "catalog", "units", "myapp")
	err := os.MkdirAll(catalogDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogDir, "terragrunt.hcl"), []byte(`# catalog unit`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(catalogDir, "main.tf"), []byte(`output "ok" { value = "ok" }`), 0o644)
	require.NoError(t, err)

	// Create a stack that reads a config file (triggers reading filter path in worktreeStacksToGenerate)
	stackDir := filepath.Join(tmpDir, "live", "stack")
	err = os.MkdirAll(stackDir, 0o755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackDir, "config.hcl"), []byte(`inputs = { v = "v1" }`), 0o644)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(stackDir, "terragrunt.stack.hcl"), []byte(`
locals {
  config = read_terragrunt_config("config.hcl")
}

unit "myapp" {
  source = "${get_repo_root()}/catalog/units/myapp"
  path   = "myapp"
  values = local.config.inputs
}
`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Initial commit")

	// Change only the config file (triggers reading filter, not a direct stack change)
	err = os.WriteFile(filepath.Join(stackDir, "config.hcl"), []byte(`inputs = { v = "v2" }`), 0o644)
	require.NoError(t, err)

	commitChanges(t, runner, "Update config file")

	l := logger.CreateLogger()
	gitExpressions := filter.GitExpressions{filter.NewGitExpression("HEAD~1", "HEAD")}

	w, err := worktrees.NewWorktrees(t.Context(), l, worktrees.WorktreeOpts{
		WorkingDir:     tmpDir,
		GitExpressions: gitExpressions,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.WithoutCancel(t.Context()), l)
		require.NoError(t, cleanupErr)
	})

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	parsedFilters, parseErr := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, parseErr)

	opts.Filters = parsedFilters
	opts.Experiments = experiment.NewExperiments()
	err = opts.Experiments.EnableExperiment(experiment.FilterFlag)
	require.NoError(t, err)

	// GenerateStacks internally calls discoverStacks with reading filters.
	// If the land-mine unit is parsed, run_cmd("exit 1") causes a fatal error.
	err = generate.GenerateStacks(t.Context(), l, opts, w)
	require.NoError(t, err)
}
