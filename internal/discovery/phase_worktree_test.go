package discovery_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewWorktreePhase tests the WorktreePhase constructor.
func TestNewWorktreePhase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		numWorkers         int
		expectedNumWorkers int
	}{
		{
			name:               "positive workers",
			numWorkers:         4,
			expectedNumWorkers: 4,
		},
		{
			name:               "zero workers defaults to CPU count",
			numWorkers:         0,
			expectedNumWorkers: -1, // Will check > 0
		},
		{
			name:               "negative workers defaults to CPU count",
			numWorkers:         -1,
			expectedNumWorkers: -1, // Will check > 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			phase := discovery.NewWorktreePhase(nil, tt.numWorkers)

			assert.NotNil(t, phase)
			assert.Equal(t, "worktree", phase.Name())
			assert.Equal(t, discovery.PhaseWorktree, phase.Kind())

			if tt.expectedNumWorkers > 0 {
				assert.Equal(t, tt.expectedNumWorkers, phase.NumWorkers())
			} else {
				// When workers <= 0, it should default to runtime.NumCPU()
				assert.Positive(t, phase.NumWorkers())
			}
		})
	}
}

// TestGenerateDirSHA256 tests the SHA256 hash generation for directories.
func TestGenerateDirSHA256(t *testing.T) {
	t.Parallel()

	t.Run("empty_directory_produces_consistent_hash", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		hash1, err := discovery.GenerateDirSHA256(tmpDir)
		require.NoError(t, err)

		hash2, err := discovery.GenerateDirSHA256(tmpDir)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "Same empty directory should produce same hash")
	})

	t.Run("same_files_produce_same_hash", func(t *testing.T) {
		t.Parallel()

		tmpDir1 := t.TempDir()
		tmpDir2 := t.TempDir()

		content := []byte("test content")
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "file.txt"), content, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, "file.txt"), content, 0644))

		hash1, err := discovery.GenerateDirSHA256(tmpDir1)
		require.NoError(t, err)

		hash2, err := discovery.GenerateDirSHA256(tmpDir2)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "Directories with same files should produce same hash")
	})

	t.Run("modified_file_produces_different_hash", func(t *testing.T) {
		t.Parallel()

		tmpDir := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content1"), 0644))

		hash1, err := discovery.GenerateDirSHA256(tmpDir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("content2"), 0644))

		hash2, err := discovery.GenerateDirSHA256(tmpDir)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2, "Modified file should produce different hash")
	})

	t.Run("file_rename_produces_different_hash", func(t *testing.T) {
		t.Parallel()

		tmpDir1 := t.TempDir()
		tmpDir2 := t.TempDir()

		content := []byte("same content")
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "original.txt"), content, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, "renamed.txt"), content, 0644))

		hash1, err := discovery.GenerateDirSHA256(tmpDir1)
		require.NoError(t, err)

		hash2, err := discovery.GenerateDirSHA256(tmpDir2)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2, "File rename (different path) should produce different hash")
	})

	t.Run("file_move_to_subdirectory_produces_different_hash", func(t *testing.T) {
		t.Parallel()

		tmpDir1 := t.TempDir()
		tmpDir2 := t.TempDir()

		content := []byte("same content")
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "file.txt"), content, 0644))

		subDir := filepath.Join(tmpDir2, "subdir")
		require.NoError(t, os.MkdirAll(subDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(subDir, "file.txt"), content, 0644))

		hash1, err := discovery.GenerateDirSHA256(tmpDir1)
		require.NoError(t, err)

		hash2, err := discovery.GenerateDirSHA256(tmpDir2)
		require.NoError(t, err)

		assert.NotEqual(t, hash1, hash2, "File move to subdirectory should produce different hash")
	})

	t.Run("ignores_terragrunt_stack_manifest", func(t *testing.T) {
		t.Parallel()

		tmpDir1 := t.TempDir()
		tmpDir2 := t.TempDir()

		content := []byte("test content")
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "file.txt"), content, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, "file.txt"), content, 0644))

		// Add .terragrunt-stack-manifest only to tmpDir2
		manifestContent := []byte("/path/to/something\n/another/path")
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, ".terragrunt-stack-manifest"), manifestContent, 0644))

		hash1, err := discovery.GenerateDirSHA256(tmpDir1)
		require.NoError(t, err)

		hash2, err := discovery.GenerateDirSHA256(tmpDir2)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, ".terragrunt-stack-manifest should be ignored in hash calculation")
	})

	t.Run("multiple_files_order_independent", func(t *testing.T) {
		t.Parallel()

		tmpDir1 := t.TempDir()
		tmpDir2 := t.TempDir()

		// Create files in different order but same content
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "a.txt"), []byte("a"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir1, "b.txt"), []byte("b"), 0644))

		require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, "b.txt"), []byte("b"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir2, "a.txt"), []byte("a"), 0644))

		hash1, err := discovery.GenerateDirSHA256(tmpDir1)
		require.NoError(t, err)

		hash2, err := discovery.GenerateDirSHA256(tmpDir2)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "File creation order should not affect hash")
	})

	t.Run("nonexistent_directory_returns_error", func(t *testing.T) {
		t.Parallel()

		_, err := discovery.GenerateDirSHA256("/nonexistent/path/to/directory")
		require.Error(t, err)
	})

	t.Run("nested_directories_included", func(t *testing.T) {
		t.Parallel()

		tmpDir1 := t.TempDir()
		tmpDir2 := t.TempDir()

		// Create nested structure in both
		subDir1 := filepath.Join(tmpDir1, "sub", "nested")
		subDir2 := filepath.Join(tmpDir2, "sub", "nested")

		require.NoError(t, os.MkdirAll(subDir1, 0755))
		require.NoError(t, os.MkdirAll(subDir2, 0755))

		content := []byte("nested content")
		require.NoError(t, os.WriteFile(filepath.Join(subDir1, "file.txt"), content, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(subDir2, "file.txt"), content, 0644))

		hash1, err := discovery.GenerateDirSHA256(tmpDir1)
		require.NoError(t, err)

		hash2, err := discovery.GenerateDirSHA256(tmpDir2)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2, "Nested directories with same structure should produce same hash")
	})
}

