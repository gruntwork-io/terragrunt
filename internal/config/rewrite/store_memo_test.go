package config_test

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

	config "github.com/gruntwork-io/terragrunt/internal/config/rewrite"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	pkghclparse "github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

var errBody = errors.New("body failed")

// outcome is what one caller got back from Memoize.
type outcome struct {
	err error
	res config.Memoized[string]
}

func TestStoreMemoizeRunsBodyOnce(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	k := config.Key{Path: "/repo/unit/terragrunt.hcl", Context: "ctx", Stage: config.StageQueue}
	runs := 0

	body := func(context.Context) (string, error) {
		runs++

		return "value", nil
	}

	first, err := s.Memoize(t.Context(), k, body)
	require.NoError(t, err)
	assert.Equal(t, "value", first.Value)
	assert.False(t, first.Hit)

	second, err := s.Memoize(t.Context(), k, body)
	require.NoError(t, err)
	assert.Equal(t, "value", second.Value)
	assert.True(t, second.Hit)

	assert.Equal(t, 1, runs)
	assert.Equal(t, config.Stats{Hits: 1, Misses: 1}, s.Stats())
}

func TestStoreMemoizeKeysOnEveryField(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	base := config.Key{Path: "/repo/unit/terragrunt.hcl", Context: "ctx", Stage: config.StageQueue}
	keys := []config.Key{
		base,
		{Path: "/repo/other/terragrunt.hcl", Context: "ctx", Stage: config.StageQueue},
		{Path: base.Path, Context: "other", Stage: config.StageQueue},
		{Path: base.Path, Context: "ctx", Stage: config.StageAST},
	}

	for i, k := range keys {
		res, err := s.Memoize(t.Context(), k, func(context.Context) (int, error) { return i, nil })
		require.NoError(t, err)
		assert.False(t, res.Hit)
		assert.Equal(t, i, res.Value)
	}
}

func TestStoreMemoizeDoesNotRetainErrors(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	k := config.Key{Path: "/repo/unit/terragrunt.hcl", Stage: config.StageQueue}
	runs := 0

	failing := func(context.Context) (string, error) {
		runs++

		return "partial", errBody
	}

	res, err := s.Memoize(t.Context(), k, failing)
	require.ErrorIs(t, err, errBody)
	assert.Equal(t, "partial", res.Value, "the value travels with the error")

	res, err = s.Memoize(t.Context(), k, failing)
	require.ErrorIs(t, err, errBody)
	assert.False(t, res.Hit)
	assert.Equal(t, 2, runs)

	res, err = s.Memoize(t.Context(), k, func(context.Context) (string, error) { return "ok", nil })
	require.NoError(t, err)
	assert.False(t, res.Hit)
	assert.Equal(t, "ok", res.Value)
}

func TestStoreMemoizeSharesOneRunWithRacing(t *testing.T) {
	t.Parallel()

	const callers = 16

	s := &config.Store{}
	k := config.Key{Path: "/repo/unit/terragrunt.hcl", Stage: config.StageQueue}

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
			res, err := s.Memoize(t.Context(), k, body)

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

func TestStoreMemoizeWaiterRunsAgainAfterLeaderCancelsWithRacing(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	k := config.Key{Path: "/repo/unit/terragrunt.hcl", Stage: config.StageQueue}

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
		_, err := s.Memoize(leaderCtx, k, body)
		leaderDone <- err
	}()

	<-started

	waiterDone := make(chan outcome, 1)

	go func() {
		res, err := s.Memoize(t.Context(), k, body)

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

func TestStoreMemoizeDoesNotRetainABodyThatOutlivedItsContext(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	k := config.Key{Path: "/repo/unit/terragrunt.hcl", Stage: config.StageQueue}

	ctx, cancel := context.WithCancel(t.Context())

	_, err := s.Memoize(ctx, k, func(context.Context) (string, error) {
		cancel()

		return "fallback", nil
	})
	require.NoError(t, err)

	res, err := s.Memoize(t.Context(), k, func(context.Context) (string, error) {
		return "value", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "value", res.Value)
	assert.False(t, res.Hit)
}

func TestStoreMemoizeWaiterWithAnEndedContextGetsNoAbandonedResultWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		s := &config.Store{}
		k := config.Key{Path: "/repo/unit/terragrunt.hcl", Stage: config.StageQueue}

		ctx, cancel := context.WithCancel(t.Context())
		release := make(chan struct{})

		go func() {
			_, err := s.Memoize(ctx, k, func(context.Context) (string, error) {
				<-release

				return "fallback", nil
			})
			assert.NoError(t, err)
		}()

		synctest.Wait()

		waiterDone := make(chan outcome, 1)

		go func() {
			res, err := s.Memoize(ctx, k, func(context.Context) (string, error) {
				return "value", nil
			})

			waiterDone <- outcome{res: res, err: err}
		}()

		synctest.Wait()
		cancel()
		close(release)

		waiter := <-waiterDone
		require.ErrorIs(t, waiter.err, context.Canceled)
		assert.Empty(t, waiter.res.Value)
	})
}

