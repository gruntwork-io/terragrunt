package hclparse_test

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

var errBody = errors.New("body failed")

// outcome is what one caller got back from Resolve.
type outcome struct {
	err error
	res hclparse.Resolved[string]
}

func TestStoreResolveRunsBodyOnce(t *testing.T) {
	t.Parallel()

	s := &hclparse.Store{}
	k := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Context: "ctx", Stage: hclparse.StageQueue}
	runs := 0

	body := func(context.Context) (string, error) {
		runs++

		return "value", nil
	}

	first, err := s.Resolve(t.Context(), k, body)
	require.NoError(t, err)
	assert.Equal(t, "value", first.Value)
	assert.False(t, first.Hit)

	second, err := s.Resolve(t.Context(), k, body)
	require.NoError(t, err)
	assert.Equal(t, "value", second.Value)
	assert.True(t, second.Hit)

	assert.Equal(t, 1, runs)
	assert.Equal(t, hclparse.Stats{Hits: 1, Misses: 1}, s.Stats())
}

func TestStoreResolveKeysOnEveryField(t *testing.T) {
	t.Parallel()

	s := &hclparse.Store{}
	base := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Context: "ctx", Stage: hclparse.StageQueue}
	keys := []hclparse.Key{
		base,
		{Path: "/repo/other/terragrunt.hcl", Context: "ctx", Stage: hclparse.StageQueue},
		{Path: base.Path, Context: "other", Stage: hclparse.StageQueue},
		{Path: base.Path, Context: "ctx", Stage: hclparse.StageAST},
	}

	for i, k := range keys {
		res, err := s.Resolve(t.Context(), k, func(context.Context) (int, error) { return i, nil })
		require.NoError(t, err)
		assert.False(t, res.Hit)
		assert.Equal(t, i, res.Value)
	}
}

func TestStoreResolveDoesNotRetainErrors(t *testing.T) {
	t.Parallel()

	s := &hclparse.Store{}
	k := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Stage: hclparse.StageQueue}
	runs := 0

	failing := func(context.Context) (string, error) {
		runs++

		return "partial", errBody
	}

	res, err := s.Resolve(t.Context(), k, failing)
	require.ErrorIs(t, err, errBody)
	assert.Equal(t, "partial", res.Value, "the value travels with the error")

	res, err = s.Resolve(t.Context(), k, failing)
	require.ErrorIs(t, err, errBody)
	assert.False(t, res.Hit)
	assert.Equal(t, 2, runs)

	res, err = s.Resolve(t.Context(), k, func(context.Context) (string, error) { return "ok", nil })
	require.NoError(t, err)
	assert.False(t, res.Hit)
	assert.Equal(t, "ok", res.Value)
}

func TestStoreResolveSharesOneRunWithRacing(t *testing.T) {
	t.Parallel()

	const callers = 16

	s := &hclparse.Store{}
	k := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Stage: hclparse.StageQueue}

	var (
		runs     atomic.Int32
		release  = make(chan struct{})
		wg       sync.WaitGroup
		outcomes = make(chan outcome, callers)
	)

	body := func(context.Context) (string, error) {
		runs.Add(1)
		<-release

		return "value", nil
	}

	for range callers {
		wg.Go(func() {
			res, err := s.Resolve(t.Context(), k, body)

			outcomes <- outcome{res: res, err: err}
		})
	}

	require.Eventually(t, func() bool { return runs.Load() == 1 }, time.Second, time.Millisecond)
	close(release)
	wg.Wait()
	close(outcomes)

	hits := 0

	for o := range outcomes {
		require.NoError(t, o.err)
		assert.Equal(t, "value", o.res.Value)

		if o.res.Hit {
			hits++
		}
	}

	assert.Equal(t, int32(1), runs.Load())
	assert.Equal(t, callers-1, hits)
}

