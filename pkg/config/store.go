package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// Stage names a step of a config parse whose result the Store memoizes.
type Stage uint8

const (
	// StageAST is a file parsed to its syntax tree, whose [Key] has no Context.
	StageAST Stage = iota + 1
	// StageBase is a file's base blocks evaluated, once per [Key.Context].
	StageBase
	// StageQueue is a file decoded with no dependency outputs in scope, once per [Key.Context].
	StageQueue
	// StageDependencyScan is a file scanned for dependency and dependencies blocks, whose [Key] has no Context.
	StageDependencyScan
)

// Key identifies one memoized result.
type Key struct {
	// Path is the cleaned path of the file the result is of.
	Path string
	// Context is a digest of every setting beyond the file that the stage reads, empty for a stage that reads none.
	Context string
	// Stage is the step the result is of.
	Stage Stage
}

// Store runs the stages of config parses and memoizes those a [Stage] names. It retains a result whose [Key] has no
// Context for the life of a process. A resolution releases the results with a Context that its runs led when it
// returns, unless it runs under another resolution, which releases them instead. The zero value is ready to use, and
// every method is safe for concurrent use.
//
// A unit's stages are [Store.ParseUnitAST], [Store.ResolveUnitBaseVariables], [Store.ResolveUnitQueueConfig], and
// [Store.ResolveRunConfig], each reading more of the file than the last.
//
// Each key holds a promise. The first caller of a key runs its body while later callers wait for the outcome. A
// result is retained only when the body returns no error before its caller's context ends: an error is recomputed by
// the next caller, and a body that outlives its caller's context stops nobody, as the waiters run the body again under
// their own.
//
// Only [Store.ParseUnitAST] and the scan of [StageDependencyScan] notice a file changing on disk. Every other stage serves its result until
// [Store.Invalidate] or [Store.InvalidateUnder] drops it, so whatever writes a config file a Store may hold has to
// call one of them.
//
// A body that memoizes its own key, directly or through the bodies it calls, waits for itself.
type Store struct {
	entries map[Key]*entry
	mu      sync.Mutex
	hits    atomic.Uint64
	misses  atomic.Uint64
}

// Memoized is the outcome of [Store.Memoize].
type Memoized[T any] struct {
	// Value is what the body returned.
	Value T
	// Reads is every path reported through [NoteReads] while the value was computed, including by the bodies nested
	// under it, in lexical order.
	Reads []string
	// Hit is true when another call ran the body.
	Hit bool
}

// Stats counts the outcomes of [Store.Memoize].
type Stats struct {
	// Hits is the calls that received the result of another call's body.
	Hits uint64
	// Misses is the calls that ran a body.
	Misses uint64
}

// entry is the promise of one key.
type entry struct {
	// done closes once the body has returned.
	done chan struct{}
	// val is what the body returned.
	val any
	// err is the error the body returned.
	err error
	// reads is the paths [NoteReads] reported to the body.
	reads map[string]struct{}
	// files is reads plus the path of every file the body parsed through the Store or reported through [NoteFiles],
	// other than path.
	files map[string]struct{}
	// path is the cleaned path of the file the run is keyed by.
	path string
	// mu guards reads and files.
	mu sync.Mutex
	// abandoned is true when the body returned after its caller's context ended.
	abandoned bool
}

// FlightAbortedError is what a call waiting on a [Store] run receives when the run's body panicked or exited its
// goroutine instead of returning.
type FlightAbortedError struct {
	// Panic is the value the body panicked with, nil when it exited.
	Panic any
	// Key is the key of the run.
	Key Key
}

func (e FlightAbortedError) Error() string {
	if e.Panic == nil {
		return fmt.Sprintf("parse of %s exited before it returned", e.Key.Path)
	}

	return fmt.Sprintf("parse of %s panicked: %v", e.Key.Path, e.Panic)
}

// flightKey is the context key of the entry whose body is running.
type flightKey struct{}

// flightEnd is how a run of a body ended.
type flightEnd uint8

const (
	// flightCompleted is a body that returned while its caller's context was live.
	flightCompleted flightEnd = iota + 1
	// flightAbandoned is a body that returned after its caller's context ended, whose result nobody receives but that
	// caller.
	flightAbandoned
)

