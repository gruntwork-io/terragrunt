package worktrees_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unexpandableDir is a directory name the filter glob cannot compile.
const unexpandableDir = "bad[dir"

func TestNewWorktrees(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)
	require.NoError(t, runner.Commit(t.Context(), "Initial commit", "--allow-empty"))
	require.NoError(t, runner.Commit(t.Context(), "Second commit", "--allow-empty"))

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(),
		worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.Background(), logger.CreateLogger(), venvtest.NewOSWithEmptyEnv())
		require.NoError(t, cleanupErr)
	})

	require.NotEmpty(t, w.WorktreePairs)
}

// TestNewWorktreesWithSymlinkOutsideRepository pins that a tracked symlink
// pointing outside the repository does not keep a reference from being
// materialized. Git checks the link out as it is committed, writing it the
// way any clone or `git worktree add` would.
func TestNewWorktreesWithSymlinkOutsideRepository(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("creating a symlink on Windows takes a privilege the runner may not have")
	}

	tmpDir := helpers.TmpDirWOSymlinks(t)
	outside := filepath.Join(helpers.TmpDirWOSymlinks(t), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("outside\n"), 0o600))

	runner := helpers.InitTestGitRunner(t, tmpDir)

	unitDir := filepath.Join(tmpDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte("inputs = {}\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(tmpDir, "link")))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte("# changed\n"), 0o600))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Second commit"))

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv()

	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		v,
		worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.Background(), logger.CreateLogger(), v))
	})

	materialized := 0

	for _, pair := range w.WorktreePairs {
		for _, worktree := range []worktrees.Worktree{pair.FromWorktree, pair.ToWorktree} {
			if worktree.Path == "" {
				continue
			}

			materialized++

			target, err := os.Readlink(filepath.Join(worktree.Path, "link"))
			require.NoError(t, err)
			assert.Equal(t, outside, target)
			assert.FileExists(t, filepath.Join(worktree.Path, "unit", "terragrunt.hcl"))
		}
	}

	assert.Positive(t, materialized)
}

// TestNewWorktreesForSeveralRefsWithRacing materializes several references at
// once, which is what the concurrent worktree creation has to get right.
func TestNewWorktreesForSeveralRefsWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, tmpDir)

	for i := range 3 {
		name := fmt.Sprintf("unit-%d", i)

		require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, name), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(tmpDir, name, "terragrunt.hcl"),
			[]byte("inputs = {}\n"),
			0o600,
		))
		require.NoError(t, runner.Add(t.Context(), "."))
		require.NoError(t, runner.Commit(t.Context(), "Commit "+name))
	}

	filters, err := filter.ParseFilterQueries(
		logger.CreateLogger(),
		[]string{"[HEAD~2...HEAD~1]", "[HEAD~1...HEAD]"},
	)
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(),
		worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, w.Cleanup(context.Background(), logger.CreateLogger(), venvtest.NewOSWithEmptyEnv()))
	})

	require.Len(t, w.WorktreePairs, 2)

	// A worktree that was never created has no path: these commits only add
	// units, so nothing reads the "from" side of either diff.
	materialized := 0

	for _, pair := range w.WorktreePairs {
		for _, worktree := range []worktrees.Worktree{pair.FromWorktree, pair.ToWorktree} {
			if worktree.Path == "" {
				continue
			}

			materialized++

			assert.FileExists(t, filepath.Join(worktree.Path, "unit-0", "terragrunt.hcl"))
		}
	}

	assert.Positive(t, materialized)
}