// TestMatchComponentPairs tests the component pair matching logic.
func TestMatchComponentPairs(t *testing.T) {
	t.Parallel()

	t.Run("matches_by_relative_path", func(t *testing.T) {
		t.Parallel()

		fromComponents := component.Components{
			createTestComponent("/worktree-from/app", "/worktree-from"),
			createTestComponent("/worktree-from/db", "/worktree-from"),
		}

		toComponents := component.Components{
			createTestComponent("/worktree-to/app", "/worktree-to"),
			createTestComponent("/worktree-to/db", "/worktree-to"),
		}

		pairs := discovery.MatchComponentPairs(fromComponents, toComponents)

		assert.Len(t, pairs, 2, "Should match 2 component pairs")

		// Verify the pairs are correctly matched
		paths := make(map[string]bool)

		for _, p := range pairs {
			fromSuffix := getRelativePath(p.FromComponent)
			toSuffix := getRelativePath(p.ToComponent)
			assert.Equal(t, fromSuffix, toSuffix, "Matched components should have same relative paths")
			paths[fromSuffix] = true
		}

		assert.True(t, paths["/app"], "Should have matched app")
		assert.True(t, paths["/db"], "Should have matched db")
	})

	t.Run("handles_added_only_components", func(t *testing.T) {
		t.Parallel()

		fromComponents := component.Components{}

		toComponents := component.Components{
			createTestComponent("/worktree-to/new-unit", "/worktree-to"),
		}

		pairs := discovery.MatchComponentPairs(fromComponents, toComponents)

		assert.Empty(t, pairs, "Added-only components should not produce pairs")
	})

	t.Run("handles_removed_only_components", func(t *testing.T) {
		t.Parallel()

		fromComponents := component.Components{
			createTestComponent("/worktree-from/removed-unit", "/worktree-from"),
		}

		toComponents := component.Components{}

		pairs := discovery.MatchComponentPairs(fromComponents, toComponents)

		assert.Empty(t, pairs, "Removed-only components should not produce pairs")
	})

	t.Run("handles_renamed_components_no_match", func(t *testing.T) {
		t.Parallel()

		fromComponents := component.Components{
			createTestComponent("/worktree-from/old-name", "/worktree-from"),
		}

		toComponents := component.Components{
			createTestComponent("/worktree-to/new-name", "/worktree-to"),
		}

		pairs := discovery.MatchComponentPairs(fromComponents, toComponents)

		assert.Empty(t, pairs, "Renamed components (different paths) should not match")
	})

	t.Run("handles_mixed_scenario", func(t *testing.T) {
		t.Parallel()

		fromComponents := component.Components{
			createTestComponent("/worktree-from/shared", "/worktree-from"),
			createTestComponent("/worktree-from/removed", "/worktree-from"),
		}

		toComponents := component.Components{
			createTestComponent("/worktree-to/shared", "/worktree-to"),
			createTestComponent("/worktree-to/added", "/worktree-to"),
		}

		pairs := discovery.MatchComponentPairs(fromComponents, toComponents)

		assert.Len(t, pairs, 1, "Should only match the shared component")
		assert.Equal(t, "/shared", getRelativePath(pairs[0].FromComponent))
		assert.Equal(t, "/shared", getRelativePath(pairs[0].ToComponent))
	})

	t.Run("handles_empty_inputs", func(t *testing.T) {
		t.Parallel()

		pairs := discovery.MatchComponentPairs(component.Components{}, component.Components{})

		assert.Empty(t, pairs, "Empty inputs should produce empty pairs")
	})

	t.Run("handles_nested_paths", func(t *testing.T) {
		t.Parallel()

		fromComponents := component.Components{
			createTestComponent("/worktree-from/apps/frontend", "/worktree-from"),
			createTestComponent("/worktree-from/apps/backend", "/worktree-from"),
		}

		toComponents := component.Components{
			createTestComponent("/worktree-to/apps/frontend", "/worktree-to"),
			createTestComponent("/worktree-to/apps/backend", "/worktree-to"),
		}

		pairs := discovery.MatchComponentPairs(fromComponents, toComponents)

		assert.Len(t, pairs, 2, "Should match 2 nested component pairs")
	})
}

