// Package vfs provides a virtual filesystem abstraction for testing and production use.
// It wraps afero to provide a consistent interface for filesystem operations.
package vfs

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charlievieth/fastwalk"
	"github.com/gofrs/flock"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/spf13/afero"
)

// FS is the filesystem interface used throughout the codebase.
// It provides an abstraction over real and in-memory filesystems.
type FS = afero.Fs

// File represents a file in the filesystem.
type File = afero.File

// HardLinker is an optional interface for filesystems that support hard links.
type HardLinker interface {
	LinkIfPossible(oldname, newname string) error
}

// Unlocker can release a held lock.
type Unlocker interface {
	Unlock() error
}

// Locker is an optional interface for filesystems that support locking.
type Locker interface {
	// Lock acquires a blocking lock for the given name.
	Lock(name string) (Unlocker, error)
	// TryLock attempts a non-blocking lock for the given name.
	// Returns the unlocker and true if acquired, nil and false otherwise.
	TryLock(name string) (Unlocker, bool, error)
}

// symlinkEvaluator lets filesystem implementations provide native symlink resolution.
type symlinkEvaluator interface {
	EvalSymlinksIfPossible(name string) (string, bool, error)
}

// ContextLocker is an optional interface for filesystems whose locks can be
// acquired with a context. Implementations should poll/retry until the lock
// is acquired or ctx is canceled. On ctx cancellation, the returned error
// wraps ctx.Err(). Implementations also impose a hard upper bound on the
// total wait (see [maxLockWait]), so a never-canceled ctx will still return
// after that bound elapses.
type ContextLocker interface {
	LockContext(ctx context.Context, name string) (Unlocker, error)
}

const maxSymlinkEvaluations = 255

// NewOSFS returns a filesystem backed by the real operating system filesystem.
func NewOSFS() FS {
	return &osFS{afero.NewOsFs()}
}

// IsOSFS reports whether fsys is the OS-backed filesystem from [NewOSFS].
// Callers that shell out to processes which only see the real disk (e.g.
// `git`) should reject other filesystems up front rather than failing
// inside the subprocess.
func IsOSFS(fsys FS) bool {
	_, ok := fsys.(*osFS)
	return ok
}

// NewMemMapFS returns an in-memory filesystem for testing purposes.
// The returned filesystem supports symlink operations via an in-memory link table.
func NewMemMapFS() FS {
	return &memMapFS{
		Fs:       afero.NewMemMapFs(),
		symlinks: make(map[string]string),
		locks:    make(map[string]*memLock),
	}
}

// FileExists checks if a path exists using the given filesystem.
// Returns (true, nil) if the file exists, (false, nil) if it does not exist,
// and (false, error) for other errors (e.g., permission denied).
func FileExists(fsys FS, path string) (bool, error) {
	_, err := fsys.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	return false, err
}

// Exists reports whether path is present on the given filesystem. A path that
// cannot be stat'd counts as absent, so an entry the caller may not read reads
// the same as one that is not there. Use [FileExists] to tell those apart.
func Exists(fsys FS, path string) bool {
	_, err := fsys.Stat(path)
	return err == nil
}

// IsDir reports whether path is a directory on the given filesystem,
// following symlinks. A path that cannot be stat'd is not a directory.
func IsDir(fsys FS, path string) bool {
	info, err := fsys.Stat(path)
	return err == nil && info.IsDir()
}

// checksumReadBlock is the block size FileSHA256 hashes with.
const checksumReadBlock = 8192

// IsFile reports whether path points to a regular file. A path that cannot be
// stat'd is not a file.
func IsFile(fsys FS, path string) bool {
	info, err := fsys.Stat(path)
	return err == nil && !info.IsDir()
}

// EnsureDirectory creates the directory at path, and any missing parents, when
// nothing is there yet. A path already occupied by a file yields
// [PathIsNotDirectory].
func EnsureDirectory(fsys FS, path string) error {
	if IsFile(fsys, path) {
		return PathIsNotDirectory{path: path}
	}

	if Exists(fsys, path) {
		return nil
	}

	const ownerReadWriteExecutePerms = 0o700

	return fsys.MkdirAll(path, ownerReadWriteExecutePerms)
}

// IsDirectoryEmpty reports whether path is a directory holding no entries.
func IsDirectoryEmpty(fsys FS, path string) (retEmpty bool, retErr error) {
	dir, err := fsys.Open(path)
	if err != nil {
		return false, err
	}

	defer func() {
		if err := dir.Close(); err != nil && retErr == nil {
			retEmpty, retErr = false, err
		}
	}()

	// Reading a single entry is enough to answer the question, so a directory
	// holding a million files costs the same as one holding one.
	if _, err := dir.Readdir(1); err == nil {
		return false, nil
	}

	return true, nil
}

// FileSHA256 returns the SHA256 of the file at path, read in fixed-size blocks
// so hashing a large archive does not pull it entirely into memory.
func FileSHA256(fsys FS, path string) (_ []byte, retErr error) {
	file, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := file.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	hash := sha256.New()

	if _, err := io.CopyBuffer(hash, file, make([]byte, checksumReadBlock)); err != nil {
		return nil, err
	}

	return hash.Sum(nil), nil
}

// CopyFile copies a file from source to destination on the given filesystem,
// preserving the source's permissions.
func CopyFile(fsys FS, source, destination string) error {
	file, err := fsys.Open(source)
	if err != nil {
		return err
	}

	err = WriteFileWithSamePermissions(fsys, source, destination, file)

	return errors.Join(err, file.Close())
}

