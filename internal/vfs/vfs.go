// Package vfs provides a virtual filesystem abstraction for testing and production use.
// It wraps afero to provide a consistent interface for filesystem operations.
package vfs

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
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

// ErrNoHardLink is returned when a filesystem does not support hard links.
var ErrNoHardLink = errors.New("hard link not supported")

// ErrNoLock is returned when a filesystem does not support locking.
var ErrNoLock = errors.New("locking not supported")

// NewOSFS returns a filesystem backed by the real operating system filesystem.
func NewOSFS() FS {
	return &osFS{afero.NewOsFs()}
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
func FileExists(vfs FS, path string) (bool, error) {
	_, err := vfs.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	return false, err
}

// WriteFile writes data to a file on the given filesystem.
func WriteFile(fs FS, filename string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filename)
	if err := fs.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	return afero.WriteFile(fs, filename, data, perm)
}

// ReadFile reads the contents of a file from the given filesystem.
func ReadFile(fs FS, filename string) ([]byte, error) {
	return afero.ReadFile(fs, filename)
}

// Lstat returns file info for path without following the final symlink when the filesystem supports it.
func Lstat(fs FS, path string) (os.FileInfo, error) {
	return lstatIfPossible(fs, path)
}

// ParentPathHasSymlink reports whether rel cannot be safely traversed under rootDir.
// It returns true when rel is empty, ".", absolute, escapes rootDir with "..", or has a symlink in a parent component.
// The final path component is not checked, so callers can safely remove a leaf symlink.
func ParentPathHasSymlink(fsys FS, rootDir, rel string) (bool, error) {
	rel = filepath.Clean(rel)
	if rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
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

// MkdirTemp creates a temporary directory on the given filesystem.
func MkdirTemp(fs FS, dir, pattern string) (string, error) {
	return afero.TempDir(fs, dir, pattern)
}

// Link creates a hard link. It delegates to LinkIfPossible for filesystems
// that implement the HardLinker interface.
func Link(fs FS, oldname, newname string) error {
	linker, ok := fs.(HardLinker)
	if !ok {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: ErrNoHardLink}
	}

	return linker.LinkIfPossible(oldname, newname)
}

// Symlink creates a symbolic link. It uses afero's SymlinkIfPossible
// which is supported by OsFs and any FS implementing afero.Linker.
func Symlink(fs FS, oldname, newname string) error {
	linker, ok := fs.(afero.Linker)
	if !ok {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: afero.ErrNoSymlink}
	}

	return linker.SymlinkIfPossible(oldname, newname)
}

// Readlink reads the target of a symbolic link. It uses afero's
// ReadlinkIfPossible which is supported by OsFs and any FS implementing
// afero.Symlinker.
func Readlink(fs FS, name string) (string, error) {
	reader, ok := fs.(afero.Symlinker)
	if !ok {
		return "", &os.PathError{Op: "readlink", Path: name, Err: afero.ErrNoSymlink}
	}

	return reader.ReadlinkIfPossible(name)
}

// Lock acquires a blocking lock for the given name on the filesystem.
func Lock(fs FS, name string) (Unlocker, error) {
	locker, ok := fs.(Locker)
	if !ok {
		return nil, ErrNoLock
	}

	return locker.Lock(name)
}