func TestStoreMemoizeWaiterReturnsWhenItsContextEndsWithRacing(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	k := config.Key{Path: "/repo/unit/terragrunt.hcl", Stage: config.StageQueue}
	started := make(chan struct{})
	release := make(chan struct{})

	go func() {
		_, _ = s.Memoize(t.Context(), k, func(context.Context) (string, error) {
			close(started)
			<-release

			return "value", nil
		})
	}()

	<-started

	waiterCtx, cancelWaiter := context.WithCancel(t.Context())
	cancelWaiter()

	_, err := s.Memoize(waiterCtx, k, func(context.Context) (string, error) { return "", nil })
	require.ErrorIs(t, err, context.Canceled)

	close(release)
}

func TestStoreMemoizePanicReachesWaitersWithRacing(t *testing.T) {
	t.Parallel()

	synctest.Test(t, func(t *testing.T) {
		s := &config.Store{}
		k := config.Key{Path: "/repo/unit/terragrunt.hcl", Stage: config.StageQueue}
		started := make(chan struct{})
		release := make(chan struct{})
		leaderPanic := make(chan any, 1)

		go func() {
			defer func() {
				leaderPanic <- recover()
			}()

			_, _ = s.Memoize(t.Context(), k, func(context.Context) (string, error) {
				close(started)
				<-release
				panic("boom")
			})
		}()

		<-started

		waiterDone := make(chan error, 1)

		go func() {
			_, err := s.Memoize(
				t.Context(),
				k,
				func(context.Context) (string, error) { return "", nil },
			)
			waiterDone <- err
		}()

		synctest.Wait()
		close(release)

		assert.Equal(t, "boom", <-leaderPanic)

		var aborted config.FlightAbortedError

		require.ErrorAs(t, <-waiterDone, &aborted)
		assert.Equal(t, "boom", aborted.Panic)
		assert.Equal(t, k, aborted.Key)
	})
}

func TestStoreMemoizeCollectsReadsOfNestedRuns(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	outer := config.Key{
		Path:    "/repo/unit/terragrunt.hcl",
		Context: "unit",
		Stage:   config.StageQueue,
	}
	inner := config.Key{Path: "/repo/root.hcl", Context: "unit", Stage: config.StageQueue}

	body := func(ctx context.Context) (string, error) {
		config.NoteReads(ctx, "/repo/unit/values.yaml")

		res, err := s.Memoize(ctx, inner, func(ctx context.Context) (string, error) {
			config.NoteReads(ctx, "/repo/common.yaml")

			return "root", nil
		})
		if err != nil {
			return "", err
		}

		return res.Value, nil
	}

	first, err := s.Memoize(t.Context(), outer, body)
	require.NoError(t, err)
	assert.Equal(t, []string{"/repo/common.yaml", "/repo/unit/values.yaml"}, first.Reads)

	second, err := s.Memoize(t.Context(), outer, body)
	require.NoError(t, err)
	assert.True(t, second.Hit)
	assert.Equal(t, first.Reads, second.Reads)

	config.NoteReads(t.Context(), "/nowhere")
}

func TestStoreInvalidateFindsNotedFilesThatAreNotReads(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	k := config.Key{Path: "/repo/unit/terragrunt.hcl", Context: "unit", Stage: config.StageQueue}
	runs := 0

	memoize := func() config.Memoized[string] {
		res, err := s.Memoize(t.Context(), k, func(ctx context.Context) (string, error) {
			runs++

			config.NoteFiles(ctx, "/repo/dep/../dep/terragrunt.hcl")

			return k.Path, nil
		})
		require.NoError(t, err)

		return res
	}

	assert.Empty(t, memoize().Reads)
	memoize()
	assert.Equal(t, 1, runs)

	s.Invalidate("/repo/dep/terragrunt.hcl")
	memoize()
	assert.Equal(t, 2, runs)
}