// TestNewWorktreesFilteredPathsOnly pins how much of a reference is checked
// out. A caller that only locates the components a filter names gets those
// directories, and anything that parses configurations gets the whole tree.
func TestNewWorktreesFilteredPathsOnly(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		files       map[string]string
		name        string
		changed     string
		experiments []string
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name: "a changed unit configuration selects units by path",
			files: map[string]string{
				"unit/terragrunt.hcl":  "inputs = {}\n",
				"other/terragrunt.hcl": "inputs = {}\n",
				"modules/app/main.tf":  "",
			},
			changed:     "unit/terragrunt.hcl",
			wantPresent: []string{"unit/terragrunt.hcl"},
			wantAbsent:  []string{"other/terragrunt.hcl", "modules/app/main.tf"},
		},
		{
			// Pathspecs are '/'-separated on every platform, so a nested unit
			// must not be checked out under its OS spelling.
			name: "a changed unit configuration in a nested directory selects units by path",
			files: map[string]string{
				"nested/changed/terragrunt.hcl":   "inputs = {}\n",
				"nested/unchanged/terragrunt.hcl": "inputs = {}\n",
			},
			changed:     "nested/changed/terragrunt.hcl",
			wantPresent: []string{"nested/changed/terragrunt.hcl"},
			wantAbsent:  []string{"nested/unchanged/terragrunt.hcl"},
		},
		{
			name: "a changed file beside a unit selects it by path",
			files: map[string]string{
				"unit/terragrunt.hcl":  "inputs = {}\n",
				"unit/main.tf":         "",
				"other/terragrunt.hcl": "inputs = {}\n",
			},
			changed:     "unit/main.tf",
			wantPresent: []string{"unit/terragrunt.hcl", "unit/main.tf"},
			wantAbsent:  []string{"other/terragrunt.hcl"},
		},
		{
			name: "a changed file no unit sits beside selects units by what they read",
			files: map[string]string{
				"unit/terragrunt.hcl": "inputs = {}\n",
				"modules/app/main.tf": "",
			},
			changed:     "modules/app/main.tf",
			wantPresent: []string{"unit/terragrunt.hcl", "modules/app/main.tf"},
		},
		{
			// Discovery following a symlink walks wherever it points, which a
			// worktree of some directories cannot answer for.
			name: "symlinks are followed out of the checked-out directories",
			files: map[string]string{
				"unit/terragrunt.hcl":  "inputs = {}\n",
				"other/terragrunt.hcl": "inputs = {}\n",
			},
			changed:     "unit/terragrunt.hcl",
			experiments: []string{experiment.Symlinks},
			wantPresent: []string{"unit/terragrunt.hcl", "other/terragrunt.hcl"},
		},
		{
			name: "a stack in the tree is generated by parsing it",
			files: map[string]string{
				"unit/terragrunt.hcl":       "inputs = {}\n",
				"live/terragrunt.stack.hcl": "unit \"app\" {\n  source = \"../unit\"\n  path   = \"app\"\n}\n",
			},
			changed:     "unit/terragrunt.hcl",
			wantPresent: []string{"unit/terragrunt.hcl", "live/terragrunt.stack.hcl"},
		},
		{
			name: "a unit at the repository root checks out the whole tree",
			files: map[string]string{
				"terragrunt.hcl":       "inputs = {}\n",
				"other/terragrunt.hcl": "inputs = {}\n",
			},
			changed:     "terragrunt.hcl",
			wantPresent: []string{"terragrunt.hcl", "other/terragrunt.hcl"},
		},
		{
			name: "a directory git reads as a pattern checks out the whole tree",
			files: map[string]string{
				"unit[1]/terragrunt.hcl": "inputs = {}\n",
				"other/terragrunt.hcl":   "inputs = {}\n",
			},
			changed:     "unit[1]/terragrunt.hcl",
			wantPresent: []string{"unit[1]/terragrunt.hcl", "other/terragrunt.hcl"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tmpDir := helpers.TmpDirWOSymlinks(t)

			runner := helpers.InitTestGitRunner(t, tmpDir)

			for name, content := range tc.files {
				path := filepath.Join(tmpDir, filepath.FromSlash(name))

				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
				require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			}

			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

			changed := filepath.Join(tmpDir, filepath.FromSlash(tc.changed))
			require.NoError(t, os.WriteFile(changed, []byte("# changed\n"), 0o600))
			require.NoError(t, runner.Add(t.Context(), "."))
			require.NoError(t, runner.Commit(t.Context(), "Second commit"))

			filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"[HEAD~1...HEAD]"})
			require.NoError(t, err)

			v := venvtest.NewOSWithEmptyEnv()

			experiments := experiment.NewExperiments()

			for _, name := range tc.experiments {
				require.NoError(t, experiments.EnableExperiment(name))
			}

			w, err := worktrees.NewWorktrees(t.Context(), logger.CreateLogger(), v, worktrees.WorktreeOpts{
				WorkingDir:        tmpDir,
				GitExpressions:    filters.UniqueGitFilters(),
				Experiments:       experiments,
				FilteredPathsOnly: true,
			})
			require.NoError(t, err)

			t.Cleanup(func() {
				require.NoError(t, w.Cleanup(context.Background(), logger.CreateLogger(), v))
			})

			materialized := 0

			for _, pair := range w.WorktreePairs {
				for _, worktree := range []worktrees.Worktree{pair.FromWorktree, pair.ToWorktree} {
					if worktree.Path == "" {
						continue
					}

					materialized++

					for _, present := range tc.wantPresent {
						assert.FileExists(t, filepath.Join(worktree.Path, filepath.FromSlash(present)))
					}

					for _, absent := range tc.wantAbsent {
						assert.NoFileExists(t, filepath.Join(worktree.Path, filepath.FromSlash(absent)))
					}
				}
			}

			assert.Positive(t, materialized)
		})
	}
}

func TestNewWorktreesWithInvalidReference(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)

	// Initialize Git repository
	runner := helpers.InitTestGitRunner(t, tmpDir)
	require.NoError(t, runner.Commit(t.Context(), "Initial commit", "--allow-empty"))

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Parse filter with invalid Git reference
	filters, err := filter.ParseFilterQueries(
		logger.CreateLogger(),
		[]string{"[nonexistent-branch]"},
	)
	require.NoError(t, err) // Parsing should succeed

	_, err = worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(),
		worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()},
	)
	require.Error(t, err)
}

