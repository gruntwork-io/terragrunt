package engine

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBuildFailed = errors.New("build failed")

// TestEngineClients_LoadOrCreateIsSingleFlightWithRacing verifies that racing callers on one
// key build exactly one engine and reuse it (the destroy-diamond read at its core).
func TestEngineClients_LoadOrCreateIsSingleFlightWithRacing(t *testing.T) {
	t.Parallel()

	c := newEngineClients()

	const key = "/cache/single-flight"

	execOpts := &ExecutionOptions{UnitDir: "/unit/sf", CacheDir: key}
	inst := &engineInstance{execOptions: execOpts}

	var builds atomic.Int64

	create := func() (*engineInstance, error) {
		builds.Add(1)

		return inst, nil
	}

	const n = 64

	var (
		wg       sync.WaitGroup
		start    = make(chan struct{})
		gotInst  = make([]*engineInstance, n)
		gotReuse = make([]bool, n)
		gotErr   = make([]error, n)
	)

	for i := range n {
		wg.Go(func() {
			<-start

			got, reused, err := c.loadOrCreate(key, execOpts, create)
			gotInst[i], gotReuse[i], gotErr[i] = got, reused, err
		})
	}

	close(start)
	wg.Wait()

	assert.Equal(t, int64(1), builds.Load(), "create must run exactly once across racing callers")

	fresh := 0

	for i := range n {
		require.NoError(t, gotErr[i])
		assert.Same(t, inst, gotInst[i], "every caller gets the single built instance")

		if !gotReuse[i] {
			fresh++
		}
	}

	assert.Equal(t, 1, fresh, "exactly one caller built the engine; the rest reused it")
}

// TestEngineClients_LoadOrCreateRetriesAfterFailedBuild verifies a failed build is dropped, so
// the next call builds fresh instead of being served the stale failure.
func TestEngineClients_LoadOrCreateRetriesAfterFailedBuild(t *testing.T) {
	t.Parallel()

	c := newEngineClients()

	const key = "/cache/retry"

	execOpts := &ExecutionOptions{UnitDir: "/unit/retry", CacheDir: key}

	_, _, err := c.loadOrCreate(key, execOpts, func() (*engineInstance, error) {
		return nil, errBuildFailed
	})
	require.ErrorIs(t, err, errBuildFailed)

	inst := &engineInstance{execOptions: execOpts}

	got, reused, err := c.loadOrCreate(key, execOpts, func() (*engineInstance, error) {
		return inst, nil
	})
	require.NoError(t, err)
	assert.False(t, reused, "a failed build is dropped, so the next call builds fresh")
	assert.Same(t, inst, got)
}

// TestEngineClients_LoadOrCreateGuardsReplacementOnFailedBuildWithRacing checks the ABA guard:
// after a drain removes a still-building entry and a replacement takes its key, the original
// build's failure must not delete the replacement.
func TestEngineClients_LoadOrCreateGuardsReplacementOnFailedBuildWithRacing(t *testing.T) {
	t.Parallel()

	c := newEngineClients()

	const key = "/cache/aba"

	execOpts := &ExecutionOptions{UnitDir: "/unit/aba", CacheDir: key}

	reserved := make(chan struct{})
	release := make(chan struct{})

	var (
		first    sync.WaitGroup
		firstErr error
	)

	first.Go(func() {
		_, _, firstErr = c.loadOrCreate(key, execOpts, func() (*engineInstance, error) {
			close(reserved)
			<-release

			return nil, errBuildFailed
		})
	})

	<-reserved // first build is parked in create() with its entry reserved

	require.NotNil(t, c.takeUnit("/unit/aba"))

	replacement := &engineInstance{execOptions: execOpts}

	got, reused, err := c.loadOrCreate(key, execOpts, func() (*engineInstance, error) {
		return replacement, nil
	})
	require.NoError(t, err)
	require.False(t, reused)
	require.Same(t, replacement, got)

	// Now let the first build fail; its cleanup must not delete the replacement.
	close(release)
	first.Wait()
	require.ErrorIs(t, firstErr, errBuildFailed)

	survivor := c.takeUnit("/unit/aba")
	require.NotNil(t, survivor, "the replacement must survive the failed build's cleanup")
	assert.Same(t, replacement, survivor.instance)
}