func TestStoreInvalidateDropsReaders(t *testing.T) {
	t.Parallel()

	s := &config.Store{}
	unit := config.Key{Path: "/repo/unit/terragrunt.hcl", Context: "unit", Stage: config.StageQueue}
	other := config.Key{
		Path:    "/repo/other/terragrunt.hcl",
		Context: "other",
		Stage:   config.StageQueue,
	}
	runs := map[config.Key]int{}

	resolve := func(k config.Key, reads ...string) config.Memoized[string] {
		res, err := s.Memoize(t.Context(), k, func(ctx context.Context) (string, error) {
			runs[k]++

			config.NoteReads(ctx, reads...)

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
	assert.Equal(t, map[config.Key]int{unit: 2, other: 2}, runs)
}

func TestStoreParseUnitASTSharesOneParse(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	s := &config.Store{}

	first, err := s.ParseUnitAST(
		t.Context(),
		fsys,
		path,
		stat(t, fsys, path),
		pkghclparse.NewParser(),
	)
	require.NoError(t, err)

	parser := pkghclparse.NewParser()

	second, err := s.ParseUnitAST(t.Context(), fsys, path, stat(t, fsys, path), parser)
	require.NoError(t, err)

	assert.Same(t, first.File, second.File, "the syntax tree is shared")
	assert.Same(t, parser, second.Parser, "the tree is bound to the caller's parser")
	assert.NotSame(t, first.Parser, second.Parser)
	assert.Equal(t, config.Stats{Hits: 1, Misses: 1}, s.Stats())
}

func TestStoreParseUnitASTParsesAgainWhenTheFileChanges(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	s := &config.Store{}

	first, err := s.ParseUnitAST(
		t.Context(),
		fsys,
		path,
		stat(t, fsys, path),
		pkghclparse.NewParser(),
	)
	require.NoError(t, err)

	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 12 }`), 0o644))

	second, err := s.ParseUnitAST(
		t.Context(),
		fsys,
		path,
		stat(t, fsys, path),
		pkghclparse.NewParser(),
	)
	require.NoError(t, err)

	assert.NotSame(t, first.File, second.File)
	assert.Equal(t, `locals { a = 12 }`, second.Content())
	assert.Equal(t, config.Stats{Hits: 0, Misses: 2}, s.Stats())
}

func TestStoreParseUnitASTDoesNotRetainParseErrors(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals {`), 0o644))

	s := &config.Store{}

	_, err := s.ParseUnitAST(t.Context(), fsys, path, stat(t, fsys, path), pkghclparse.NewParser())
	require.Error(t, err)

	_, err = s.ParseUnitAST(t.Context(), fsys, path, stat(t, fsys, path), pkghclparse.NewParser())
	require.Error(t, err)

	assert.Equal(t, config.Stats{Hits: 0, Misses: 2}, s.Stats())
}

func TestStoreParseUnitASTJoinsTheEnclosingRun(t *testing.T) {
	t.Parallel()

	fsys := vfs.NewMemMapFS()
	path := filepath.Join(t.TempDir(), "root.hcl")
	require.NoError(t, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	s := &config.Store{}
	unit := config.Key{Path: "/repo/unit/terragrunt.hcl", Context: "unit", Stage: config.StageQueue}

	body := func(ctx context.Context) (string, error) {
		_, err := s.ParseUnitAST(ctx, fsys, path, stat(t, fsys, path), pkghclparse.NewParser())

		return "unit", err
	}

	_, err := s.Memoize(t.Context(), unit, body)
	require.NoError(t, err)

	s.Invalidate(path)

	res, err := s.Memoize(t.Context(), unit, body)
	require.NoError(t, err)
	assert.False(t, res.Hit, "invalidating a parsed file drops the run that parsed it")
}

func BenchmarkStoreResolveHit(b *testing.B) {
	s := &config.Store{}
	k := config.Key{Path: "/repo/unit/terragrunt.hcl", Context: "unit", Stage: config.StageQueue}
	body := func(context.Context) (string, error) { return "value", nil }

	_, err := s.Memoize(b.Context(), k, body)
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		_, err := s.Memoize(b.Context(), k, body)
		require.NoError(b, err)
	}
}

func BenchmarkStoreASTHit(b *testing.B) {
	fsys := vfs.NewMemMapFS()
	path := filepath.Join(b.TempDir(), "root.hcl")
	require.NoError(b, vfs.WriteFile(fsys, path, []byte(`locals { a = 1 }`), 0o644))

	s := &config.Store{}
	info := stat(b, fsys, path)

	_, err := s.ParseUnitAST(b.Context(), fsys, path, info, pkghclparse.NewParser())
	require.NoError(b, err)

	b.ResetTimer()

	for b.Loop() {
		_, err := s.ParseUnitAST(b.Context(), fsys, path, info, pkghclparse.NewParser())
		require.NoError(b, err)
	}
}

func stat(tb testing.TB, fsys vfs.FS, path string) fs.FileInfo {
	tb.Helper()

	info, err := fsys.Stat(path)
	require.NoError(tb, err)

	return info
}