func TestExpressionExpansion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		diffs              *git.Diffs
		name               string
		expectedToPaths    []string
		expectedToReadings []string
		expectedFrom       int
		expectedTo         int
	}{
		{
			name: "removed terragrunt.hcl files create from filters",
			diffs: &git.Diffs{
				Removed: []string{
					"app1/terragrunt.hcl",
					"app2/terragrunt.hcl",
				},
			},
			expectedFrom:       2,
			expectedTo:         0,
			expectedToPaths:    []string{},
			expectedToReadings: []string{},
		},
		{
			name: "added terragrunt.hcl files create to filters",
			diffs: &git.Diffs{
				Added: []string{
					"app1/terragrunt.hcl",
					"app2/terragrunt.hcl",
				},
			},
			expectedFrom:       0,
			expectedTo:         2,
			expectedToPaths:    []string{"app1", "app2"},
			expectedToReadings: []string{},
		},
		{
			name: "added stack file selects the stack and everything under it",
			diffs: &git.Diffs{
				Added: []string{"stack/terragrunt.stack.hcl"},
			},
			expectedFrom:       0,
			expectedTo:         2,
			expectedToPaths:    []string{"stack", "stack/**"},
			expectedToReadings: []string{},
		},
		{
			name: "changed terragrunt.hcl files create to filters",
			diffs: &git.Diffs{
				Changed: []string{
					"app1/terragrunt.hcl",
					"app2/terragrunt.hcl",
				},
			},
			expectedFrom:       0,
			expectedTo:         2,
			expectedToPaths:    []string{"app1", "app2"},
			expectedToReadings: []string{},
		},
		{
			name: "changed non-terragrunt.hcl files create reading filters",
			diffs: &git.Diffs{
				Changed: []string{
					"app1/main.tf",
					"app1/variables.tf",
					"app2/data.tf",
				},
			},
			expectedFrom:       0,
			expectedTo:         3,
			expectedToPaths:    []string{},
			expectedToReadings: []string{"app1/main.tf", "app1/variables.tf", "app2/data.tf"},
		},
		{
			name: "changed stack files create reading filters",
			diffs: &git.Diffs{
				Changed: []string{
					"stack/terragrunt.stack.hcl",
				},
			},
			expectedFrom:       0,
			expectedTo:         1,
			expectedToPaths:    []string{},
			expectedToReadings: []string{"stack/terragrunt.stack.hcl"},
		},
		{
			name: "mixed file types create appropriate filters",
			diffs: &git.Diffs{
				Removed: []string{
					"app-removed/terragrunt.hcl",
				},
				Added: []string{
					"app-added/terragrunt.hcl",
				},
				Changed: []string{
					"app-modified/terragrunt.hcl",
					"app-modified/main.tf",
					"stack/terragrunt.stack.hcl",
					"other/file.hcl",
				},
			},
			expectedFrom:    1,
			expectedTo:      5,
			expectedToPaths: []string{"app-added", "app-modified"},
			expectedToReadings: []string{
				"app-modified/main.tf",
				"stack/terragrunt.stack.hcl",
				"other/file.hcl",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wp := &worktrees.WorktreePair{Diffs: tt.diffs}

			fromFilters, toFilters, err := wp.Expand(treePaths())
			require.NoError(t, err)

			// Verify from filters count
			assert.Len(t, fromFilters, tt.expectedFrom, "From filters count should match")

			// Verify to filters count
			assert.Len(t, toFilters, tt.expectedTo, "To filters count should match")

			// Verify from filters are path filters with correct paths
			for i, f := range fromFilters {
				pathExpr, ok := f.Expression().(*filter.PathExpression)
				require.True(t, ok, "From filter %d should be a PathExpression", i)
				expectedPath := filepath.Dir(tt.diffs.Removed[i])
				assert.Equal(
					t,
					expectedPath,
					pathExpr.Value,
					"From filter %d should have correct path",
					i,
				)
			}

			// Verify to filters
			toPaths := []string{}
			toReadings := []string{}

			for _, f := range toFilters {
				switch expr := f.Expression().(type) {
				case *filter.PathExpression:
					toPaths = append(toPaths, expr.Value)
				case *filter.AttributeExpression:
					if expr.Key == filter.AttributeReading {
						toReadings = append(toReadings, expr.Value)
					}
				}
			}

			// Verify path filters
			assert.ElementsMatch(t, tt.expectedToPaths, toPaths, "To path filters should match")

			// Verify reading filters
			assert.ElementsMatch(
				t,
				tt.expectedToReadings,
				toReadings,
				"To reading filters should match",
			)
		})
	}
}

func TestExpansionAttributeReadingFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		diffs            *git.Diffs
		expectedReadings []string
	}{
		{
			name: "changed .tf file creates reading filter",
			diffs: &git.Diffs{
				Changed: []string{
					"app/main.tf",
				},
			},
			expectedReadings: []string{"app/main.tf"},
		},
		{
			name: "changed .hcl file (not terragrunt.hcl) creates reading filter",
			diffs: &git.Diffs{
				Changed: []string{
					"app/config.hcl",
				},
			},
			expectedReadings: []string{"app/config.hcl"},
		},
		{
			name: "changed file in subdirectory creates reading filter with correct path",
			diffs: &git.Diffs{
				Changed: []string{
					"app/modules/database/main.tf",
				},
			},
			expectedReadings: []string{"app/modules/database/main.tf"},
		},
		{
			name: "multiple changed files create multiple reading filters",
			diffs: &git.Diffs{
				Changed: []string{
					"app1/main.tf",
					"app1/variables.tf",
					"app2/data.tf",
					"app2/outputs.tf",
				},
			},
			expectedReadings: []string{
				"app1/main.tf",
				"app1/variables.tf",
				"app2/data.tf",
				"app2/outputs.tf",
			},
		},
		{
			name: "mixed terragrunt.hcl and other files",
			diffs: &git.Diffs{
				Changed: []string{
					"app/terragrunt.hcl",
					"app/main.tf",
					"app/variables.tf",
				},
			},
			expectedReadings: []string{
				"app/main.tf",
				"app/variables.tf",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wp := &worktrees.WorktreePair{Diffs: tt.diffs}

			_, toFilters, err := wp.Expand(treePaths())
			require.NoError(t, err)

			// Extract reading filters
			readings := []string{}

			for _, f := range toFilters {
				if attrExpr, ok := f.Expression().(*filter.AttributeExpression); ok {
					if attrExpr.Key == filter.AttributeReading {
						readings = append(readings, attrExpr.Value)
					}
				}
			}

			// Verify reading filters match expected
			assert.ElementsMatch(
				t,
				tt.expectedReadings,
				readings,
				"Reading filters should match expected paths",
			)

			// Verify each reading filter is properly constructed
			for _, expectedReading := range tt.expectedReadings {
				found := false

				for _, f := range toFilters {
					if attrExpr, ok := f.Expression().(*filter.AttributeExpression); ok {
						if attrExpr.Key == filter.AttributeReading &&
							attrExpr.Value == expectedReading {
							found = true

							assert.Equal(
								t,
								"reading",
								attrExpr.Key,
								"Filter should have reading key",
							)
							assert.Equal(
								t,
								expectedReading,
								attrExpr.Value,
								"Filter should have correct file path",
							)

							break
						}
					}
				}

				assert.True(
					t,
					found,
					"Expected reading filter for %s should be present",
					expectedReading,
				)
			}
		})
	}
}

func TestExpandWithUnitDirectoryDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		diffs                *git.Diffs
		name                 string
		toTree               []string
		expectedToPaths      []string
		expectedToReadings   []string
		expectedFromReadings []string
		expectedFrom         int
	}{
		{
			name:   "removed file in unit directory creates path filter",
			toTree: []string{"unit1/terragrunt.hcl"},
			diffs: &git.Diffs{
				Removed: []string{
					"unit1/main.tf",
				},
			},
			expectedToPaths:    []string{"unit1"},
			expectedToReadings: []string{},
			expectedFrom:       0,
		},
		{
			name:   "removed file in non-unit directory creates reading filter against from worktree",
			toTree: []string{"non-unit/some-file.tf"},
			diffs: &git.Diffs{
				Removed: []string{
					"non-unit/some-file.tf",
				},
			},
			expectedToPaths:      []string{},
			expectedToReadings:   []string{},
			expectedFromReadings: []string{"non-unit/some-file.tf"},
			expectedFrom:         1,
		},
		{
			name:   "added file in unit directory creates path filter",
			toTree: []string{"unit1/terragrunt.hcl"},
			diffs: &git.Diffs{
				Added: []string{
					"unit1/variables.tf",
				},
			},
			expectedToPaths:    []string{"unit1"},
			expectedToReadings: []string{},
			expectedFrom:       0,
		},
		{
			name:   "added file in non-unit directory creates reading filter against to worktree",
			toTree: []string{"non-unit/some-file.tf"},
			diffs: &git.Diffs{
				Added: []string{
					"non-unit/new-file.tf",
				},
			},
			expectedToPaths:    []string{},
			expectedToReadings: []string{"non-unit/new-file.tf"},
			expectedFrom:       0,
		},
		{
			name:   "changed file in unit directory creates path filter",
			toTree: []string{"unit1/terragrunt.hcl"},
			diffs: &git.Diffs{
				Changed: []string{
					"unit1/main.tf",
				},
			},
			expectedToPaths:    []string{"unit1"},
			expectedToReadings: []string{},
			expectedFrom:       0,
		},
		{
			name:   "changed file in non-unit directory creates reading filter",
			toTree: []string{"non-unit/some-file.tf"},
			diffs: &git.Diffs{
				Changed: []string{
					"non-unit/some-file.tf",
				},
			},
			expectedToPaths:    []string{},
			expectedToReadings: []string{"non-unit/some-file.tf"},
			expectedFrom:       0,
		},
		{
			name: "mixed scenarios with multiple units and non-units",
			toTree: []string{
				"unit1/terragrunt.hcl",
				"unit2/terragrunt.hcl",
				"non-unit/shared.tf",
			},
			diffs: &git.Diffs{
				Removed: []string{
					"unit1/old-file.tf",
				},
				Added: []string{
					"unit2/new-file.tf",
				},
				Changed: []string{
					"unit1/modified.tf",
					"non-unit/shared.tf",
				},
			},
			expectedToPaths:    []string{"unit1", "unit2"},
			expectedToReadings: []string{"non-unit/shared.tf"},
			expectedFrom:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			wp := &worktrees.WorktreePair{Diffs: tt.diffs}

			fromFilters, toFilters, err := wp.Expand(treePaths(tt.toTree...))
			require.NoError(t, err)

			// Verify from filters count
			assert.Len(t, fromFilters, tt.expectedFrom, "From filters count should match")

			// Extract reading filters from fromFilters
			fromReadings := []string{}

			for _, f := range fromFilters {
				if expr, ok := f.Expression().(*filter.AttributeExpression); ok &&
					expr.Key == filter.AttributeReading {
					fromReadings = append(fromReadings, expr.Value)
				}
			}

			assert.ElementsMatch(
				t,
				tt.expectedFromReadings,
				fromReadings,
				"From reading filters should match",
			)

			// Extract path and reading filters from toFilters
			toPathsMap := make(map[string]bool)
			toReadings := []string{}

			for _, f := range toFilters {
				switch expr := f.Expression().(type) {
				case *filter.PathExpression:
					toPathsMap[expr.Value] = true
				case *filter.AttributeExpression:
					if expr.Key == filter.AttributeReading {
						toReadings = append(toReadings, expr.Value)
					}
				}
			}

			// Convert map to slice for comparison (deduplicates)
			toPaths := make([]string, 0, len(toPathsMap))
			for path := range toPathsMap {
				toPaths = append(toPaths, path)
			}

			// Verify path filters
			assert.ElementsMatch(t, tt.expectedToPaths, toPaths, "To path filters should match")

			// Verify reading filters
			assert.ElementsMatch(
				t,
				tt.expectedToReadings,
				toReadings,
				"To reading filters should match",
			)
		})
	}
}

