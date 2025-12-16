package filter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTraceGitWorktreeCreate_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitWorktreeCreate(context.Background(), "main", "/tmp/worktree", "git@github.com:org/repo.git", "main", "abc123", func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreeRemove_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitWorktreeRemove(context.Background(), "main", "/tmp/worktree", func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreesCreate_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitWorktreesCreate(context.Background(), "/work", 2, "git@github.com:org/repo.git", "main", "abc123", func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreesCleanup_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitWorktreesCleanup(context.Background(), 2, "git@github.com:org/repo.git", func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitDiff_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitDiff(context.Background(), "main", "HEAD", "git@github.com:org/repo.git", func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreeDiscovery_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitWorktreeDiscovery(context.Background(), 3, func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitWorktreeStackWalk_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitWorktreeStackWalk(context.Background(), "main", "feature", func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceFilterEvaluate_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceFilterEvaluate(context.Background(), 5, 10, func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceFilterParse_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceFilterParse(context.Background(), "name=foo", func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitFilterExpand_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitFilterExpand(context.Background(), "main", "HEAD", 3, 1, 5, func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGitFilterEvaluate_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGitFilterEvaluate(context.Background(), "main", "HEAD", 10, func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTraceGraphFilterTraverse_NoTelemeter(t *testing.T) {
	t.Parallel()

	called := false
	err := TraceGraphFilterTraverse(context.Background(), "dependencies", 5, func(ctx context.Context) error {
		called = true
		return nil
	})

	assert.NoError(t, err)
	assert.True(t, called, "callback should be called even without telemeter")
}

func TestTelemetryConstants(t *testing.T) {
	t.Parallel()

	// Verify operation names are unique and well-formed
	opNames := []string{
		TelemetryOpGitWorktreeCreate,
		TelemetryOpGitWorktreeRemove,
		TelemetryOpGitWorktreesCreate,
		TelemetryOpGitWorktreesCleanup,
		TelemetryOpGitDiff,
		TelemetryOpGitWorktreeDiscovery,
		TelemetryOpGitWorktreeStackWalk,
		TelemetryOpGitWorktreeFilterApply,
		TelemetryOpFilterEvaluate,
		TelemetryOpFilterParse,
		TelemetryOpGitFilterExpand,
		TelemetryOpGitFilterEvaluate,
		TelemetryOpGraphFilterTraverse,
	}

	seen := make(map[string]bool)
	for _, name := range opNames {
		assert.NotEmpty(t, name, "operation name should not be empty")
		assert.False(t, seen[name], "operation name should be unique: %s", name)
		seen[name] = true
	}

	// Verify attribute keys are well-formed
	attrKeys := []string{
		AttrGitRef,
		AttrGitFromRef,
		AttrGitToRef,
		AttrGitWorktreeDir,
		AttrGitWorkingDir,
		AttrGitRefCount,
		AttrGitDiffAdded,
		AttrGitDiffRemoved,
		AttrGitDiffChanged,
		AttrFilterQuery,
		AttrFilterType,
		AttrFilterCount,
		AttrComponentCount,
		AttrResultCount,
		AttrWorktreePairCount,
	}

	seenAttrs := make(map[string]bool)
	for _, key := range attrKeys {
		assert.NotEmpty(t, key, "attribute key should not be empty")
		assert.False(t, seenAttrs[key], "attribute key should be unique: %s", key)
		seenAttrs[key] = true
	}
}