// TestTranslateDiscoveryContextArgsForWorktree tests the command argument translation for worktrees.
func TestTranslateDiscoveryContextArgsForWorktree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		cmd              string
		args             []string
		kind             discovery.WorktreeKind
		expectError      bool
		expectDestroyArg bool
	}{
		// fromWorktree cases - should add -destroy for plan/apply
		{
			name:             "from_worktree_plan_adds_destroy",
			cmd:              "plan",
			args:             []string{},
			kind:             discovery.FromWorktreeKind,
			expectError:      false,
			expectDestroyArg: true,
		},
		{
			name:             "from_worktree_apply_adds_destroy",
			cmd:              "apply",
			args:             []string{},
			kind:             discovery.FromWorktreeKind,
			expectError:      false,
			expectDestroyArg: true,
		},
		{
			name:             "from_worktree_plan_with_other_args_adds_destroy",
			cmd:              "plan",
			args:             []string{"-out", "plan.out"},
			kind:             discovery.FromWorktreeKind,
			expectError:      false,
			expectDestroyArg: true,
		},
		{
			name:             "from_worktree_plan_with_destroy_already_present_errors",
			cmd:              "plan",
			args:             []string{"-destroy"},
			kind:             discovery.FromWorktreeKind,
			expectError:      true,
			expectDestroyArg: false,
		},
		{
			name:             "from_worktree_empty_command_allowed",
			cmd:              "",
			args:             []string{},
			kind:             discovery.FromWorktreeKind,
			expectError:      false,
			expectDestroyArg: false,
		},
		{
			name:             "from_worktree_unsupported_command_errors",
			cmd:              "destroy",
			args:             []string{},
			kind:             discovery.FromWorktreeKind,
			expectError:      true,
			expectDestroyArg: false,
		},
		{
			name:             "from_worktree_output_command_errors",
			cmd:              "output",
			args:             []string{},
			kind:             discovery.FromWorktreeKind,
			expectError:      true,
			expectDestroyArg: false,
		},
		// toWorktree cases - should NOT add -destroy for plan/apply
		{
			name:             "to_worktree_plan_no_destroy",
			cmd:              "plan",
			args:             []string{},
			kind:             discovery.ToWorktreeKind,
			expectError:      false,
			expectDestroyArg: false,
		},
		{
			name:             "to_worktree_apply_no_destroy",
			cmd:              "apply",
			args:             []string{},
			kind:             discovery.ToWorktreeKind,
			expectError:      false,
			expectDestroyArg: false,
		},
		{
			name:             "to_worktree_plan_with_other_args",
			cmd:              "plan",
			args:             []string{"-out", "plan.out"},
			kind:             discovery.ToWorktreeKind,
			expectError:      false,
			expectDestroyArg: false,
		},
		{
			name:             "to_worktree_plan_with_destroy_already_present_errors",
			cmd:              "plan",
			args:             []string{"-destroy"},
			kind:             discovery.ToWorktreeKind,
			expectError:      true,
			expectDestroyArg: false,
		},
		{
			name:             "to_worktree_empty_command_allowed",
			cmd:              "",
			args:             []string{},
			kind:             discovery.ToWorktreeKind,
			expectError:      false,
			expectDestroyArg: false,
		},
		{
			name:             "to_worktree_unsupported_command_errors",
			cmd:              "destroy",
			args:             []string{},
			kind:             discovery.ToWorktreeKind,
			expectError:      true,
			expectDestroyArg: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dc := &component.DiscoveryContext{
				Cmd:  tt.cmd,
				Args: tt.args,
			}

			result, err := discovery.TranslateDiscoveryContextArgsForWorktree(dc, tt.kind)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "Git-based filtering is not supported")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.expectDestroyArg {
				assert.Contains(t, result.Args, "-destroy",
					"Expected -destroy flag for %s command in from worktree", tt.cmd)
			} else if tt.cmd == "plan" || tt.cmd == "apply" {
				// For to worktrees, verify -destroy is not added
				assert.False(t, slices.Contains(result.Args, "-destroy"),
					"Did not expect -destroy flag for %s command in to worktree", tt.cmd)
			}
		})
	}
}

// TestWorktreeKind tests the worktreeKind constants.
func TestWorktreeKind(t *testing.T) {
	t.Parallel()

	assert.Equal(t, discovery.FromWorktreeKind, discovery.WorktreeKind(0))
	assert.Equal(t, discovery.ToWorktreeKind, discovery.WorktreeKind(1))
	assert.NotEqual(t, discovery.FromWorktreeKind, discovery.ToWorktreeKind)
}

// Helper function to create a test component with discovery context.
func createTestComponent(path, workingDir string) component.Component {
	c := component.NewUnit(path)
	c.SetDiscoveryContext(&component.DiscoveryContext{
		WorkingDir: workingDir,
	})

	return c
}

// Helper function to get the relative path of a component.
func getRelativePath(c component.Component) string {
	dc := c.DiscoveryContext()
	if dc == nil {
		return c.Path()
	}

	rel := c.Path()[len(dc.WorkingDir):]
	if rel == "" {
		return "/"
	}

	return filepath.Clean(rel)
}