// TestWorktreeCleanup test worktree cleanup
func TestWorktreeCleanup(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Initialize Git repository
	runner := helpers.InitTestGitRunner(t, tmpDir)

	for i := range 3 {
		require.NoError(t, runner.Commit(t.Context(), fmt.Sprintf("Commit %d", i), "--allow-empty"))
	}

	opts := options.NewTerragruntOptions(vexec.NewOSExec())
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	filters, err := filter.ParseFilterQueries(
		logger.CreateLogger(),
		[]string{"[test-worktree-cleanup]"},
	)
	require.NoError(t, err)

	_, err = worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		venvtest.NewOSWithEmptyEnv(),
		worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()},
	)
	require.Error(t, err)

	tempDir := os.TempDir()

	worktreeDirs, err := filepath.Glob(filepath.Join(tempDir, "terragrunt-worktree-*"))
	require.NoError(t, err)
	// validate that test-worktree-cleanup worktree was deleted
	worktreeExists := false

	for _, dir := range worktreeDirs {
		if strings.Contains(filepath.Base(dir), "test-worktree-cleanup") {
			worktreeExists = true
			break
		}
	}

	assert.False(t, worktreeExists, "Worktree test-worktree-cleanup should be deleted")
}

// TestNewWorktreesInterruptedCheckoutLeavesNoRegistrationWithRacing pins that a
// worktree registered before an interrupt is unregistered again. The context is
// cancelled as the checkout that fills the worktree starts.
func TestNewWorktreesInterruptedCheckoutLeavesNoRegistrationWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	commitUnits(t, tmpDir, 2)

	l := logger.CreateLogger()

	filters, err := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	v := venvtest.NewOSWithEmptyEnv()
	v = v.WithExec(&cancelOnCheckoutExec{Exec: v.Exec, cancel: cancel})

	_, err = worktrees.NewWorktrees(
		ctx,
		l,
		v,
		worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()},
	)
	require.ErrorIs(t, err, context.Canceled)

	assert.Empty(t, registeredWorktrees(t, tmpDir))
}

// TestWorktreeCleanupWithCancelledContextWithRacing pins that Cleanup removes
// worktrees when its context is already cancelled, as it is when an
// interrupted run cleans up.
func TestWorktreeCleanupWithCancelledContextWithRacing(t *testing.T) {
	t.Parallel()

	tmpDir := helpers.TmpDirWOSymlinks(t)
	commitUnits(t, tmpDir, 2)

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()

	filters, err := filter.ParseFilterQueries(l, []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(
		t.Context(),
		l,
		v,
		worktrees.WorktreeOpts{WorkingDir: tmpDir, GitExpressions: filters.UniqueGitFilters()},
	)
	require.NoError(t, err)
	require.NotEmpty(t, registeredWorktrees(t, tmpDir))

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	require.NoError(t, w.Cleanup(ctx, l, v))

	assert.Empty(t, registeredWorktrees(t, tmpDir))

	for _, pair := range w.WorktreePairs {
		for _, worktree := range []worktrees.Worktree{pair.FromWorktree, pair.ToWorktree} {
			if worktree.Path == "" {
				continue
			}

			assert.NoDirExists(t, worktree.Path)
		}
	}
}

// TestNewWorktreesWithoutExpressions pins that a run with no Git expressions needs no git at all.
func TestNewWorktreesWithoutExpressions(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")

	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		venvtest.New(),
		worktrees.WorktreeOpts{WorkingDir: root},
	)
	require.NoError(t, err)
	assert.Empty(t, w.WorktreePairs)
	assert.Equal(t, root, w.OriginalWorkingDir)
}

// TestGitNotFound pins that creating and cleaning up worktrees both report a missing git binary.
func TestGitNotFound(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	v := venvtest.New().WithExec(vexec.NewMemExec(
		func(context.Context, vexec.Invocation) vexec.Result { return vexec.Result{} },
		vexec.WithLookPath(func(string) (string, error) { return "", errors.New("not found") }),
	))

	t.Run("create", func(t *testing.T) {
		t.Parallel()

		_, err := worktrees.NewWorktrees(t.Context(), logger.CreateLogger(), v, worktrees.WorktreeOpts{
			WorkingDir:     root,
			GitExpressions: gitExpressions(t),
		})
		require.ErrorIs(t, err, git.ErrGitCommand)
	})

	t.Run("cleanup", func(t *testing.T) {
		t.Parallel()

		w := worktreesAt(root, filepath.Join(root, "worktree"))

		require.ErrorIs(t, w.Cleanup(t.Context(), logger.CreateLogger(), v), git.ErrGitCommand)
	})
}

// TestNewWorktreesReportsSlowCreation pins the progress messages for one reference and for several.
func TestNewWorktreesReportsSlowCreation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		diff        string
		wantWorking string
		wantDone    string
	}{
		{
			name:        "one reference",
			diff:        "A\tunit/terragrunt.hcl\n",
			wantWorking: "Creating Git worktree for reference HEAD...",
			wantDone:    "Created Git worktree for reference HEAD",
		},
		{
			name:        "several references",
			diff:        "A\tunit/terragrunt.hcl\nD\told/terragrunt.hcl\n",
			wantWorking: "Creating Git worktrees for 2 references...",
			wantDone:    "Created Git worktrees for 2 references",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			synctest.Test(t, func(t *testing.T) {
				buf := new(bytes.Buffer)
				l := log.New(log.WithLevel(log.InfoLevel), log.WithOutput(buf))

				root := venvtest.Root("/repo")
				v := memRepoVenv(t, root, tc.diff, func(_ context.Context, inv vexec.Invocation) vexec.Result {
					// Sleeping in the worktree add would hold a mutex, which stops the bubble's clock.
					if slices.Contains(inv.Args, "checkout") {
						time.Sleep(2 * time.Second)
					}

					return vexec.Result{}
				})

				experiments := experiment.NewExperiments()
				require.NoError(t, experiments.EnableExperiment(experiment.SlowTaskReporting))

				_, err := worktrees.NewWorktrees(t.Context(), l, v, worktrees.WorktreeOpts{
					WorkingDir:     root,
					GitExpressions: gitExpressions(t),
					Experiments:    experiments,
				})
				require.NoError(t, err)

				assert.Contains(t, buf.String(), tc.wantWorking)
				assert.Contains(t, buf.String(), tc.wantDone)
			})
		})
	}
}