// WriteFileWithSamePermissions writes contents to destination using the same
// permissions as the file at source.
func WriteFileWithSamePermissions(fsys FS, source, destination string, contents io.Reader) error {
	fileInfo, err := fsys.Stat(source)
	if err != nil {
		return err
	}

	// CAS may place read-only files at the destination, which would block a plain open.
	if err := fsys.Remove(destination); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	file, err := fsys.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fileInfo.Mode())
	if err != nil {
		return err
	}

	_, err = io.Copy(file, contents)

	return errors.Join(err, file.Close())
}

// Lstat returns the FileInfo for the named path without following symlinks.
// Filesystems that do not implement afero.Lstater fall back to Stat.
func Lstat(fsys FS, path string) (os.FileInfo, error) {
	if lstater, ok := fsys.(afero.Lstater); ok {
		info, _, err := lstater.LstatIfPossible(path)
		return info, err
	}

	return fsys.Stat(path)
}

// WriteFile writes data to a file on the given filesystem.
func WriteFile(fsys FS, filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if err := fsys.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	return afero.WriteFile(fsys, filename, data, perm)
}

// ReadFile reads the contents of a file from the given filesystem.
func ReadFile(fsys FS, filename string) ([]byte, error) {
	return afero.ReadFile(fsys, filename)
}

// ReadFileAsString reads the contents of a file from the given filesystem as a
// string, annotating the failure with the path so a caller reading several
// files can tell which one failed.
func ReadFileAsString(fsys FS, filename string) (string, error) {
	contents, err := ReadFile(fsys, filename)
	if err != nil {
		return "", fmt.Errorf("error reading file at path %s: %w", filename, err)
	}

	return string(contents), nil
}

// ReadFileLimit reads up to limit bytes from the start of a file on the given
// filesystem, for callers that only need a bounded prefix (such as previewing
// the head of a possibly-large file) rather than the whole thing.
func ReadFileLimit(fsys FS, filename string, limit int64) (data []byte, err error) {
	f, err := fsys.Open(filename)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	return io.ReadAll(io.LimitReader(f, limit))
}

// EvalSymlinks returns path after evaluating symlinks using the supplied filesystem.
func EvalSymlinks(fsys FS, path string) (string, error) {
	if evaluator, ok := fsys.(symlinkEvaluator); ok {
		resolved, supported, err := evaluator.EvalSymlinksIfPossible(path)
		if supported {
			return resolved, err
		}
	}

	return walkSymlinks(fsys, path)
}

// ResolveForCompare returns the symlink-resolved form of path, for comparing
// one path against another. Two spellings of the same location must reduce to
// one string or a path-keyed set counts them twice: on macOS /var is a symlink
// to /private/var, so a path under either spelling has to resolve before it is
// compared. [EvalSymlinks] resolves only paths that exist, so resolving the
// longest existing ancestor and rejoining the remaining components keeps an
// absent path comparable with a resolved one instead of leaving it merely
// cleaned.
func ResolveForCompare(fsys FS, path string) string {
	path = filepath.Clean(path)

	if resolved, err := EvalSymlinks(fsys, path); err == nil {
		return resolved
	}

	if parent := filepath.Dir(path); parent != path {
		return filepath.Join(ResolveForCompare(fsys, parent), filepath.Base(path))
	}

	return path
}

// Within reports whether path is dir or a descendant of it, comparing both
// through [ResolveForCompare] so a symlink that leaves dir is caught rather
// than counted as inside it.
func Within(fsys FS, dir, path string) bool {
	rel, err := filepath.Rel(ResolveForCompare(fsys, dir), ResolveForCompare(fsys, path))
	if err != nil {
		return false
	}

	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// ParentPathHasSymlink reports whether rel cannot be safely traversed under rootDir.
// It returns true when rel is empty, ".", absolute, escapes rootDir with "..", or has a symlink in a parent component.
// The final path component is not checked, so callers can safely remove a leaf symlink.
func ParentPathHasSymlink(fsys FS, rootDir, rel string) (bool, error) {
	rel = filepath.Clean(rel)
	if rel == "." || filepath.IsAbs(rel) || rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return true, nil
	}

	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) > 0 {
		parts = parts[:len(parts)-1]
	}

	current := filepath.Clean(rootDir)
	for _, part := range parts {
		current = filepath.Join(current, part)

		info, err := Lstat(fsys, current)
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}

	return false, nil
}

// MkdirTemp creates a temporary directory on the given filesystem. Unlike
// [os.MkdirTemp], prefix is always literal: the random component is appended and
// a "*" in prefix is not treated as a placeholder.
func MkdirTemp(fsys FS, dir, prefix string) (string, error) {
	return afero.TempDir(fsys, dir, prefix)
}

// CreateTemp creates a temporary file on the given filesystem, following the
// same rule as [os.CreateTemp]: the last "*" in pattern is replaced by the
// random component, or, when pattern has no "*", the random component is
// appended.
func CreateTemp(fsys FS, dir, pattern string) (File, error) {
	return afero.TempFile(fsys, dir, pattern)
}

// Link creates a hard link. It delegates to LinkIfPossible for filesystems
// that implement the HardLinker interface.
func Link(fsys FS, oldname, newname string) error {
	linker, ok := fsys.(HardLinker)
	if !ok {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: ErrNoHardLink}
	}

	return linker.LinkIfPossible(oldname, newname)
}

// Symlink creates a symbolic link. It uses afero's SymlinkIfPossible
// which is supported by OsFs and any FS implementing afero.Linker.
func Symlink(fsys FS, oldname, newname string) error {
	linker, ok := fsys.(afero.Linker)
	if !ok {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: afero.ErrNoSymlink}
	}

	return linker.SymlinkIfPossible(oldname, newname)
}