// TryLock attempts a non-blocking lock for the given name on the filesystem.
func TryLock(fs FS, name string) (Unlocker, bool, error) {
	locker, ok := fs.(Locker)
	if !ok {
		return nil, false, ErrNoLock
	}

	return locker.TryLock(name)
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
// directory in the tree, including root. The fn callback receives an fs.DirEntry
// instead of os.FileInfo, which can be more efficient since it does not require
// a stat call for every visited file.
//
// All errors that arise visiting files and directories are filtered by fn:
// see the fs.WalkDirFunc documentation for details.
//
// The files are walked in lexical order, which makes the output deterministic
// but means that for very large directories WalkDir can be inefficient.
// WalkDir does not follow symbolic links.
//
// Adapted from spf13/afero#571 — replace with afero.WalkDir once merged.
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

// osFS wraps afero.OsFs with hard link support.
type osFS struct {
	afero.Fs
}

func (fs *osFS) LinkIfPossible(oldname, newname string) error {
	return os.Link(oldname, newname)
}

func (fs *osFS) SymlinkIfPossible(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

func (fs *osFS) ReadlinkIfPossible(name string) (string, error) {
	return os.Readlink(name)
}

func (fs *osFS) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	info, err := os.Lstat(name)

	return info, true, err
}

func (fs *osFS) Lock(name string) (Unlocker, error) {
	l := flock.New(name)
	if err := l.Lock(); err != nil {
		return nil, err
	}

	return l, nil
}

func (fs *osFS) TryLock(name string) (Unlocker, bool, error) {
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

// memMapFS wraps afero.MemMapFs with in-memory symlink support.
type memMapFS struct {
	afero.Fs
	symlinks map[string]string
	locks    map[string]*memLock
	locksMu  sync.Mutex
}

func (fs *memMapFS) SymlinkIfPossible(oldname, newname string) error {
	if _, exists := fs.symlinks[newname]; exists {
		return &os.LinkError{Op: "symlink", Old: oldname, New: newname, Err: os.ErrExist}
	}

	fs.symlinks[newname] = oldname

	return nil
}

func (fs *memMapFS) LinkIfPossible(oldname, newname string) error {
	if _, err := fs.Fs.Stat(newname); err == nil {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: os.ErrExist}
	}

	data, err := afero.ReadFile(fs.Fs, oldname)
	if err != nil {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: err}
	}

	info, err := fs.Fs.Stat(oldname)
	if err != nil {
		return &os.LinkError{Op: "link", Old: oldname, New: newname, Err: err}
	}

	return afero.WriteFile(fs.Fs, newname, data, info.Mode())
}

func (fs *memMapFS) ReadlinkIfPossible(name string) (string, error) {
	target, ok := fs.symlinks[name]
	if !ok {
		return "", &os.PathError{Op: "readlink", Path: name, Err: os.ErrInvalid}
	}

	return target, nil
}

func (fs *memMapFS) LstatIfPossible(name string) (os.FileInfo, bool, error) {
	if _, ok := fs.symlinks[name]; ok {
		return symlinkFileInfo{name: filepath.Base(name)}, true, nil
	}

	info, err := fs.Fs.Stat(name)

	return info, false, err
}

type symlinkFileInfo struct {
	name string
}

func (info symlinkFileInfo) Name() string       { return info.name }
func (info symlinkFileInfo) Size() int64        { return 0 }
func (info symlinkFileInfo) Mode() os.FileMode  { return os.ModeSymlink | os.ModePerm }
func (info symlinkFileInfo) ModTime() time.Time { return time.Time{} }
func (info symlinkFileInfo) IsDir() bool        { return false }
func (info symlinkFileInfo) Sys() any           { return nil }

func (fs *memMapFS) Lock(name string) (Unlocker, error) {
	l := fs.getOrCreateLock(name)
	l.mu.Lock()

	return l, nil
}

func (fs *memMapFS) TryLock(name string) (Unlocker, bool, error) {
	l := fs.getOrCreateLock(name)

	if !l.mu.TryLock() {
		return nil, false, nil
	}

	return l, true, nil
}

func (fs *memMapFS) getOrCreateLock(name string) *memLock {
	fs.locksMu.Lock()
	defer fs.locksMu.Unlock()

	l, ok := fs.locks[name]
	if !ok {
		l = &memLock{}
		fs.locks[name] = l
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
func (z *ZipDecompressor) Unzip(l log.Logger, fs FS, dst, src string, umask os.FileMode) error {
	file, err := fs.Open(src)
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

	if err := fs.MkdirAll(dst, os.ModePerm); err != nil {
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
		if err := z.extractZipFile(l, fs, dst, zipFile, umask, &totalSize); err != nil {
			return fmt.Errorf("failed to extract file %q: %w", zipFile.Name, err)
		}
	}

	return nil
}

// extractZipFile extracts a single file from a zip archive.
func (z *ZipDecompressor) extractZipFile(
	l log.Logger, fs FS, dst string, zipFile *zip.File, umask os.FileMode, totalSize *int64,
) error {
	destPath, err := sanitizeZipPath(dst, zipFile.Name)
	if err != nil {
		return err
	}

	fileInfo := zipFile.FileInfo()

	if fileInfo.IsDir() {
		if err := fs.MkdirAll(destPath, applyUmask(fileInfo.Mode(), umask)); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", destPath, err)
		}

		return nil
	}

	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return extractSymlink(l, fs, dst, destPath, zipFile)
	}

	return z.extractRegularFile(l, fs, destPath, zipFile, umask, totalSize)
}

