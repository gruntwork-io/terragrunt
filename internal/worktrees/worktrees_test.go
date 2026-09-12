package worktrees_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
// whose target is absolute and outside the repository keeps a reference from
// being materialized, like the relative target
// TestNewWorktreesPartialFailureCleanup pins. Extraction refuses such a link,
// and the checkout it could fall back to would write the link as git would,
// so the refusal is never routed around the fallback.
func TestNewWorktreesWithSymlinkOutsideRepository(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("creating a symlink on Windows takes a privilege the runner may not have")
	}

	repoDir := helpers.TmpDirWOSymlinks(t)
	tempDir := helpers.TmpDirWOSymlinks(t)
	outside := filepath.Join(helpers.TmpDirWOSymlinks(t), "outside.txt")
	require.NoError(t, os.WriteFile(outside, []byte("outside\n"), 0o600))

	runner := helpers.InitTestGitRunner(t, repoDir)

	unitDir := filepath.Join(repoDir, "unit")
	require.NoError(t, os.MkdirAll(unitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte("inputs = {}\n"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(repoDir, "link")))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	require.NoError(t, os.WriteFile(filepath.Join(unitDir, "terragrunt.hcl"), []byte("# changed\n"), 0o600))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Second commit"))

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir })

	_, err = worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		v,
		worktrees.WorktreeOpts{WorkingDir: repoDir, GitExpressions: filters.UniqueGitFilters()},
	)
	require.ErrorIs(t, err, vfs.ErrSymlinkEscapes)

	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries)

	// Git deletes its worktrees directory along with the last registration.
	assert.NoDirExists(t, filepath.Join(repoDir, ".git", "worktrees"))
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

// TestNewWorktreesPartialFailureCleanup pins that a reference failing to
// materialize leaves neither its own worktree nor the ones created beside it
// in the temporary directory or in the repository's worktree list.
func TestNewWorktreesPartialFailureCleanup(t *testing.T) {
	t.Parallel()

	if helpers.IsWindows() {
		t.Skip("os.Symlink on Windows requires special permissions; covered by Unix CI")
	}

	repoDir := helpers.TmpDirWOSymlinks(t)
	tempDir := helpers.TmpDirWOSymlinks(t)

	runner := helpers.InitTestGitRunner(t, repoDir)

	require.NoError(t, os.MkdirAll(filepath.Join(repoDir, "unit"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(repoDir, "unit", "terragrunt.hcl"),
		[]byte("inputs = {}\n"),
		0o600,
	))

	// Extraction refuses a link pointing out of the worktree, so HEAD~1 fails
	// to materialize after it is registered, while HEAD without the link
	// succeeds.
	link := filepath.Join(repoDir, "escape")
	require.NoError(t, os.Symlink("../outside", link))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Initial commit"))

	require.NoError(t, os.Remove(link))
	require.NoError(t, runner.Add(t.Context(), "."))
	require.NoError(t, runner.Commit(t.Context(), "Remove link"))

	filters, err := filter.ParseFilterQueries(logger.CreateLogger(), []string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	v := venvtest.NewOSWithEmptyEnv().WithTempDir(func() string { return tempDir })

	_, err = worktrees.NewWorktrees(t.Context(), logger.CreateLogger(), v, worktrees.WorktreeOpts{
		WorkingDir:     repoDir,
		GitExpressions: filters.UniqueGitFilters(),
	})
	require.ErrorIs(t, err, vfs.ErrSymlinkEscapes)

	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	assert.Empty(t, entries)

	// Git deletes its worktrees directory along with the last registration.
	assert.NoDirExists(t, filepath.Join(repoDir, ".git", "worktrees"))
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

// treePaths returns the paths of a reference, as [git.GitRunner.LsTreeNames]
// reports them.
func treePaths(paths ...string) git.TreePaths {
	tree := make(git.TreePaths, len(paths))

	for _, path := range paths {
		tree[path] = struct{}{}
	}

	return tree
}