func TestStoreResolveWaiterRunsAgainAfterLeaderCancelsWithRacing(t *testing.T) {
	t.Parallel()

	s := &hclparse.Store{}
	k := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Stage: hclparse.StageQueue}

	leaderCtx, cancelLeader := context.WithCancel(t.Context())
	started := make(chan struct{})

	var runs atomic.Int32

	body := func(ctx context.Context) (string, error) {
		if runs.Add(1) == 1 {
			close(started)
			<-ctx.Done()

			return "", ctx.Err()
		}

		return "value", nil
	}

	leaderDone := make(chan error, 1)

	go func() {
		_, err := s.Resolve(leaderCtx, k, body)
		leaderDone <- err
	}()

	<-started

	waiterDone := make(chan outcome, 1)

	go func() {
		res, err := s.Resolve(t.Context(), k, body)

		waiterDone <- outcome{res: res, err: err}
	}()

	require.Eventually(t, func() bool { return runs.Load() == 1 }, time.Second, time.Millisecond)
	cancelLeader()

	require.ErrorIs(t, <-leaderDone, context.Canceled)

	waiter := <-waiterDone
	require.NoError(t, waiter.err)
	assert.Equal(t, "value", waiter.res.Value)
	assert.False(t, waiter.res.Hit, "the waiter ran the body itself")
	assert.Equal(t, int32(2), runs.Load())
}

func TestStoreResolveWaiterReturnsWhenItsContextEndsWithRacing(t *testing.T) {
	t.Parallel()

	s := &hclparse.Store{}
	k := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Stage: hclparse.StageQueue}
	started := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_, _ = s.Resolve(t.Context(), k, func(context.Context) (string, error) {
			close(started)
			<-release

			return "value", nil
		})
	}()

	<-started

	waiterCtx, cancelWaiter := context.WithCancel(t.Context())
	cancelWaiter()

	_, err := s.Resolve(waiterCtx, k, func(context.Context) (string, error) { return "", nil })
	require.ErrorIs(t, err, context.Canceled)

	close(release)
}

func TestStoreResolvePanicReachesWaitersWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		s := &hclparse.Store{}
		k := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Stage: hclparse.StageQueue}
		started := make(chan struct{})
		release := make(chan struct{})
		leaderPanic := make(chan any, 1)

		go func() {
			defer func() {
				leaderPanic <- recover()
			}()

			_, _ = s.Resolve(t.Context(), k, func(context.Context) (string, error) {
				close(started)
				<-release
				panic("boom")
			})
		}()

		<-started

		waiterDone := make(chan error, 1)

		go func() {
			_, err := s.Resolve(t.Context(), k, func(context.Context) (string, error) { return "", nil })
			waiterDone <- err
		}()

		synctest.Wait()
		close(release)

		assert.Equal(t, "boom", <-leaderPanic)

		var aborted hclparse.FlightAbortedError

		require.ErrorAs(t, <-waiterDone, &aborted)
		assert.Equal(t, "boom", aborted.Panic)
		assert.Equal(t, k, aborted.Key)
	})
}

func TestStoreResolveCollectsReadsOfNestedRuns(t *testing.T) {
	t.Parallel()

	s := &hclparse.Store{}
	outer := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Context: "unit", Stage: hclparse.StageQueue}
	inner := hclparse.Key{Path: "/repo/root.hcl", Context: "unit", Stage: hclparse.StageQueue}

	body := func(ctx context.Context) (string, error) {
		hclparse.NoteReads(ctx, "/repo/unit/values.yaml")

		res, err := s.Resolve(ctx, inner, func(ctx context.Context) (string, error) {
			hclparse.NoteReads(ctx, "/repo/common.yaml")

			return "root", nil
		})
		if err != nil {
			return "", err
		}

		return res.Value, nil
	}

	first, err := s.Resolve(t.Context(), outer, body)
	require.NoError(t, err)
	assert.Equal(t, []string{"/repo/common.yaml", "/repo/unit/values.yaml"}, first.Reads)

	second, err := s.Resolve(t.Context(), outer, body)
	require.NoError(t, err)
	assert.True(t, second.Hit)
	assert.Equal(t, first.Reads, second.Reads)

	hclparse.NoteReads(t.Context(), "/nowhere")
}