// Readlink reads the target of a symbolic link. It uses afero's
// ReadlinkIfPossible which is supported by OsFs and any FS implementing
// afero.Symlinker.
func Readlink(fsys FS, name string) (string, error) {
	reader, ok := fsys.(afero.Symlinker)
	if !ok {
		return "", &os.PathError{Op: "readlink", Path: name, Err: afero.ErrNoSymlink}
	}

	return reader.ReadlinkIfPossible(name)
}

// Lock acquires a blocking lock for the given name on the filesystem.
func Lock(fsys FS, name string) (Unlocker, error) {
	locker, ok := fsys.(Locker)
	if !ok {
		return nil, ErrNoLock
	}

	return locker.Lock(name)
}

// TryLock attempts a non-blocking lock for the given name on the filesystem.
func TryLock(fsys FS, name string) (Unlocker, bool, error) {
	locker, ok := fsys.(Locker)
	if !ok {
		return nil, false, ErrNoLock
	}

	return locker.TryLock(name)
}

// LockContext acquires a lock for the given name, blocking until it is
// available or ctx is canceled. Filesystems that implement ContextLocker
// use their native context-aware path; otherwise the call falls back to a
// blocking Lock without context support.
//
// ContextLocker implementations cap the total wait at [maxLockWait], so the
// call returns after that bound elapses even when ctx is never canceled.
// Callers that want a shorter deadline should pass a ctx with their own
// timeout.
func LockContext(ctx context.Context, fsys FS, name string) (Unlocker, error) {
	if cl, ok := fsys.(ContextLocker); ok {
		return cl.LockContext(ctx, name)
	}

	return Lock(fsys, name)
}

// WalkDirParallelOption configures a [WalkDirParallel] call.
type WalkDirParallelOption func(*walkDirParallelConfig)

type walkDirParallelConfig struct {
	followSymlinks bool
}

// WithFollowSymlinks makes [WalkDirParallel] descend into directories
// reached through symbolic links. The DirEntry passed to fn for a
// followed symlink reports the target's type, so `d.IsDir()` is true
// for a symlink that resolves to a directory. Infinite loops are
// guarded by fastwalk's ancestor-cycle detection.
//
// Without this option, symlinked directories are visited as single
// entries with `d.IsDir() == false`, matching stdlib [fs.WalkDir].
func WithFollowSymlinks() WalkDirParallelOption {
	return func(c *walkDirParallelConfig) {
		c.followSymlinks = true
	}
}

// WalkDirParallel walks the file tree rooted at root like [WalkDir]
// does. On a [NewOSFS] filesystem it reads directories in parallel via
// [fastwalk.Walk]. On any other FS, including [NewMemMapFS], it falls
// back to the sequential [WalkDir].
//
// The parallel walk calls fn concurrently from multiple goroutines and
// gives no ordering guarantee across directories. Callers that depend
// on deterministic order, or that write to shared state from fn, must
// use [WalkDir] or serialize access themselves.
func WalkDirParallel(fsys FS, root string, fn fs.WalkDirFunc, opts ...WalkDirParallelOption) error {
	if _, ok := fsys.(*osFS); !ok {
		return WalkDir(fsys, root, fn)
	}

	var cfg walkDirParallelConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	var fwCfg *fastwalk.Config
	if cfg.followSymlinks {
		fwCfg = &fastwalk.Config{Follow: true}
	}

	err := fastwalk.Walk(fwCfg, root, fn)

	if errors.Is(err, filepath.SkipDir) || errors.Is(err, filepath.SkipAll) {
		return nil
	}

	return err
}

// WalkDir walks the file tree rooted at root, calling fn for each file or
// directory in the tree, including root. The fn callback receives an [fs.DirEntry]
// instead of [os.FileInfo], which can be more efficient since it does not require
// a stat call for every visited file.
//
// All errors that arise visiting files and directories are filtered by fn:
// see the [fs.WalkDirFunc] documentation for details.
//
// The files are walked in lexical order, which makes the output deterministic
// but means that for very large directories WalkDir can be inefficient.
// WalkDir does not follow symbolic links.
//
// Adapted from spf13/afero#571; replace with afero.WalkDir once merged.
func WalkDir(fsys FS, root string, fn fs.WalkDirFunc) error {
	info, err := lstatIfPossible(fsys, root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walkDir(fsys, root, FileInfoDirEntry{FileInfo: info}, fn)
	}

	if errors.Is(err, filepath.SkipDir) || errors.Is(err, filepath.SkipAll) {
		return nil
	}

	return err
}

// WalkDirWithSymlinks walks the file tree rooted at root like [WalkDir] does,
// additionally descending into the directories that symbolic links resolve to.
// Paths handed to fn are logical: they read as if the link target lived at the
// link's own location, and a root that is itself reached through a link keeps
// the spelling the caller passed.
//
// Each logical path is reported once, so a directory reachable through several
// links is visited once, and a link pointing back at an ancestor terminates
// instead of looping.
func WalkDirWithSymlinks(fsys FS, root string, fn fs.WalkDirFunc) error {
	w := &symlinkWalker{
		fsys:           fsys,
		fn:             fn,
		visited:        make(map[string]bool),
		visitedLogical: make(map[string]bool),
	}

	realRoot, err := EvalSymlinks(fsys, root)
	if err != nil {
		return fmt.Errorf("failed to evaluate symlinks for %s: %w", root, err)
	}

	return w.walk(realRoot, filepath.Clean(root))
}