// flightRole is what a call did for the run it received a result from.
type flightRole uint8

const (
	// flightLed is the call that ran the body.
	flightLed flightRole = iota + 1
	// flightJoined is a call that received the result of another call's body.
	flightJoined
)

// Memoize returns the value of k, running fn to produce it when no call has yet. Concurrent calls for k share
// one run of fn. The value is retained only when fn returns nil before ctx ends.
//
// fn receives a context that carries ctx's values and identifies the run: the reads of every [Store.ParseUnitAST] and
// [NoteReads] call and of every nested Memoize under it join the entry.
//
// Returns fn's error, with fn's value, when the run this call joined failed. A call that joined a run which
// outlived its own caller's context runs fn again. Returns ctx.Err() with no value when ctx ends while the call waits,
// or before it can run fn again.
//
// Panics again on the caller that ran fn when fn panics. Callers waiting on that run receive
// [FlightAbortedError].
func (s *Store) Memoize[T any](
	ctx context.Context,
	k Key,
	fn func(context.Context) (T, error),
) (Memoized[T], error) {
	for {
		e, lead := s.join(k)
		if lead {
			s.misses.Add(1)
			noteRelease(ctx, k)

			return s.run(ctx, k, e, fn)
		}

		select {
		case <-e.done:
			if e.abandoned {
				if err := ctx.Err(); err != nil {
					var zero Memoized[T]

					return zero, err
				}

				continue
			}

			s.hits.Add(1)
			e.propagate(ctx)

			return memoized[T](e, flightJoined), e.err
		case <-ctx.Done():
			var zero Memoized[T]

			return zero, ctx.Err()
		}
	}
}

// ParseUnitAST returns the syntax tree of the file at path, parsed by the first caller and bound to parser for every
// caller. info is the file's current metadata, and a retained tree is parsed again when the file's size or modification
// time no longer matches the one it was parsed from.
func (s *Store) ParseUnitAST(
	ctx context.Context,
	fsys vfs.FS,
	path string,
	info fs.FileInfo,
	parser *hclparse.Parser,
) (*hclparse.File, error) {
	k := Key{Path: filepath.Clean(path), Stage: StageAST}

	file, err := s.memoizeStamped(ctx, k, info, func(context.Context) (*hclparse.File, error) {
		file, err := parser.ParseFromFile(fsys, path)
		if err != nil {
			return nil, err
		}

		return file.WithoutParser(), nil
	})
	if err != nil {
		return nil, err
	}

	return file.Rebind(parser), nil
}

// hasDependencyBlock reports whether the file at path declares a dependency or dependencies block. The first caller
// scans the file, and it is scanned again when its size or modification time changes.
//
// Returns [DependencyFileNotFoundError] when the file does not exist.
func (s *Store) hasDependencyBlock(ctx context.Context, fsys vfs.FS, path string) (bool, error) {
	info, err := fsys.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, DependencyFileNotFoundError{Path: path}
	}

	if err != nil {
		return false, err
	}

	k := Key{Path: filepath.Clean(path), Stage: StageDependencyScan}

	return s.memoizeStamped(ctx, k, info, func(context.Context) (bool, error) {
		return ConfigFileHasDependencyBlock(fsys, path)
	})
}

// memoizeStamped returns the value and error of k as [Store.Memoize] does, where fn reads the value from the file
// that info describes. A retained value read from another version of the file is dropped first. A call that joined a
// run which read another version runs fn again outside s, whether that run succeeded or failed.
func (s *Store) memoizeStamped[T any](
	ctx context.Context,
	k Key,
	info fs.FileInfo,
	fn func(context.Context) (T, error),
) (T, error) {
	s.dropStale(k, info)

	stamp := entryStampOf(info)

	res, err := s.Memoize(ctx, k, func(ctx context.Context) (*stampedValue[T], error) {
		val, err := fn(ctx)

		return &stampedValue[T]{value: val, stamp: stamp}, err
	})
	if res.Value == nil {
		var zero T

		return zero, err
	}

	if !res.Value.stamp.matches(info) {
		return fn(ctx)
	}

	return res.Value.value, err
}