// TestNewWorktreesCreationFailures pins that a reference that cannot be materialized fails the run.
func TestNewWorktreesCreationFailures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		fails        func(args []string) bool
		name         string
		diff         string
		wantTempDirs int
	}{
		{
			name: "registering fails",
			diff: "A\tunit/terragrunt.hcl\n",
			fails: func(args []string) bool {
				return slices.Contains(args, "add")
			},
		},
		{
			name: "filling fails",
			diff: "A\tunit/terragrunt.hcl\n",
			fails: func(args []string) bool {
				return slices.Contains(args, "checkout")
			},
		},
		{
			name: "filling and unregistering both fail",
			diff: "A\tunit/terragrunt.hcl\n",
			fails: func(args []string) bool {
				return slices.Contains(args, "checkout") || slices.Contains(args, "remove")
			},
		},
		{
			name: "one reference fails and the other cannot be cleaned up",
			diff: "A\tunit/terragrunt.hcl\nD\told/terragrunt.hcl\n",
			fails: func(args []string) bool {
				return slices.Contains(args, "remove") || (slices.Contains(args, "add") && args[len(args)-1] == "HEAD")
			},
			wantTempDirs: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			root := venvtest.Root("/repo")
			v := memRepoVenv(t, root, tc.diff, func(_ context.Context, inv vexec.Invocation) vexec.Result {
				if tc.fails(inv.Args) {
					return vexec.Result{ExitCode: 1}
				}

				return vexec.Result{}
			})

			_, err := worktrees.NewWorktrees(t.Context(), logger.CreateLogger(), v, worktrees.WorktreeOpts{
				WorkingDir:     root,
				GitExpressions: gitExpressions(t),
			})
			require.ErrorIs(t, err, git.ErrGitCommand)

			entries, err := vfs.ReadDir(v.FS, v.Platform.TempDir())
			require.NoError(t, err)
			assert.Len(t, entries, tc.wantTempDirs, "a failed reference should not leave its directory behind")
		})
	}
}

// TestNewWorktreesTempDirFailure pins that a temporary directory that cannot be created fails the run.
func TestNewWorktreesTempDirFailure(t *testing.T) {
	t.Parallel()

	root := helpers.TmpDirWOSymlinks(t)
	notADir := filepath.Join(root, "not-a-directory")

	require.NoError(t, os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o600))
	require.NoError(t, os.WriteFile(notADir, nil, 0o600))

	v := venvtest.NewWithOSFS().
		WithHandler(memRepoHandler("A\tunit/terragrunt.hcl\n", noopGit)).
		WithTempDir(func() string { return notADir })

	_, err := worktrees.NewWorktrees(t.Context(), logger.CreateLogger(), v, worktrees.WorktreeOpts{
		WorkingDir:     root,
		GitExpressions: gitExpressions(t),
	})
	require.Error(t, err)
}

// TestNewWorktreesUnexpandableDiff pins that a diff path no filter can match fails the run instead of dropping it.
func TestNewWorktreesUnexpandableDiff(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	v := memRepoVenv(t, root, "A\t"+unexpandableDir+"/terragrunt.hcl\n", noopGit)

	_, err := worktrees.NewWorktrees(t.Context(), logger.CreateLogger(), v, worktrees.WorktreeOpts{
		WorkingDir:     root,
		GitExpressions: gitExpressions(t),
	})
	require.Error(t, err)
}

// TestNewWorktreesRecordsDiffTelemetry pins the diff sizes reported to the configured meter.
func TestNewWorktreesRecordsDiffTelemetry(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	buf := new(bytes.Buffer)

	tlm, err := telemetry.NewTelemeter(t.Context(), l, "terragrunt", "v0.0.0-test", buf, &telemetry.Options{
		MetricExporter: "console",
	}, false)
	require.NoError(t, err)

	diff := "A\ta/terragrunt.hcl\nA\tb/terragrunt.hcl\n" +
		"D\tc/terragrunt.hcl\n" +
		"M\td/terragrunt.hcl\nM\te/terragrunt.hcl\nM\tf/terragrunt.hcl\n"

	root := venvtest.Root("/repo")
	v := memRepoVenv(t, root, diff, noopGit)

	ctx := telemetry.ContextWithTelemeter(t.Context(), tlm)

	_, err = worktrees.NewWorktrees(ctx, l, v, worktrees.WorktreeOpts{
		WorkingDir:     root,
		GitExpressions: gitExpressions(t),
	})
	require.NoError(t, err)
	require.NoError(t, tlm.Shutdown(ctx))

	counts := metricCounts(t, buf)

	assert.Equal(t, int64(2), counts["git_diff_files_added_count"])
	assert.Equal(t, int64(1), counts["git_diff_files_removed_count"])
	assert.Equal(t, int64(3), counts["git_diff_files_changed_count"])
}

