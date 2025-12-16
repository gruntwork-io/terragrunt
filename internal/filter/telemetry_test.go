package filter_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceGitWorktreeCreate_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitWorktreeCreate(context.Background(), "main", "/tmp/worktree", "git@github.com:org/repo.git", "main", "abc123", func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreeRemove_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitWorktreeRemove(context.Background(), "main", "/tmp/worktree", func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreesCreate_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitWorktreesCreate(context.Background(), "/work", 2, "git@github.com:org/repo.git", "main", "abc123", func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreesCleanup_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitWorktreesCleanup(context.Background(), 2, "git@github.com:org/repo.git", func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitDiff_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitDiff(context.Background(), "main", "HEAD", "git@github.com:org/repo.git", func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreeDiscovery_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitWorktreeDiscovery(context.Background(), 3, func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreeStackWalk_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitWorktreeStackWalk(context.Background(), "main", "feature", func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceFilterEvaluate_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceFilterEvaluate(context.Background(), 5, 10, func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceFilterParse_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceFilterParse(context.Background(), "name=foo", func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitFilterExpand_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitFilterExpand(context.Background(), "main", "HEAD", 3, 1, 5, func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitFilterEvaluate_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGitFilterEvaluate(context.Background(), "main", "HEAD", 10, func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGraphFilterTraverse_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := filter.TraceGraphFilterTraverse(context.Background(), "dependencies", 5, func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTelemetryConstants(t *testing.T) {
	t.Parallel()

	// Verify operation names are unique and well-formed
	opNames := []string{
		filter.TelemetryOpGitWorktreeCreate,
		filter.TelemetryOpGitWorktreeRemove,
		filter.TelemetryOpGitWorktreesCreate,
		filter.TelemetryOpGitWorktreesCleanup,
		filter.TelemetryOpGitDiff,
		filter.TelemetryOpGitWorktreeDiscovery,
		filter.TelemetryOpGitWorktreeStackWalk,
		filter.TelemetryOpGitWorktreeFilterApply,
		filter.TelemetryOpFilterEvaluate,
		filter.TelemetryOpFilterParse,
		filter.TelemetryOpGitFilterExpand,
		filter.TelemetryOpGitFilterEvaluate,
		filter.TelemetryOpGraphFilterTraverse,
	}

	seen := make(map[string]bool)

	for _, name := range opNames {
		assert.NotEmpty(t, name, "operation name should not be empty")
		assert.False(t, seen[name], "operation name should be unique: %s", name)
		seen[name] = true
	}

	// Verify attribute keys are well-formed
	attrKeys := []string{
		filter.AttrGitRef,
		filter.AttrGitFromRef,
		filter.AttrGitToRef,
		filter.AttrGitWorktreeDir,
		filter.AttrGitWorkingDir,
		filter.AttrGitRefCount,
		filter.AttrGitDiffAdded,
		filter.AttrGitDiffRemoved,
		filter.AttrGitDiffChanged,
		filter.AttrFilterQuery,
		filter.AttrFilterType,
		filter.AttrFilterCount,
		filter.AttrComponentCount,
		filter.AttrResultCount,
		filter.AttrWorktreePairCount,
	}

	seenAttrs := make(map[string]bool)

	for _, key := range attrKeys {
		assert.NotEmpty(t, key, "attribute key should not be empty")
		assert.False(t, seenAttrs[key], "attribute key should be unique: %s", key)
		seenAttrs[key] = true
	}
}