// Invalidate drops every retained result of the file at path and of every result whose run parsed it through
// [Store.ParseUnitAST] or reported it through [NoteReads] or [NoteFiles]. A run in progress is dropped as well, and
// the calls already waiting on it still receive its result.
func (s *Store) Invalidate(path string) {
	path = filepath.Clean(path)

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, e := range s.entries {
		if e.read(path) {
			delete(s.entries, k)
		}
	}
}

// InvalidateUnder drops what [Store.Invalidate] would for every file in dir and below it.
func (s *Store) InvalidateUnder(dir string) {
	dir = filepath.Clean(dir)

	s.mu.Lock()
	defer s.mu.Unlock()

	for k, e := range s.entries {
		if e.readUnder(dir) {
			delete(s.entries, k)
		}
	}
}

// Stats returns the counts so far.
func (s *Store) Stats() Stats {
	return Stats{Hits: s.hits.Load(), Misses: s.misses.Load()}
}

// NoteReads reports to the run ctx belongs to that its body read paths. It does nothing outside a run.
func NoteReads(ctx context.Context, paths ...string) {
	e, ok := ctx.Value(flightKey{}).(*entry)
	if !ok {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, path := range paths {
		e.reads = addPath(e.reads, path)
		e.files = addPath(e.files, filepath.Clean(path))
	}
}

// NoteFiles reports to the run ctx belongs to that its result depends on paths, which [Store.Invalidate] then finds,
// without reporting them as reads. It does nothing outside a run.
func NoteFiles(ctx context.Context, paths ...string) {
	e, ok := ctx.Value(flightKey{}).(*entry)
	if !ok {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, path := range paths {
		e.files = addPath(e.files, filepath.Clean(path))
	}
}

// addPath returns set with path added, making set when it is nil.
func addPath(set map[string]struct{}, path string) map[string]struct{} {
	if set == nil {
		set = make(map[string]struct{})
	}

	set[path] = struct{}{}

	return set
}

// entryStamp is the metadata of the file a value was read from.
type entryStamp struct {
	modTime time.Time
	size    int64
}

// entryStampOf returns the stamp of the file info describes.
func entryStampOf(info fs.FileInfo) entryStamp {
	return entryStamp{modTime: info.ModTime(), size: info.Size()}
}

// matches reports whether info describes the file as it was when the stamp was taken.
func (stamp entryStamp) matches(info fs.FileInfo) bool {
	return stamp.size == info.Size() && stamp.modTime.Equal(info.ModTime())
}

// stamped is a value that goes stale when the file it was read from changes.
type stamped interface {
	// readFrom returns the stamp of the file the value was read from.
	readFrom() entryStamp
}

// stampedValue is a value read from a file, with the stamp of the file it was read from.
type stampedValue[T any] struct {
	value T
	stamp entryStamp
}

func (v *stampedValue[T]) readFrom() entryStamp {
	return v.stamp
}

// join returns the entry of k and whether the caller has to run its body.
func (s *Store) join(k Key) (*entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if e, ok := s.entries[k]; ok {
		return e, false
	}

	if s.entries == nil {
		s.entries = make(map[Key]*entry)
	}

	e := &entry{
		done: make(chan struct{}),
		path: filepath.Clean(k.Path),
	}
	s.entries[k] = e

	return e, true
}

// run executes fn for e and publishes its outcome.
func (s *Store) run[T any](
	ctx context.Context,
	k Key,
	e *entry,
	fn func(context.Context) (T, error),
) (Memoized[T], error) {
	returned := false

	defer func() {
		if returned {
			return
		}

		r := recover()

		var zero T

		s.finish(k, e, zero, FlightAbortedError{Key: k, Panic: r}, flightCompleted)

		if r != nil {
			panic(r)
		}
	}()

	val, err := fn(context.WithValue(ctx, flightKey{}, e))
	returned = true

	end := flightCompleted
	if ctx.Err() != nil {
		end = flightAbandoned
	}

	s.finish(k, e, val, err, end)
	e.propagate(ctx)

	return memoized[T](e, flightLed), err
}

// finish records the outcome of e's body, drops e from the map when the body failed or was abandoned, and wakes the
// waiters.
func (s *Store) finish(k Key, e *entry, val any, err error, end flightEnd) {
	s.mu.Lock()

	e.val = val
	e.err = err
	e.abandoned = end == flightAbandoned

	if (err != nil || e.abandoned) && s.entries[k] == e {
		delete(s.entries, k)
	}

	s.mu.Unlock()

	close(e.done)
}

// releaseScope is the keys with a Context that the runs under one resolution led, which the resolution releases when
// it ends.
type releaseScope struct {
	keys []Key
	mu   sync.Mutex
}

// releaseScopeKey is the context key of the [releaseScope] a run belongs to.
type releaseScopeKey struct{}

// withReleaseScope returns a copy of ctx whose runs note their keys to a new scope, and the func that releases them.
// Under a scope already, it returns ctx and a func that does nothing, so the results stay shared until the outermost
// resolution returns.
func (s *Store) withReleaseScope(ctx context.Context) (context.Context, func()) {
	if _, ok := ctx.Value(releaseScopeKey{}).(*releaseScope); ok {
		return ctx, func() {}
	}

	scope := new(releaseScope)

	return context.WithValue(ctx, releaseScopeKey{}, scope), func() { s.releaseScope(scope) }
}

// noteRelease adds k to the scope of ctx when k has a Context. It does nothing outside a scope.
func noteRelease(ctx context.Context, k Key) {
	if k.Context == "" {
		return
	}

	scope, ok := ctx.Value(releaseScopeKey{}).(*releaseScope)
	if !ok {
		return
	}

	scope.add(k)
}

// add notes k to r.
func (r *releaseScope) add(k Key) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.keys = append(r.keys, k)
}

// noted returns the keys noted to r so far.
func (r *releaseScope) noted() []Key {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.keys)
}