// symlinkWalker carries the bookkeeping [WalkDirWithSymlinks] needs across the
// nested walks it starts for each followed link.
type symlinkWalker struct {
	fsys           FS
	fn             fs.WalkDirFunc
	visited        map[string]bool
	visitedLogical map[string]bool
}

// walk traverses the tree at physical, reporting entries under logical.
func (w *symlinkWalker) walk(physical, logical string) error {
	return WalkDir(w.fsys, physical, func(current string, d fs.DirEntry, err error) error {
		if err != nil {
			return w.fn(current, d, err)
		}

		rel, err := filepath.Rel(physical, current)
		if err != nil {
			return fmt.Errorf(
				"failed to get relative path between %s and %s: %w",
				physical,
				current,
				err,
			)
		}

		logicalPath := filepath.Join(logical, rel)

		if !w.visitedLogical[logicalPath] {
			w.visitedLogical[logicalPath] = true

			if err := w.fn(logicalPath, d, nil); err != nil {
				return err
			}
		}

		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}

		return w.follow(current, logicalPath)
	})
}

// follow resolves the link at current and, when it lands on a directory,
// walks the target as though it lived at logicalPath.
func (w *symlinkWalker) follow(current, logicalPath string) error {
	realPath, err := EvalSymlinks(w.fsys, current)
	if err != nil {
		return fmt.Errorf("failed to evaluate symlinks for %s: %w", current, err)
	}

	realInfo, err := w.fsys.Stat(realPath)
	if err != nil {
		return fmt.Errorf("failed to describe file %s: %w", realPath, err)
	}

	if w.visited[realPath+":"+current] {
		return nil
	}

	w.visited[realPath+":"+current] = true

	if !realInfo.IsDir() {
		return nil
	}

	return w.walk(realPath, logicalPath)
}

// osFS wraps afero.OsFs with hard link support.
type osFS struct {
	afero.Fs
}

func (fsys *osFS) LinkIfPossible(oldname, newname string) error {
	return os.Link(oldname, newname)
}

func (fsys *osFS) SymlinkIfPossible(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

func (fsys *osFS) ReadlinkIfPossible(name string) (string, error) {
	return os.Readlink(name)
}

func (fsys *osFS) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(name)

	return info, true, err
}

func (*osFS) EvalSymlinksIfPossible(name string) (string, bool, error) {
	resolved, err := filepath.EvalSymlinks(name)

	return resolved, true, err
}

func (fsys *osFS) Lock(name string) (Unlocker, error) {
	l := flock.New(name)
	if err := l.Lock(); err != nil {
		return nil, err
	}

	return l, nil
}

func (fsys *osFS) TryLock(name string) (Unlocker, bool, error) {
	l := flock.New(name)

	acquired, err := l.TryLock()
	if err != nil {
		return nil, false, err
	}

	if !acquired {
		return nil, false, nil
	}

	return l, true, nil
}

const (
	// osFlockRetryDelay is how often osFS.LockContext polls for the flock
	// when the lock is held by another process. gofrs/flock uses a syscall
	// per attempt, so the tick is coarse enough to limit churn while still
	// reacting quickly when the holder releases.
	osFlockRetryDelay = 50 * time.Millisecond

	// memMapLockRetryDelay is how often memMapFS.LockContext retries its
	// in-process sync.Mutex TryLock. The retry is essentially free, so the
	// tick is tighter than [osFlockRetryDelay] to keep tests snappy.
	memMapLockRetryDelay = 10 * time.Millisecond

	// maxLockWait is the hard upper bound on any LockContext call regardless
	// of the caller's context. It guarantees the retry loop terminates even
	// if a caller passes a never-canceled context and the lock is permanently
	// held by another process.
	maxLockWait = 30 * time.Minute
)

func (fsys *osFS) LockContext(ctx context.Context, name string) (Unlocker, error) {
	ctx, cancel := context.WithTimeout(ctx, maxLockWait)
	defer cancel()

	l := flock.New(name)

	acquired, err := l.TryLockContext(ctx, osFlockRetryDelay)
	if err != nil {
		return nil, err
	}

	if !acquired {
		return nil, ctx.Err()
	}

	return l, nil
}

// memMapFS wraps afero.MemMapFs with in-memory symlink support.
type memMapFS struct {
	afero.Fs
	symlinks map[string]string
	locks    map[string]*memLock
	locksMu  sync.Mutex
}

func (fsys *memMapFS) SymlinkIfPossible(oldname, newname string) error {
	if _, exists := fsys.symlinks[newname]; exists {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: os.ErrExist}
	}

	fsys.symlinks[newname] = oldname

	return nil
}

func (fsys *memMapFS) LinkIfPossible(oldname, newname string) error {
	if _, err := fsys.Fs.Stat(newname); err == nil {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: os.ErrExist}
	}

	data, err := afero.ReadFile(fsys.Fs, oldname)
	if err != nil {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: err}
	}

	info, err := fsys.Fs.Stat(oldname)
	if err != nil {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: err}
	}

	return afero.WriteFile(fsys.Fs, newname, data, info.Mode())
}

func (fsys *memMapFS) ReadlinkIfPossible(name string) (string, error) {
	target, ok := fsys.symlinks[name]
	if !ok {
		return "", &os.PathError{Op: "readlink", Path: name, Err: os.ErrInvalid}
	}

	return target, nil
}

func (fsys *memMapFS) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	if _, ok := fsys.symlinks[name]; ok {
		return symlinkFileInfo{name: filepath.Base(name)}, true, nil
	}

	info, err := fsys.Fs.Stat(name)

	return info, false, err
}

