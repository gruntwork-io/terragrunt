package worktrees_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gogit "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func TestNewWorktrees(t *testing.T) {
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

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	err = runner.GoCommit("Second commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	filters, err := filter.ParseFilterQueries([]string{"[HEAD~1...HEAD]"})
	require.NoError(t, err)

	w, err := worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		tmpDir,
		filters.UniqueGitFilters(),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupErr := w.Cleanup(context.Background(), logger.CreateLogger())
		require.NoError(t, cleanupErr)
	})

	require.NotEmpty(t, w.WorktreePairs)
}

func TestNewWorktreesWithInvalidReference(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	tmpDir, err := filepath.EvalSymlinks(tmpDir)
	require.NoError(t, err)

	// Initialize Git repository
	runner, err := git.NewGitRunner()
	require.NoError(t, err)

	runner = runner.WithWorkDir(tmpDir)

	err = runner.Init(t.Context())
	require.NoError(t, err)

	err = runner.GoOpenRepo()
	require.NoError(t, err)

	defer runner.GoCloseStorage()

	err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
		AllowEmptyCommits: true,
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	require.NoError(t, err)

	opts := options.NewTerragruntOptions()
	opts.WorkingDir = tmpDir
	opts.RootWorkingDir = tmpDir

	// Parse filter with invalid Git reference
	filters, err := filter.ParseFilterQueries([]string{"[nonexistent-branch]"})
	require.NoError(t, err) // Parsing should succeed

	_, err = worktrees.NewWorktrees(
		t.Context(),
		logger.CreateLogger(),
		tmpDir,
		filters.UniqueGitFilters(),
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
			name: "changed stack files are skipped",
			diffs: &git.Diffs{
				Changed: []string{
					"stack/terragrunt.stack.hcl",
				},
			},
			expectedFrom:       0,
			expectedTo:         0,
			expectedToPaths:    []string{},
			expectedToReadings: []string{},
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
			expectedFrom:       1,
			expectedTo:         4,
			expectedToPaths:    []string{"app-added", "app-modified"},
			expectedToReadings: []string{"app-modified/main.tf", "other/file.hcl"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
				runner.GoCloseStorage()
			})

			err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
				AllowEmptyCommits: true,
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			wp := &worktrees.WorktreePair{
				Diffs: tt.diffs,
			}

			fromFilters, toFilters := wp.Expand()

			// Verify from filters count
			assert.Len(t, fromFilters, tt.expectedFrom, "From filters count should match")

			// Verify to filters count
			assert.Len(t, toFilters, tt.expectedTo, "To filters count should match")

			// Verify from filters are path filters with correct paths
			for i, f := range fromFilters {
				pathExpr, ok := f.Expression().(*filter.PathExpression)
				require.True(t, ok, "From filter %d should be a PathExpression", i)
				expectedPath := filepath.Dir(tt.diffs.Removed[i])
				assert.Equal(t, expectedPath, pathExpr.Value, "From filter %d should have correct path", i)
			}

			// Verify to filters
			toPaths := []string{}
			toReadings := []string{}

			for _, f := range toFilters {
				switch expr := f.Expression().(type) {
				case *filter.PathExpression:
					toPaths = append(toPaths, expr.Value)
				case *filter.AttributeExpression:
					if expr.Key == "reading" {
						toReadings = append(toReadings, expr.Value)
					}
				}
			}

			// Verify path filters
			assert.ElementsMatch(t, tt.expectedToPaths, toPaths, "To path filters should match")

			// Verify reading filters
			assert.ElementsMatch(t, tt.expectedToReadings, toReadings, "To reading filters should match")
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
				runner.GoCloseStorage()
			})

			err = runner.GoCommit("Initial commit", &gogit.CommitOptions{
				AllowEmptyCommits: true,
				Author: &object.Signature{
					Name:  "Test User",
					Email: "test@example.com",
					When:  time.Now(),
				},
			})
			require.NoError(t, err)

			wp := &worktrees.WorktreePair{
				Diffs: tt.diffs,
			}

			_, toFilters := wp.Expand()

			// Extract reading filters
			readings := []string{}

			for _, f := range toFilters {
				if attrExpr, ok := f.Expression().(*filter.AttributeExpression); ok {
					if attrExpr.Key == "reading" {
						readings = append(readings, attrExpr.Value)
					}
				}
			}

			// Verify reading filters match expected
			assert.ElementsMatch(t, tt.expectedReadings, readings, "Reading filters should match expected paths")

			// Verify each reading filter is properly constructed
			for _, expectedReading := range tt.expectedReadings {
				found := false

				for _, f := range toFilters {
					if attrExpr, ok := f.Expression().(*filter.AttributeExpression); ok {
						if attrExpr.Key == "reading" && attrExpr.Value == expectedReading {
							found = true

							assert.Equal(t, "reading", attrExpr.Key, "Filter should have reading key")
							assert.Equal(t, expectedReading, attrExpr.Value, "Filter should have correct file path")

							break
						}
					}
				}

				assert.True(t, found, "Expected reading filter for %s should be present", expectedReading)
			}
		})
	}
}

func TestExpandWithUnitDirectoryDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		setupFilesystem    func(tmpDir string) error
		diffs              *git.Diffs
		expectedToPaths    []string
		expectedToReadings []string
		expectedFrom       int
	}{
		{
			name: "removed file in unit directory creates path filter",
			setupFilesystem: func(tmpDir string) error {
				// Create unit directory with terragrunt.hcl
				unitDir := filepath.Join(tmpDir, "unit1")
				if err := os.MkdirAll(unitDir, 0755); err != nil {
					return err
				}
				terragruntFile := filepath.Join(unitDir, "terragrunt.hcl")
				return os.WriteFile(terragruntFile, []byte("# terragrunt config"), 0644)
			},
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
			name: "removed file in non-unit directory creates no filter",
			setupFilesystem: func(tmpDir string) error {
				// Create non-unit directory (no terragrunt.hcl)
				nonUnitDir := filepath.Join(tmpDir, "non-unit")
				return os.MkdirAll(nonUnitDir, 0755)
			},
			diffs: &git.Diffs{
				Removed: []string{
					"non-unit/some-file.tf",
				},
			},
			expectedToPaths:    []string{},
			expectedToReadings: []string{},
			expectedFrom:       0,
		},
		{
			name: "added file in unit directory creates path filter",
			setupFilesystem: func(tmpDir string) error {
				// Create unit directory with terragrunt.hcl
				unitDir := filepath.Join(tmpDir, "unit1")
				if err := os.MkdirAll(unitDir, 0755); err != nil {
					return err
				}
				terragruntFile := filepath.Join(unitDir, "terragrunt.hcl")
				return os.WriteFile(terragruntFile, []byte("# terragrunt config"), 0644)
			},
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
			name: "added file in non-unit directory creates no filter",
			setupFilesystem: func(tmpDir string) error {
				// Create non-unit directory (no terragrunt.hcl)
				nonUnitDir := filepath.Join(tmpDir, "non-unit")
				return os.MkdirAll(nonUnitDir, 0755)
			},
			diffs: &git.Diffs{
				Added: []string{
					"non-unit/new-file.tf",
				},
			},
			expectedToPaths:    []string{},
			expectedToReadings: []string{},
			expectedFrom:       0,
		},
		{
			name: "changed file in unit directory creates path filter",
			setupFilesystem: func(tmpDir string) error {
				// Create unit directory with terragrunt.hcl
				unitDir := filepath.Join(tmpDir, "unit1")
				if err := os.MkdirAll(unitDir, 0755); err != nil {
					return err
				}
				terragruntFile := filepath.Join(unitDir, "terragrunt.hcl")
				return os.WriteFile(terragruntFile, []byte("# terragrunt config"), 0644)
			},
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
			name: "changed file in non-unit directory creates reading filter",
			setupFilesystem: func(tmpDir string) error {
				// Create non-unit directory (no terragrunt.hcl)
				nonUnitDir := filepath.Join(tmpDir, "non-unit")
				return os.MkdirAll(nonUnitDir, 0755)
			},
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
			setupFilesystem: func(tmpDir string) error {
				// Create unit1 directory
				unit1Dir := filepath.Join(tmpDir, "unit1")
				if err := os.MkdirAll(unit1Dir, 0755); err != nil {
					return err
				}
				terragruntFile1 := filepath.Join(unit1Dir, "terragrunt.hcl")
				if err := os.WriteFile(terragruntFile1, []byte("# terragrunt config"), 0644); err != nil {
					return err
				}

				// Create unit2 directory
				unit2Dir := filepath.Join(tmpDir, "unit2")
				if err := os.MkdirAll(unit2Dir, 0755); err != nil {
					return err
				}
				terragruntFile2 := filepath.Join(unit2Dir, "terragrunt.hcl")
				if err := os.WriteFile(terragruntFile2, []byte("# terragrunt config"), 0644); err != nil {
					return err
				}

				// Create non-unit directory
				nonUnitDir := filepath.Join(tmpDir, "non-unit")
				return os.MkdirAll(nonUnitDir, 0755)
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

			tmpDir := t.TempDir()
			tmpDir, err := filepath.EvalSymlinks(tmpDir)
			require.NoError(t, err)

			// Setup filesystem structure
			err = tt.setupFilesystem(tmpDir)
			require.NoError(t, err)

			wp := &worktrees.WorktreePair{
				Diffs: tt.diffs,
				ToWorktree: worktrees.Worktree{
					Path: tmpDir,
				},
			}

			fromFilters, toFilters := wp.Expand()

			// Verify from filters count
			assert.Len(t, fromFilters, tt.expectedFrom, "From filters count should match")

			// Extract path and reading filters from toFilters
			toPathsMap := make(map[string]bool)
			toReadings := []string{}

			for _, f := range toFilters {
				switch expr := f.Expression().(type) {
				case *filter.PathExpression:
					toPathsMap[expr.Value] = true
				case *filter.AttributeExpression:
					if expr.Key == "reading" {
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
			assert.ElementsMatch(t, tt.expectedToReadings, toReadings, "To reading filters should match")
		})
	}
}