// TestNewWorktreesStackFileMaterializesBothReferences pins that an added stack file selects both sides of the diff.
func TestNewWorktreesStackFileMaterializesBothReferences(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	v := memRepoVenv(t, root, "A\tstack/terragrunt.stack.hcl\n", noopGit)

	w, err := worktrees.NewWorktrees(t.Context(), logger.CreateLogger(), v, worktrees.WorktreeOpts{
		WorkingDir:     root,
		GitExpressions: gitExpressions(t),
	})
	require.NoError(t, err)
	require.Len(t, w.WorktreePairs, 1)

	for _, pair := range w.WorktreePairs {
		assert.NotEmpty(t, pair.FromWorktree.Path)
		assert.NotEmpty(t, pair.ToWorktree.Path)
	}
}

// TestExpandRejectsPathsThatDoNotCompile pins that every place a diff path becomes a filter reports a bad path.
func TestExpandRejectsPathsThatDoNotCompile(t *testing.T) {
	t.Parallel()

	unitConfig := unexpandableDir + "/terragrunt.hcl"
	stackFile := unexpandableDir + "/terragrunt.stack.hcl"
	beside := unexpandableDir + "/main.tf"

	testCases := []struct {
		diffs *git.Diffs
		tree  git.TreePaths
		name  string
	}{
		{name: "removed unit", diffs: &git.Diffs{Removed: []string{unitConfig}}},
		{name: "added unit", diffs: &git.Diffs{Added: []string{unitConfig}}},
		{name: "added stack", diffs: &git.Diffs{Added: []string{stackFile}}},
		{name: "added file beside a unit", diffs: &git.Diffs{Added: []string{beside}}, tree: treePaths(unitConfig)},
		{name: "added file read by a unit", diffs: &git.Diffs{Added: []string{beside}}},
		{name: "changed unit", diffs: &git.Diffs{Changed: []string{unitConfig}}},
		{name: "changed file beside a unit", diffs: &git.Diffs{Changed: []string{beside}}, tree: treePaths(unitConfig)},
		{name: "changed file read by a unit", diffs: &git.Diffs{Changed: []string{beside}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pair := &worktrees.WorktreePair{Diffs: tc.diffs}

			_, _, err := pair.Expand(tc.tree)
			require.Error(t, err)
		})
	}
}

// TestWorktreesDisplayPath pins the translation of worktree paths back to the user's working directory.
func TestWorktreesDisplayPath(t *testing.T) {
	t.Parallel()

	repo := venvtest.Root("/repo")
	from := venvtest.Root("/tmp/from")
	to := venvtest.Root("/tmp/to")

	w := &worktrees.Worktrees{
		OriginalWorkingDir: repo,
		WorktreePairs: map[string]*worktrees.WorktreePair{
			"both sides": {
				FromWorktree: worktrees.Worktree{Ref: "HEAD~1", Path: from},
				ToWorktree:   worktrees.Worktree{Ref: "HEAD", Path: to},
			},
			"never materialized": {
				FromWorktree: worktrees.Worktree{Ref: "main"},
			},
		},
	}

	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "inside the from worktree", path: filepath.Join(from, "unit"), want: filepath.Join(repo, "unit")},
		{name: "inside the to worktree", path: filepath.Join(to, "a", "b"), want: filepath.Join(repo, "a", "b")},
		{name: "the worktree itself", path: to, want: repo},
		{name: "sibling sharing a prefix", path: to + "-other", want: to + "-other"},
		{name: "outside every worktree", path: filepath.Join(repo, "unit"), want: filepath.Join(repo, "unit")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, w.DisplayPath(tc.path))
		})
	}
}

// TestWorktreesStacks pins which side of the diff each changed stack is read from.
func TestWorktreesStacks(t *testing.T) {
	t.Parallel()

	from := venvtest.Root("/tmp/from")
	to := venvtest.Root("/tmp/to")

	readingAffected := worktrees.StackDiffChangedPair{
		FromStack: component.NewStack(filepath.Join(from, "reader")),
		ToStack:   component.NewStack(filepath.Join(to, "reader")),
	}

	w := &worktrees.Worktrees{
		ReadingAffectedStacks: []worktrees.StackDiffChangedPair{readingAffected},
		WorktreePairs: map[string]*worktrees.WorktreePair{
			"pair": {
				FromWorktree: worktrees.Worktree{Ref: "HEAD~1", Path: from},
				ToWorktree:   worktrees.Worktree{Ref: "HEAD", Path: to},
				Diffs: &git.Diffs{
					Added:   []string{"added/terragrunt.stack.hcl", "added/terragrunt.hcl"},
					Removed: []string{"removed/terragrunt.stack.hcl", "removed/main.tf"},
					Changed: []string{"changed/terragrunt.stack.hcl", "changed/terragrunt.hcl"},
				},
			},
		},
	}

	stacks := w.Stacks()

	require.Len(t, stacks.Added, 1)
	assertStackAt(t, stacks.Added[0], to, "HEAD", "added")

	require.Len(t, stacks.Removed, 1)
	assertStackAt(t, stacks.Removed[0], from, "HEAD~1", "removed")

	require.Len(t, stacks.Changed, 1)
	assertStackAt(t, stacks.Changed[0].FromStack, from, "HEAD~1", "changed")
	assertStackAt(t, stacks.Changed[0].ToStack, to, "HEAD", "changed")

	assert.Equal(t, []worktrees.StackDiffChangedPair{readingAffected}, stacks.ReadingAffected)
}