// Remove deletes the file or symlink at name. Symlinks live in a side table
// that the embedded afero.MemMapFs does not see, so they are handled here
// before delegating to the underlying filesystem.
func (fsys *memMapFS) Remove(name string) error {
	if _, ok := fsys.symlinks[name]; ok {
		delete(fsys.symlinks, name)
		return nil
	}

	return fsys.Fs.Remove(name)
}

// RemoveAll deletes path and any children it contains. Symlinks live in a
// side table that the embedded afero.MemMapFs does not see, so they are
// handled here before delegating to the underlying filesystem.
func (fsys *memMapFS) RemoveAll(path string) error {
	if _, ok := fsys.symlinks[path]; ok {
		delete(fsys.symlinks, path)
		return nil
	}

	return fsys.Fs.RemoveAll(path)
}

// symlinkFileInfo reports symlink metadata for links stored in memMapFS's side table.
type symlinkFileInfo struct {
	name string
}

func (info symlinkFileInfo) Name() string       { return info.name }
func (info symlinkFileInfo) Size() int64        { return 0 }
func (info symlinkFileInfo) Mode() os.FileMode  { return os.ModeSymlink | os.ModePerm }
func (info symlinkFileInfo) ModTime() time.Time { return time.Time{} }
func (info symlinkFileInfo) IsDir() bool        { return false }
func (info symlinkFileInfo) Sys() any           { return nil }

func (fsys *memMapFS) Lock(name string) (Unlocker, error) {
	l := fsys.getOrCreateLock(name)
	l.mu.Lock()

	return l, nil
}

func (fsys *memMapFS) TryLock(name string) (Unlocker, bool, error) {
	l := fsys.getOrCreateLock(name)

	if !l.mu.TryLock() {
		return nil, false, nil
	}

	return l, true, nil
}

func (fsys *memMapFS) LockContext(ctx context.Context, name string) (Unlocker, error) {
	ctx, cancel := context.WithTimeout(ctx, maxLockWait)
	defer cancel()

	l := fsys.getOrCreateLock(name)

	for {
		if l.mu.TryLock() {
			return l, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(memMapLockRetryDelay):
		}
	}
}

func (fsys *memMapFS) getOrCreateLock(name string) *memLock {
	fsys.locksMu.Lock()
	defer fsys.locksMu.Unlock()

	l, ok := fsys.locks[name]
	if !ok {
		l = &memLock{}
		fsys.locks[name] = l
	}

	return l
}

// memLock is an in-memory lock backed by a mutex.
type memLock struct {
	mu sync.Mutex
}

func (l *memLock) Unlock() error {
	l.mu.Unlock()
	return nil
}

// defaultZipDirMode is the pre-umask mode for directories the extractor creates itself.
const defaultZipDirMode os.FileMode = 0755

// maxSymlinkTargetSize bounds a symlink target read, far above any real path.
const maxSymlinkTargetSize = 4096

// ZipDecompressor handles zip archive extraction with configurable limits.
type ZipDecompressor struct {
	// FileSizeLimit limits total decompressed size in bytes. Zero means no limit.
	FileSizeLimit int64
	// FilesLimit limits the number of files. Zero means no limit.
	FilesLimit int
}

// ZipDecompressorOption is a functional option for configuring ZipDecompressor.
type ZipDecompressorOption func(*ZipDecompressor)

// WithFileSizeLimit sets the maximum total decompressed size in bytes.
// Zero means no limit.
func WithFileSizeLimit(limit int64) ZipDecompressorOption {
	return func(z *ZipDecompressor) {
		z.FileSizeLimit = limit
	}
}

// WithFilesLimit sets the maximum number of files that can be extracted.
// Zero means no limit.
func WithFilesLimit(limit int) ZipDecompressorOption {
	return func(z *ZipDecompressor) {
		z.FilesLimit = limit
	}
}

// NewZipDecompressor creates a new ZipDecompressor with the given options.
func NewZipDecompressor(opts ...ZipDecompressorOption) *ZipDecompressor {
	z := &ZipDecompressor{}
	for _, opt := range opts {
		opt(z)
	}

	return z
}

// Unzip extracts a zip archive from src to dst directory on the given filesystem.
// The umask parameter is applied to file permissions (use 0 to preserve original permissions).
func (z *ZipDecompressor) Unzip(l log.Logger, fsys FS, dst, src string, umask os.FileMode) error {
	file, err := fsys.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open zip archive %q: %w", src, err)
	}

	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			l.Warnf("Error closing zip archive %q: %v", src, closeErr)
		}
	}()

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat zip archive %q: %w", src, err)
	}

	size := fileInfo.Size()

	var readerAt io.ReaderAt
	if ra, ok := file.(io.ReaderAt); ok {
		readerAt = ra
	} else {
		data, err := io.ReadAll(file)
		if err != nil {
			return fmt.Errorf("failed to read zip archive %q: %w", src, err)
		}

		readerAt = bytes.NewReader(data)
		size = int64(len(data))
	}

	zipReader, err := zip.NewReader(readerAt, size)
	if err != nil {
		return fmt.Errorf("failed to read zip archive %q: %w", src, err)
	}

	if err := fsys.MkdirAll(dst, applyUmask(defaultZipDirMode, umask)); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", dst, err)
	}

	if z.FilesLimit > 0 && len(zipReader.File) > z.FilesLimit {
		return fmt.Errorf(
			"zip archive contains %d files, exceeds limit of %d",
			len(zipReader.File),
			z.FilesLimit,
		)
	}

	var totalSize int64

	for _, zipFile := range zipReader.File {
		if err := z.extractZipFile(l, fsys, dst, zipFile, umask, &totalSize); err != nil {
			return fmt.Errorf("failed to extract file %q: %w", zipFile.Name, err)
		}
	}

	return nil
}