// extractRegularFile extracts a regular file from a zip file.
func (z *ZipDecompressor) extractRegularFile(
	l log.Logger,
	fs FS,
	destPath string,
	zipFile *zip.File,
	umask os.FileMode,
	totalSize *int64,
) error {
	if err := fs.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
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

	outFile, err := fs.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("failed to create file %q: %w", destPath, err)
	}

	var reader io.Reader = rc

	if z.FileSizeLimit > 0 {
		reader = &limitedReader{
			reader:    rc,
			remaining: z.FileSizeLimit - *totalSize,
		}
	}

	written, err := io.Copy(outFile, reader)
	if err != nil {
		if closeErr := outFile.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", destPath, closeErr)
		}

		if removeErr := fs.Remove(destPath); removeErr != nil {
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

// FileInfoDirEntry wraps os.FileInfo to implement fs.DirEntry.
// Adapted from spf13/afero#571 — replace with afero equivalent once merged.
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
	remaining int64
}

func (r *limitedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, errors.New("decompressed size exceeds limit")
	}

	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}

	n, err := r.reader.Read(p)
	r.remaining -= int64(n)

	return n, err
}

// lstatIfPossible calls Lstat if the filesystem supports it, otherwise Stat.
func lstatIfPossible(fsys FS, path string) (os.FileInfo, error) {
	if lstater, ok := fsys.(afero.Lstater); ok {
		info, _, err := lstater.LstatIfPossible(path)
		return info, err
	}

	return fsys.Stat(path)
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

	entries, err := ReadDirEntries(fsys, path)
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

// ReadDirEntries reads the directory named by dirname and returns a sorted
// list of directory entries. It prefers the fs.ReadDirFile fast path when the
// backing file supports it, and otherwise falls back to Readdir wrapped in
// FileInfoDirEntry so backings that only expose the legacy os.File API still
// work.
func ReadDirEntries(fsys FS, dirname string) ([]fs.DirEntry, error) {
	f, err := fsys.Open(dirname)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = f.Close()
	}()

	if rdf, ok := f.(fs.ReadDirFile); ok {
		entries, err := rdf.ReadDir(-1)
		if err != nil {
			return nil, err
		}

		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

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

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	return entries, nil
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

// validateSymlinkTarget validates that a symlink target doesn't escape the destination directory.
func validateSymlinkTarget(dst, linkPath, target string) error {
	// Resolve the target relative to the link's directory
	absTarget := target
	if !filepath.IsAbs(target) {
		absTarget = filepath.Join(filepath.Dir(linkPath), target)
	}

	absTarget = filepath.Clean(absTarget)
	cleanDst := filepath.Clean(dst)

	// Ensure it stays within dst
	if !strings.HasPrefix(absTarget, cleanDst+string(os.PathSeparator)) && absTarget != cleanDst {
		return fmt.Errorf("symlink target escapes destination: %s -> %s", linkPath, target)
	}

	return nil
}

// extractSymlink extracts a symlink from a zip file.
func extractSymlink(l log.Logger, fs FS, dst, destPath string, zipFile *zip.File) error {
	rc, err := zipFile.Open()
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", zipFile.Name, err)
	}

	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			l.Warnf("Error closing file %q: %v", zipFile.Name, closeErr)
		}
	}()

	targetBytes, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", zipFile.Name, err)
	}

	target := string(targetBytes)

	// Validate symlink target doesn't escape destination
	if err := validateSymlinkTarget(dst, destPath, target); err != nil {
		return err
	}

	if err := fs.MkdirAll(filepath.Dir(destPath), os.ModePerm); err != nil {
		return fmt.Errorf("failed to create directory %q: %w", filepath.Dir(destPath), err)
	}

	return Symlink(fs, target, destPath)
}

// applyUmask applies a umask to a file mode.
func applyUmask(mode, umask os.FileMode) os.FileMode {
	return mode &^ umask
}