// TestWorktreeCleanupFailures pins which failures to remove a worktree Cleanup forgives.
func TestWorktreeCleanupFailures(t *testing.T) {
	t.Parallel()

	root := venvtest.Root("/repo")
	path := filepath.Join(root, "worktree")

	testCases := []struct {
		wantErr error
		name    string
		stderr  string
		exit    int
		missing bool
	}{
		{
			name:    "already gone from disk",
			missing: true,
			exit:    1,
		},
		{
			name:   "git reports it gone",
			stderr: "fatal: '" + path + "' does not exist",
			exit:   128,
		},
		{
			name:    "any other failure",
			stderr:  "fatal: the worktree is locked",
			exit:    128,
			wantErr: git.ErrGitCommand,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := venvtest.New().WithHandler(func(_ context.Context, inv vexec.Invocation) vexec.Result {
				if slices.Contains(inv.Args, "worktree") {
					return vexec.Result{ExitCode: tc.exit, Stderr: []byte(tc.stderr)}
				}

				return vexec.Result{}
			})

			if !tc.missing {
				require.NoError(t, v.FS.MkdirAll(path, 0o755))
			}

			err := worktreesAt(root, path).Cleanup(t.Context(), logger.CreateLogger(), v)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)

				return
			}

			require.NoError(t, err)
		})
	}
}

// cancelOnCheckoutExec cancels a context when a `git checkout` is prepared,
// which is after a worktree is registered and before it is filled.
type cancelOnCheckoutExec struct {
	vexec.Exec
	cancel context.CancelFunc
}

// Command cancels the context before preparing a `git checkout`, and prepares
// every command through the wrapped Exec.
func (e *cancelOnCheckoutExec) Command(ctx context.Context, name string, args ...string) vexec.Cmd {
	if slices.Contains(args, "checkout") {
		e.cancel()
	}

	return e.Exec.Command(ctx, name, args...)
}

// commitUnits initializes a repository in dir and commits n units to it, one
// per commit.
func commitUnits(t *testing.T, dir string, n int) {
	t.Helper()

	runner := helpers.InitTestGitRunner(t, dir)

	for i := range n {
		name := fmt.Sprintf("unit-%d", i)

		require.NoError(t, os.MkdirAll(filepath.Join(dir, name), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, name, "terragrunt.hcl"),
			[]byte("inputs = {}\n"),
			0o600,
		))
		require.NoError(t, runner.Add(t.Context(), "."))
		require.NoError(t, runner.Commit(t.Context(), "Commit "+name))
	}
}

// registeredWorktrees returns the worktrees registered in the repository at
// repoDir besides its main worktree.
func registeredWorktrees(t *testing.T, repoDir string) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(repoDir, ".git", "worktrees"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	require.NoError(t, err)

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

// assertStackAt asserts that stack sits at dir under worktree and was discovered there at ref.
func assertStackAt(t *testing.T, stack *component.Stack, worktree, ref, dir string) {
	t.Helper()

	assert.Equal(t, filepath.Join(worktree, dir), stack.Path())
	assert.Equal(t, worktree, stack.DiscoveryContext().WorkingDir)
	assert.Equal(t, ref, stack.DiscoveryContext().Ref)
}

// gitExpressions returns the Git expressions of a filter comparing HEAD with its parent.
func gitExpressions(t *testing.T) filter.GitExpressions {
	t.Helper()

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	return filters.UniqueGitFilters()
}

// metricCounts reads back each counter the console metric exporter wrote into buf.
func metricCounts(t *testing.T, buf *bytes.Buffer) map[string]int64 {
	t.Helper()

	type exported struct {
		ScopeMetrics []struct {
			Metrics []struct {
				Name string `json:"Name"`
				Data struct {
					DataPoints []struct {
						Value int64 `json:"Value"`
					} `json:"DataPoints"`
				} `json:"Data"`
			} `json:"Metrics"`
		} `json:"ScopeMetrics"`
	}

	counts := map[string]int64{}

	dec := json.NewDecoder(buf)
	for dec.More() {
		var e exported

		require.NoError(t, dec.Decode(&e))

		for _, scope := range e.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, point := range metric.Data.DataPoints {
					counts[metric.Name] = point.Value
				}
			}
		}
	}

	return counts
}

// memRepoVenv returns a venv with an in-memory repository at root, answered by [memRepoHandler].
func memRepoVenv(t *testing.T, root, diff string, other vexec.Handler) *venv.Venv {
	t.Helper()

	fsys := venvtest.NewFS(t, root, map[string]string{".git": "gitdir: /elsewhere\n"})

	return venvtest.New().WithFS(fsys).WithHandler(memRepoHandler(diff, other))
}

// memRepoHandler answers git diff with diff, git ls-tree with an empty tree, and anything else with other.
func memRepoHandler(diff string, other vexec.Handler) vexec.Handler {
	return func(ctx context.Context, inv vexec.Invocation) vexec.Result {
		switch {
		case slices.Contains(inv.Args, "diff"):
			return vexec.Result{Stdout: []byte(diff)}
		case slices.Contains(inv.Args, "ls-tree"):
			return vexec.Result{}
		}

		return other(ctx, inv)
	}
}

// noopGit succeeds at every command with no output.
func noopGit(context.Context, vexec.Invocation) vexec.Result {
	return vexec.Result{}
}

// worktreesAt returns a Worktrees for repo with one materialized worktree at path.
func worktreesAt(repo, path string) *worktrees.Worktrees {
	return &worktrees.Worktrees{
		OriginalWorkingDir: repo,
		WorktreePairs: map[string]*worktrees.WorktreePair{
			"pair": {ToWorktree: worktrees.Worktree{Ref: "HEAD", Path: path}},
		},
	}
}

// treePaths returns the paths of a reference, as [git.GitRunner.LsTreeNames]
// reports them.
func treePaths(paths ...string) git.TreePaths {
	tree := make(git.TreePaths, len(paths))

	for _, path := range paths {
		tree[path] = struct{}{}
	}

	return tree
}