// extractZipFile extracts a single file from a zip archive.
func (z *ZipDecompressor) extractZipFile(
	l log.Logger, fsys FS, dst string, zipFile *zip.File, umask os.FileMode, totalSize *int64,
) error {
	destPath, err := sanitizeZipPath(dst, zipFile.Name)
	if err != nil {
		return err
	}

	fileInfo := zipFile.FileInfo()

	if fileInfo.IsDir() {
		if err := fsys.MkdirAll(destPath, applyUmask(fileInfo.Mode(), umask)); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", destPath, err)
		}

		return nil
	}

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return z.extractSymlink(l, fsys, dst, destPath, zipFile, umask, totalSize)
	}

	return z.extractRegularFile(l, fsys, destPath, zipFile, umask, totalSize)
}

// extractRegularFile extracts a regular file from a zip file.
func (z *ZipDecompressor) extractRegularFile(
	l log.Logger,
	fsys FS,
	destPath string,
	zipFile *zip.File,
	umask os.FileMode,
	totalSize *int64,
) error {
	if err := fsys.MkdirAll(
		filepath.Dir(destPath),
		applyUmask(defaultZipDirMode, umask),
	); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(destPath), err)
	}

	rc, err := zipFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", zipFile.Name, err)
	}

	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", zipFile.Name, closeErr)
		}
	}()

	mode := applyUmask(zipFile.FileInfo().Mode(), umask)

	outFile, err := fsys.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", destPath, err)
	}

	var reader io.Reader = rc

	if z.FileSizeLimit > 0 {
		reader = &limitedReader{
			reader:    rc,
			remaining: z.FileSizeLimit - *totalSize,
			name:      zipFile.Name,
			size:      zipFile.UncompressedSize64,
			limit:     z.FileSizeLimit,
		}
	}

	written, err := io.Copy(outFile, reader)
	if err != nil {
		if closeErr := outFile.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", destPath, closeErr)
		}

		if removeErr := fsys.Remove(destPath); removeErr != nil {
			l.Warnf("Error removing partial file %q: %v", destPath, removeErr)
		}

		return fmt.Errorf("failed to copy file %q: %w", zipFile.Name, err)
	}

	if err := outFile.Close(); err != nil {
		l.Warnf("Error closing file %q: %v", destPath, err)
	}

	// Update total size for limit tracking
	if z.FileSizeLimit > 0 {
		*totalSize += written
	}

	return nil
}

// FileInfoDirEntry wraps [os.FileInfo] to implement [fs.DirEntry].
// Adapted from spf13/afero#571; replace with afero equivalent once merged.
type FileInfoDirEntry struct {
	FileInfo os.FileInfo
}

func (d FileInfoDirEntry) Name() string               { return d.FileInfo.Name() }
func (d FileInfoDirEntry) IsDir() bool                { return d.FileInfo.IsDir() }
func (d FileInfoDirEntry) Type() fs.FileMode          { return d.FileInfo.Mode().Type() }
func (d FileInfoDirEntry) Info() (fs.FileInfo, error) { return d.FileInfo, nil }

// limitedReader wraps a reader and enforces a size limit.
type limitedReader struct {
	reader    io.Reader
	name      string
	remaining int64
	size      uint64
	limit     int64
}

func (r *limitedReader) Read(p []byte) (int, error) {
	if r.remaining > 0 {
		if int64(len(p)) > r.remaining {
			p = p[:r.remaining]
		}

		n, err := r.reader.Read(p)
		r.remaining -= int64(n)

		return n, err
	}

	var probe [1]byte

	n, err := r.reader.Read(probe[:])
	if n > 0 {
		return 0, ZipDecompressedSizeLimitError{Name: r.name, Size: r.size, Limit: r.limit}
	}

	if err == nil {
		return 0, io.EOF
	}

	return 0, err
}

// lstatIfPossible calls Lstat if the filesystem supports it, otherwise Stat.
func lstatIfPossible(fsys FS, path string) (os.FileInfo, error) {
	if lstater, ok := fsys.(afero.Lstater); ok {
		info, _, err := lstater.LstatIfPossible(path)
		return info, err
	}

	return fsys.Stat(path)
}

// symlinkWalkState tracks progress while resolving symlinks without using the OS filesystem.
type symlinkWalkState struct {
	path        string
	vol         string
	dest        string
	volLen      int
	start       int
	linksWalked int
}

func newSymlinkWalkState(path string) *symlinkWalkState {
	volLen := len(filepath.VolumeName(path))
	if volLen < len(path) && os.IsPathSeparator(path[volLen]) {
		volLen++
	}

	vol := path[:volLen]

	return &symlinkWalkState{
		path:   path,
		vol:    vol,
		dest:   vol,
		volLen: volLen,
		start:  volLen,
	}
}

func walkSymlinks(fsys FS, path string) (string, error) {
	state := newSymlinkWalkState(path)

	for state.start < len(state.path) {
		part, end, ok := state.nextComponent()
		if !ok {
			break
		}

		keepWalking, err := state.processComponent(fsys, part, end)
		if err != nil {
			return "", err
		}

		if !keepWalking {
			break
		}
	}

	return filepath.Clean(state.dest), nil
}

func (state *symlinkWalkState) nextComponent() (string, int, bool) {
	start := state.start
	for start < len(state.path) && os.IsPathSeparator(state.path[start]) {
		start++
	}

	end := start
	for end < len(state.path) && !os.IsPathSeparator(state.path[end]) {
		end++
	}

	if end == start {
		return "", end, false
	}

	return state.path[start:end], end, true
}