// releaseScope drops the retained value of each key of scope. A run in progress is left alone.
func (s *Store) releaseScope(scope *releaseScope) {
	keys := scope.noted()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, k := range keys {
		e, ok := s.entries[k]
		if !ok {
			continue
		}

		select {
		case <-e.done:
			delete(s.entries, k)
		default:
		}
	}
}

// dropStale drops the retained value of k when info no longer matches the file it was read from. A run in
// progress is left alone.
func (s *Store) dropStale(k Key, info fs.FileInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.entries[k]
	if !ok {
		return
	}

	select {
	case <-e.done:
	default:
		return
	}

	val, ok := e.val.(stamped)
	if !ok {
		return
	}

	if !val.readFrom().matches(info) {
		delete(s.entries, k)
	}
}

// memoized returns e's outcome as a [Memoized]. The value is the zero value when the body returned none.
func memoized[T any](e *entry, role flightRole) Memoized[T] {
	val, _ := e.val.(T)

	e.mu.Lock()
	defer e.mu.Unlock()

	return Memoized[T]{Value: val, Reads: slices.Sorted(maps.Keys(e.reads)), Hit: role == flightJoined}
}

// propagate adds e's reads and files to the run ctx belongs to, if any.
func (e *entry) propagate(ctx context.Context) {
	parent, ok := ctx.Value(flightKey{}).(*entry)
	if !ok || parent == e {
		return
	}

	e.mu.Lock()
	reads := maps.Clone(e.reads)
	files := maps.Clone(e.files)
	e.mu.Unlock()

	parent.mu.Lock()
	defer parent.mu.Unlock()

	for path := range reads {
		parent.reads = addPath(parent.reads, path)
	}

	parent.files = addPath(parent.files, e.path)

	for path := range files {
		parent.files = addPath(parent.files, path)
	}
}

// read reports whether e's body parsed or read path.
func (e *entry) read(path string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	_, ok := e.files[path]

	return ok || path == e.path
}

// readUnder reports whether e's body parsed or read a file in dir or below it.
func (e *entry) readUnder(dir string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	prefix := strings.TrimSuffix(dir, string(filepath.Separator)) + string(filepath.Separator)

	if strings.HasPrefix(e.path, prefix) {
		return true
	}

	for path := range e.files {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}

	return false
}
