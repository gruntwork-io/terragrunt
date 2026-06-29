package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/options"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapFromWorktreeReadingAffected_UsesComponentConfigFile(t *testing.T) {
	t.Parallel()

	fromDir := t.TempDir()
	toDir := t.TempDir()

	fromUnitDir := filepath.Join(fromDir, "app")
	toUnitDir := filepath.Join(toDir, "app")

	require.NoError(t, os.MkdirAll(fromUnitDir, 0o755))
	require.NoError(t, os.MkdirAll(toUnitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fromUnitDir, "custom.hcl"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(toUnitDir, "custom.hcl"), []byte(""), 0o644))

	fromComponent := component.NewUnit(fromUnitDir)
	fromComponent.SetConfigFile("custom.hcl")

	phase := NewWorktreePhase(nil, 1)
	l := logger.CreateLogger()
	input := &PhaseInput{
		Opts: &options.TerragruntOptions{WorkingDir: fromDir},
		Discovery: NewDiscovery(fromDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fromDir}).
			WithConfigFilenames([]string{"custom.hcl"}),
	}

	result, err := phase.mapFromWorktreeReadingAffected(
		context.Background(),
		l,
		input,
		worktrees.WorktreePair{
			FromWorktree: worktrees.Worktree{Path: fromDir, Ref: "HEAD~1"},
			ToWorktree:   worktrees.Worktree{Path: toDir, Ref: "HEAD"},
		},
		component.Components{fromComponent},
	)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, toUnitDir, result[0].Path())
	assert.Equal(t, "custom.hcl", result[0].ConfigFile())
}

func TestMapFromWorktreeReadingAffected_PropagatesRediscoveryErrors(t *testing.T) {
	t.Parallel()

	fromDir := t.TempDir()
	toDir := t.TempDir()

	fromUnitDir := filepath.Join(fromDir, "app")
	toUnitDir := filepath.Join(toDir, "app")

	require.NoError(t, os.MkdirAll(fromUnitDir, 0o755))
	require.NoError(t, os.MkdirAll(toUnitDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fromUnitDir, "terragrunt.hcl"), []byte(""), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(toUnitDir, "terragrunt.hcl"), []byte(""), 0o644))

	fromComponent := component.NewUnit(fromUnitDir)

	phase := NewWorktreePhase(nil, 1)
	l := logger.CreateLogger()
	input := &PhaseInput{
		Opts: &options.TerragruntOptions{WorkingDir: fromDir},
		Discovery: NewDiscovery(fromDir).
			WithDiscoveryContext(&component.DiscoveryContext{WorkingDir: fromDir, Cmd: "destroy"}),
	}

	result, err := phase.mapFromWorktreeReadingAffected(
		context.Background(),
		l,
		input,
		worktrees.WorktreePair{
			FromWorktree: worktrees.Worktree{Path: fromDir, Ref: "HEAD~1"},
			ToWorktree:   worktrees.Worktree{Path: toDir, Ref: "HEAD"},
		},
		component.Components{fromComponent},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Git-based filtering is not supported")
	assert.Nil(t, result)
}