func (state *symlinkWalkState) processComponent(fsys FS, part string, end int) (bool, error) {
	isWindowsDot := runtime.GOOS == "windows" &&
		state.path[len(filepath.VolumeName(state.path)):] == "."
	if part == "." && !isWindowsDot {
		state.start = end

		return true, nil
	}

	if part == ".." {
		state.dest = walkSymlinksParent(state.dest, state.volLen, string(os.PathSeparator))
		state.start = end

		return true, nil
	}

	state.appendPart(part)

	info, err := Lstat(fsys, state.dest)
	if err != nil {
		return false, err
	}

	if info.Mode()&fs.ModeSymlink == 0 {
		return state.processRegularComponent(info, end)
	}

	return state.processSymlinkComponent(fsys, end, isWindowsDot)
}

func (state *symlinkWalkState) appendPart(part string) {
	if len(state.dest) > len(filepath.VolumeName(state.dest)) &&
		!os.IsPathSeparator(state.dest[len(state.dest)-1]) {
		state.dest += string(os.PathSeparator)
	}

	state.dest += part
}

func (state *symlinkWalkState) processRegularComponent(info os.FileInfo, end int) (bool, error) {
	if !info.Mode().IsDir() && end < len(state.path) {
		return false, syscall.ENOTDIR
	}

	state.start = end

	return true, nil
}

func (state *symlinkWalkState) processSymlinkComponent(
	fsys FS,
	end int,
	isWindowsDot bool,
) (bool, error) {
	state.linksWalked++
	if state.linksWalked > maxSymlinkEvaluations {
		return false, errors.New("EvalSymlinks: too many links")
	}

	link, err := Readlink(fsys, state.dest)
	if err != nil {
		return false, err
	}

	if isWindowsDot && !filepath.IsAbs(link) {
		return false, nil
	}

	state.path = link + state.path[end:]
	state.applyLink(link)

	return true, nil
}

func (state *symlinkWalkState) applyLink(link string) {
	linkVolLen := len(filepath.VolumeName(link))
	switch {
	case linkVolLen > 0:
		state.applyVolumeLink(link, linkVolLen)
	case len(link) > 0 && os.IsPathSeparator(link[0]):
		state.dest = link[:1]
		state.start = 1
		state.vol = link[:1]
		state.volLen = 1
	default:
		state.dest = walkSymlinksLinkParent(state.dest, state.vol, state.volLen)
		state.start = 0
	}
}

func (state *symlinkWalkState) applyVolumeLink(link string, linkVolLen int) {
	if linkVolLen < len(link) && os.IsPathSeparator(link[linkVolLen]) {
		linkVolLen++
	}

	state.vol = link[:linkVolLen]
	state.dest = state.vol
	state.start = len(state.vol)
	state.volLen = linkVolLen
}

func walkSymlinksParent(dest string, volLen int, pathSeparator string) string {
	var idx int
	for idx = len(dest) - 1; idx >= volLen; idx-- {
		if os.IsPathSeparator(dest[idx]) {
			break
		}
	}

	if idx < volLen || dest[idx+1:] == ".." {
		if len(dest) > volLen {
			dest += pathSeparator
		}

		return dest + ".."
	}

	return dest[:idx]
}

func walkSymlinksLinkParent(dest string, vol string, volLen int) string {
	var idx int
	for idx = len(dest) - 1; idx >= volLen; idx-- {
		if os.IsPathSeparator(dest[idx]) {
			break
		}
	}

	if idx < volLen {
		return vol
	}

	return dest[:idx]
}

// walkDir recursively descends path, calling walkDirFn.
// Adapted from https://go.dev/src/path/filepath/path.go
func walkDir(fsys FS, path string, d fs.DirEntry, walkDirFn fs.WalkDirFunc) error {
	if err := walkDirFn(path, d, nil); err != nil || !d.IsDir() {
		if errors.Is(err, filepath.SkipDir) && d.IsDir() {
			err = nil
		}

		return err
	}

	entries, err := ReadDir(fsys, path)
	if err != nil {
		err = walkDirFn(path, d, err)
		if err != nil {
			if errors.Is(err, filepath.SkipDir) && d.IsDir() {
				err = nil
			}

			return err
		}
	}

	for _, entry := range entries {
		name := filepath.Join(path, entry.Name())
		if err := walkDir(fsys, name, entry, walkDirFn); err != nil {
			if errors.Is(err, filepath.SkipDir) {
				break
			}

			return err
		}
	}

	return nil
}

// ReadDir reads the directory named by dirname and returns a sorted list of
// directory entries, as [fs.ReadDir] does. It prefers the [fs.ReadDirFile] fast
// path when the backing file supports it, and otherwise falls back to Readdir
// wrapped in [FileInfoDirEntry] so backings that only expose the legacy [os.File]
// API still work.
func ReadDir(fsys FS, dirname string) (_ []fs.DirEntry, retErr error) {
	f, err := fsys.Open(dirname)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	if rdf, ok := f.(fs.ReadDirFile); ok {
		entries, err := rdf.ReadDir(-1)
		if err != nil {
			return nil, err
		}

		slices.SortFunc(
			entries,
			func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) },
		)

		return entries, nil
	}

	infos, err := f.Readdir(-1)
	if err != nil {
		return nil, err
	}

	entries := make([]fs.DirEntry, len(infos))

	for i, info := range infos {
		entries[i] = FileInfoDirEntry{FileInfo: info}
	}

	slices.SortFunc(
		entries,
		func(a, b fs.DirEntry) int { return strings.Compare(a.Name(), b.Name()) },
	)

	return entries, nil
}

