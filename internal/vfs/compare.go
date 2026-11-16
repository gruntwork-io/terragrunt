package vfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// compareBlock is the size of the blocks in which [DirsDiffer] compares the contents of two files.
const compareBlock = 64 * 1024

// compareBuffersPool holds the buffers of finished [DirsDiffer] calls for later ones to read into.
var compareBuffersPool = sync.Pool{
	New: func() any {
		return &compareBuffers{
			from: make([]byte, compareBlock),
			to:   make([]byte, compareBlock),
		}
	},
}

// DirsDifferOption configures a [DirsDiffer] call.
type DirsDifferOption func(*dirsDifferConfig)

// WithIgnore leaves the files that ignore reports true for out of a [DirsDiffer] comparison. Given more than once,
// it leaves out the files that any of the functions reports true for.
func WithIgnore(ignore func(fs.DirEntry) bool) DirsDifferOption {
	return func(c *dirsDifferConfig) {
		c.ignore = append(c.ignore, ignore)
	}
}

// DirsDiffer reports whether the directories fromDir and toDir hold different files, by relative path, permissions
// or content.
//
// File contents are read only when both directories hold the same paths with the same sizes and permissions, and
// only up to the first difference.
func DirsDiffer(fsys FS, fromDir, toDir string, opts ...DirsDifferOption) (bool, error) {
	var cfg dirsDifferConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	fromFiles, err := listDirFiles(fsys, fromDir, cfg.ignore)
	if err != nil {
		return false, err
	}

	toFiles, err := listDirFiles(fsys, toDir, cfg.ignore)
	if err != nil {
		return false, err
	}

	if !slices.Equal(fromFiles, toFiles) {
		return true, nil
	}

	if len(fromFiles) == 0 {
		return false, nil
	}

	bufs, ok := compareBuffersPool.Get().(*compareBuffers)
	if !ok {
		panic("vfs: compareBuffersPool holds a value that is not *compareBuffers")
	}

	defer compareBuffersPool.Put(bufs)

	for _, file := range fromFiles {
		differ, err := fileContentsDiffer(
			fsys,
			filepath.Join(fromDir, file.relPath),
			filepath.Join(toDir, file.relPath),
			bufs.from,
			bufs.to,
		)
		if err != nil {
			return false, err
		}

		if differ {
			return true, nil
		}
	}

	return false, nil
}

// dirsDifferConfig is what the options of a [DirsDiffer] call set.
type dirsDifferConfig struct {
	ignore []func(fs.DirEntry) bool
}

// compareBuffers is the pair of equally long buffers the two files of a comparison are read into.
type compareBuffers struct {
	from []byte
	to   []byte
}

// dirFile is a file below a directory being compared with another.
type dirFile struct {
	relPath string
	size    int64
	perm    fs.FileMode
}

// listDirFiles returns the files below rootDir that none of ignore reports true for, sorted by relative path.
func listDirFiles(fsys FS, rootDir string, ignore []func(fs.DirEntry) bool) ([]dirFile, error) {
	var files []dirFile

	err := WalkDir(fsys, rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if slices.ContainsFunc(ignore, func(ignored func(fs.DirEntry) bool) bool { return ignored(d) }) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// The comparison reads through a symbolic link, so the size and permissions have to be those of its target.
		if !info.Mode().IsRegular() {
			info, err = fsys.Stat(path)
			if err != nil {
				return err
			}
		}

		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return fmt.Errorf("could not compute relative path for %s: %w", path, err)
		}

		files = append(files, dirFile{relPath: relPath, size: info.Size(), perm: info.Mode().Perm()})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	slices.SortFunc(files, func(a, b dirFile) int {
		return strings.Compare(a.relPath, b.relPath)
	})

	return files, nil
}

// fileContentsDiffer reports whether the files at fromPath and toPath hold different bytes, reading them a block at
// a time into fromBuf and toBuf, which are the same length.
func fileContentsDiffer(fsys FS, fromPath, toPath string, fromBuf, toBuf []byte) (_ bool, retErr error) {
	from, err := fsys.Open(fromPath)
	if err != nil {
		return false, fmt.Errorf("could not open file %s: %w", fromPath, err)
	}

	defer func() {
		if err := from.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("could not close file %s: %w", fromPath, err)
		}
	}()

	to, err := fsys.Open(toPath)
	if err != nil {
		return false, fmt.Errorf("could not open file %s: %w", toPath, err)
	}

	defer func() {
		if err := to.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("could not close file %s: %w", toPath, err)
		}
	}()

	for {
		fromN, err := readBlock(from, fromBuf)
		if err != nil {
			return false, fmt.Errorf("could not read file %s: %w", fromPath, err)
		}

		toN, err := readBlock(to, toBuf)
		if err != nil {
			return false, fmt.Errorf("could not read file %s: %w", toPath, err)
		}

		if !bytes.Equal(fromBuf[:fromN], toBuf[:toN]) {
			return true, nil
		}

		if fromN < len(fromBuf) {
			return false, nil
		}
	}
}

// readBlock fills buf from r, and returns fewer bytes than buf holds only at the end of r.
func readBlock(r io.Reader, buf []byte) (int, error) {
	n, err := io.ReadFull(r, buf)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return n, nil
	}

	return n, err
}