func TestStoreInvalidateDropsReaders(t *testing.T) {
	t.Parallel()

	s := &hclparse.Store{}
	unit := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Context: "unit", Stage: hclparse.StageQueue}
	other := hclparse.Key{Path: "/repo/other/terragrunt.hcl", Context: "other", Stage: hclparse.StageQueue}
	runs := map[hclparse.Key]int{}

	resolve := func(k hclparse.Key, reads ...string) hclparse.Resolved[string] {
		res, err := s.Resolve(t.Context(), k, func(ctx context.Context) (string, error) {
			runs[k]++

			hclparse.NoteReads(ctx, reads...)

			return k.Path, nil
		})
		require.NoError(t, err)

		return res
	}

	resolve(unit, "/repo/common.yaml")
	resolve(other)

	s.Invalidate("/repo/common.yaml")

	assert.False(t, resolve(unit, "/repo/common.yaml").Hit, "a reader of the file is dropped")
	assert.True(t, resolve(other).Hit, "a non-reader is kept")

	s.Invalidate("/repo/other/../other/terragrunt.hcl")

	assert.False(t, resolve(other).Hit, "the file's own entry is dropped by its cleaned path")
	assert.Equal(t, map[hclparse.Key]int{unit: 2, other: 2}, runs)
}

func TestStoreASTSharesOneParse(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	s := &hclparse.Store{}

	first, err := s.AST(t.Context(), fsys, path, stat(t, fsys, path), pkghclparse.NewParser())
	require.NoError(t, err)

	parser := pkghclparse.NewParser()

	second, err := s.AST(t.Context(), fsys, path, stat(t, fsys, path), parser)
	require.NoError(t, err)

	assert.Same(t, first.File, second.File, "the syntax tree is shared")
	assert.Same(t, parser, second.Parser, "the tree is bound to the caller's parser")
	assert.NotSame(t, first.Parser, second.Parser)
	assert.Equal(t, hclparse.Stats{Hits: 1, Misses: 1}, s.Stats())
}

func TestStoreASTParsesAgainWhenTheFileChanges(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	s := &hclparse.Store{}

	first, err := s.AST(t.Context(), fsys, path, stat(t, fsys, path), pkghclparse.NewParser())
	require.NoError(t, err)

	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 12 }`), 0o644))

	second, err := s.AST(t.Context(), fsys, path, stat(t, fsys, path), pkghclparse.NewParser())
	require.NoError(t, err)

	assert.NotSame(t, first.File, second.File)
	assert.Equal(t, `locals { a = 12 }`, second.Content())
	assert.Equal(t, hclparse.Stats{Hits: 0, Misses: 2}, s.Stats())
}

func TestStoreASTDoesNotRetainParseErrors(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals {`), 0o644))

	s := &hclparse.Store{}

	_, err := s.AST(t.Context(), fsys, path, stat(t, fsys, path), pkghclparse.NewParser())
	require.Error(t, err)

	_, err = s.AST(t.Context(), fsys, path, stat(t, fsys, path), pkghclparse.NewParser())
	require.Error(t, err)

	assert.Equal(t, hclparse.Stats{Hits: 0, Misses: 2}, s.Stats())
}

func TestStoreASTJoinsTheEnclosingRun(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	s := &hclparse.Store{}
	unit := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Context: "unit", Stage: hclparse.StageQueue}

	body := func(ctx context.Context) (string, error) {
		_, err := s.AST(ctx, fsys, path, stat(t, fsys, path), pkghclparse.NewParser())

		return "unit", err
	}

	_, err := s.Resolve(t.Context(), unit, body)
	require.NoError(t, err)

	s.Invalidate(path)

	res, err := s.Resolve(t.Context(), unit, body)
	require.NoError(t, err)
	assert.False(t, res.Hit, "invalidating a parsed file drops the run that parsed it")
}

func BenchmarkStoreResolveHit(b *testing.B) {
	s := &hclparse.Store{}
	k := hclparse.Key{Path: "/repo/unit/terragrunt.hcl", Context: "unit", Stage: hclparse.StageQueue}
	body := func(context.Context) (string, error) { return "value", nil }

	_, err := s.Resolve(b.Context(), k, body)
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		_, err := s.Resolve(b.Context(), k, body)
		require.NoError(b, err)
	}
}

func BenchmarkStoreASTHit(b *testing.B) {
	fsys := vfs.NewMemMapFS()
	path := filepath.Join(b.TempDir(), "root.hcl")
	require.NoError(b, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	s := &hclparse.Store{}
	info := stat(b, fsys, path)

	_, err := s.AST(b.Context(), fsys, path, info, pkghclparse.NewParser())
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		_, err := s.AST(b.Context(), fsys, path, info, pkghclparse.NewParser())
		require.NoError(b, err)
	}
}

func stat(tb testing.TB, fsys vfs.FS, path string) fs.FileInfo {
	tb.Helper()

	info, err := fsys.Stat(path)
	require.NoError(tb, err)

	return info
}