// ListFilesWithSuffixes returns the paths of the files directly under dir whose
// names end in any of the given suffixes, in the order [ReadDir] yields
// them. Subdirectories are skipped, and each match is joined to dir.
func ListFilesWithSuffixes(fsys FS, dir string, suffixes ...string) ([]string, error) {
	entries, err := ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		if !slices.ContainsFunc(suffixes, func(suffix string) bool {
			return strings.HasSuffix(name, suffix)
		}) {
			continue
		}

		files = append(files, filepath.Join(dir, name))
	}

	return files, nil
}

// containsDotDot checks if a path contains ".." as a path component.
// This is more precise than strings.Contains(name, "..") which would
// reject legitimate files like "file..txt".
func containsDotDot(v string) bool {
	if !strings.Contains(v, "..") {
		return false
	}

	return slices.Contains(strings.FieldsFunc(v, func(r rune) bool {
		return r == '/' || r == '\\'
	}), "..")
}

// sanitizeZipPath validates and sanitizes a zip entry path to prevent ZipSlip attacks.
func sanitizeZipPath(dst, name string) (string, error) {
	if containsDotDot(name) {
		return "", fmt.Errorf("illegal file path in zip: %s", name)
	}

	destPath := filepath.Join(dst, filepath.Clean(name))

	if !strings.HasPrefix(destPath, filepath.Clean(dst)+string(os.PathSeparator)) {
		return "", fmt.Errorf("illegal destination path in zip: %s", destPath)
	}

	return destPath, nil
}

// ValidateSymlinkTarget reports whether a symbolic link whose path is linkPath
// and whose stored target is target names a path inside dst. Absolute targets
// and dot-dot targets that climb above dst are rejected, so a symlink from an
// untrusted source (zip archives, fetched tarballs, git trees) cannot name a
// path outside the destination directory.
//
// Only the target the link stores is examined, which is all there is to go on
// when the link is being recorded or recreated rather than followed. When the
// target may itself be a symlink the same untrusted source controls, this is
// not sufficient on its own: the chain can leave dst through a link stored
// elsewhere. Callers that follow such a link must also check where it lands,
// with [ValidateResolvedSymlinkTarget].
func ValidateSymlinkTarget(dst, linkPath, target string) error {
	// Resolve the target relative to the link's directory
	absTarget := target
	if !filepath.IsAbs(target) {
		absTarget = filepath.Join(filepath.Dir(linkPath), target)
	}

	absTarget = filepath.Clean(absTarget)
	cleanDst := filepath.Clean(dst)

	// Ensure it stays within dst
	if !strings.HasPrefix(absTarget, cleanDst+string(os.PathSeparator)) && absTarget != cleanDst {
		return fmt.Errorf("%w: %s -> %s", ErrSymlinkEscapes, linkPath, target)
	}

	return nil
}

// ValidateResolvedSymlinkTarget reports whether the link at linkPath still
// lands inside root once its whole chain is followed. Use it before
// dereferencing a link from an untrusted source: [ValidateSymlinkTarget]
// examines only the target a link stores, so a chain that leaves root through
// a link stored somewhere else passes it.
//
// root is resolved as well, so a link is not reported as escaping merely
// because an ancestor of root is itself a symlink, as /var is on macOS.
//
// A chain that cannot be resolved at all, because it dangles, returns the
// resolution error rather than an escape. Callers that need to tell a hostile
// link from a broken one should check the stored target with
// [ValidateSymlinkTarget] first, which classifies a dangling link by the path
// it names.
func ValidateResolvedSymlinkTarget(fsys FS, root, linkPath string) error {
	resolved, err := EvalSymlinks(fsys, linkPath)
	if err != nil {
		return err
	}

	resolvedRoot, err := EvalSymlinks(fsys, root)
	if err != nil {
		return err
	}

	return ValidateSymlinkTarget(resolvedRoot, linkPath, resolved)
}

// extractSymlink extracts a symlink from a zip file.
func (z *ZipDecompressor) extractSymlink(
	l log.Logger,
	fsys FS,
	dst, destPath string,
	zipFile *zip.File,
	umask os.FileMode,
	totalSize *int64,
) error {
	if zipFile.UncompressedSize64 > maxSymlinkTargetSize {
		return fmt.Errorf("symlink %q target exceeds %d bytes", zipFile.Name, maxSymlinkTargetSize)
	}

	rc, err := zipFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", zipFile.Name, err)
	}

	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", zipFile.Name, closeErr)
		}
	}()

	targetBytes, err := io.ReadAll(io.LimitReader(rc, maxSymlinkTargetSize+1))
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", zipFile.Name, err)
	}

	if len(targetBytes) > maxSymlinkTargetSize {
		return fmt.Errorf("symlink %q target exceeds %d bytes", zipFile.Name, maxSymlinkTargetSize)
	}

	if z.FileSizeLimit > 0 {
		if *totalSize+int64(len(targetBytes)) > z.FileSizeLimit {
			return ZipDecompressedSizeLimitError{
				Name:  zipFile.Name,
				Size:  uint64(len(targetBytes)),
				Limit: z.FileSizeLimit,
			}
		}

		*totalSize += int64(len(targetBytes))
	}

	target := string(targetBytes)

	// Validate symlink target doesn't escape destination
	if err := ValidateSymlinkTarget(dst, destPath, target); err != nil {
		return err
	}

	if err := fsys.MkdirAll(
		filepath.Dir(destPath),
		applyUmask(defaultZipDirMode, umask),
	); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(destPath), err)
	}

	return Symlink(fsys, target, destPath)
}

// applyUmask applies a umask to a file mode.
func applyUmask(mode, umask os.FileMode) os.FileMode {
	return mode &^ umask
}
