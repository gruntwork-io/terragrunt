package discovery_test

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

		pairs, err := discovery.MatchComponentPairs(fromComponents, toComponents)
		require.NoError(t, err)

		assert.Len(t, pairs, 2, "Should match 2 component pairs")

		// Verify the pairs are correctly matched
		paths := make(map[string]bool)

		for _, p := range pairs {
			fromSuffix := getRelativePath(p.FromComponent)
			toSuffix := getRelativePath(p.ToComponent)
			assert.Equal(
				t,
				fromSuffix,
				toSuffix,
				"Matched components should have same relative paths",
			)
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

		pairs, err := discovery.MatchComponentPairs(fromComponents, toComponents)
		require.NoError(t, err)

		assert.Empty(t, pairs, "Added-only components should not produce pairs")
	})

	t.Run("handles_removed_only_components", func(t *testing.T) {
		t.Parallel()

		fromComponents := component.Components{
			createTestComponent("/worktree-from/removed-unit", "/worktree-from"),
		}

		toComponents := component.Components{}

		pairs, err := discovery.MatchComponentPairs(fromComponents, toComponents)
		require.NoError(t, err)

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

		pairs, err := discovery.MatchComponentPairs(fromComponents, toComponents)
		require.NoError(t, err)

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

		pairs, err := discovery.MatchComponentPairs(fromComponents, toComponents)
		require.NoError(t, err)

		assert.Len(t, pairs, 1, "Should only match the shared component")
		assert.Equal(t, "/shared", getRelativePath(pairs[0].FromComponent))
		assert.Equal(t, "/shared", getRelativePath(pairs[0].ToComponent))
	})

	t.Run("handles_empty_inputs", func(t *testing.T) {
		t.Parallel()

		pairs, err := discovery.MatchComponentPairs(component.Components{}, component.Components{})
		require.NoError(t, err)

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

		pairs, err := discovery.MatchComponentPairs(fromComponents, toComponents)
		require.NoError(t, err)

		assert.Len(t, pairs, 2, "Should match 2 nested component pairs")
	})

	t.Run("returns_error_for_nil_discovery_context", func(t *testing.T) {
		t.Parallel()

		nilCtxComponent := component.NewUnit("/some/path")
		nilCtxComponent.SetDiscoveryContext(nil)

		fromComponents := component.Components{nilCtxComponent}
		toComponents := component.Components{}

		_, err := discovery.MatchComponentPairs(fromComponents, toComponents)
		require.Error(t, err)

		var missingCtxErr discovery.MissingDiscoveryContextError
		require.ErrorAs(t, err, &missingCtxErr)
		assert.Equal(t, "/some/path", missingCtxErr.ComponentPath)
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
			name:             "from_worktree_plan_with_destroy_true_errors",
			cmd:              "plan",
			args:             []string{"-destroy=true"},
			kind:             discovery.FromWorktreeKind,
			expectError:      true,
			expectDestroyArg: false,
		},
		{
			name:             "from_worktree_plan_with_destroy_false_adds_destroy",
			cmd:              "plan",
			args:             []string{"-destroy=false"},
			kind:             discovery.FromWorktreeKind,
			expectError:      false,
			expectDestroyArg: true,
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
			name:             "to_worktree_apply_with_destroy_true_errors",
			cmd:              "apply",
			args:             []string{"-destroy=true"},
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

	return filepath.ToSlash(filepath.Clean(rel))
}
