package vfs

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// MirrorSkippedDirs are the directory names [MirrorToMem] does not descend
// into. Each holds generated or vendored content that a mirror does not need
// and that dwarfs the configuration it sits beside.
var MirrorSkippedDirs = []string{".git", ".terraform", ".terragrunt-cache"}

// ErrMirrorTooLarge is returned when a tree exceeds the limits given to
// [MirrorToMem]. A partial mirror would answer questions about a tree that is
// not the one on disk, so the copy fails rather than truncating.
var ErrMirrorTooLarge = errors.New("tree is too large to mirror in memory")

// ErrMirrorUnknownPath is returned when a [MirrorSelector] returns a path it
// was not handed.
var ErrMirrorUnknownPath = errors.New("mirror selector returned a path the walk never found")

// mirrorWorkers is how many files [MirrorToMem] has in flight at once, both
// while finding them and while copying them. The copy reads every file it
// finds, and on APFS that read traffic is fastest with about four in flight.
const mirrorWorkers = 4

// MirrorLimits bounds what [MirrorToMem] will copy. The file count is
// checked on everything the walk finds, before any selector runs, since every
// candidate is held until selection. The byte total is checked on the files
// actually copied, which are the only ones read.
type MirrorLimits struct {
	MaxFiles int
	MaxBytes int64
}

// MirrorSelector narrows which of the files found under the root are copied.
// It is handed every candidate's path, sorted and spelled as the walk found
// it, and returns the subset to copy. A path it was not handed is refused
// with [ErrMirrorUnknownPath].
//
// [github.com/gruntwork-io/terragrunt/internal/filter.Filters.EvaluateOnFiles]
// fits it, given the root as the working directory, so a mirror can be
// narrowed with the same queries --filter accepts.
type MirrorSelector func(files []string) ([]string, error)

// MirrorOption configures a [MirrorToMem] call.
type MirrorOption func(*mirrorConfig)

type mirrorConfig struct {
	selector MirrorSelector
}

// WithSelector makes [MirrorToMem] copy only the files sel returns. Without
// it every file the walk finds is copied.
func WithSelector(sel MirrorSelector) MirrorOption {
	return func(c *mirrorConfig) {
		c.selector = sel
	}
}

// MirrorToMem copies the tree rooted at dir from src into a fresh in-memory
// filesystem, keeping every path exactly where it was. Callers that resolve
// paths against the copy, or hand them back to a user, therefore see the paths
// the tree really has.
//
// Only files are copied. Writing one registers its parent directories, so the
// tree arrives with them, but a directory holding nothing does not survive.
//
// On a [NewOSFS] source the directories are read, and then the files copied,
// several at a time. The copy is complete when MirrorToMem returns, and an
// error from any file fails the whole copy.
func MirrorToMem(src FS, dir string, limits MirrorLimits, opts ...MirrorOption) (FS, error) {
	var cfg mirrorConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	found, err := findMirrorFiles(src, dir, limits.MaxFiles)
	if err != nil {
		return nil, err
	}

	selected, err := cfg.selectFiles(found)
	if err != nil {
		return nil, err
	}

	var bytes int64
	for _, f := range selected {
		bytes += f.info.Size()
	}

	if bytes > limits.MaxBytes {
		return nil, fmt.Errorf(
			"%w: more than %d bytes under %s",
			ErrMirrorTooLarge,
			limits.MaxBytes,
			dir,
		)
	}

	dst := NewMemMapFS()

	if err := copyMirrorFiles(src, dst, selected); err != nil {
		return nil, err
	}

	return dst, nil
}

// mirrorFile is a file the walk found, with the info the copy needs so that
// nothing is stat'd twice.
type mirrorFile struct {
	info fs.FileInfo
	path string
}

// findMirrorFiles returns every file under dir outside [MirrorSkippedDirs],
// sorted by path, refusing a tree holding more than maxFiles of them.
func findMirrorFiles(src FS, dir string, maxFiles int) ([]mirrorFile, error) {
	// On an OS source the walk calls back from several goroutines at once;
	// the list is the only thing they share.
	var (
		mu    sync.Mutex
		found []mirrorFile
	)

	err := WalkDirParallel(src, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if slices.Contains(MirrorSkippedDirs, d.Name()) {
				return fs.SkipDir
			}

			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		mu.Lock()
		defer mu.Unlock()

		if len(found) >= maxFiles {
			return fmt.Errorf("%w: more than %d files under %s", ErrMirrorTooLarge, maxFiles, dir)
		}

		found = append(found, mirrorFile{path: filepath.Clean(path), info: info})

		return nil
	}, WithWorkers(mirrorWorkers))
	if err != nil {
		return nil, err
	}

	slices.SortFunc(found, func(a, b mirrorFile) int {
		return strings.Compare(a.path, b.path)
	})

	return found, nil
}

// selectFiles hands the found paths to the selector, if any, and maps its
// answer back onto the files the walk recorded.
func (c mirrorConfig) selectFiles(found []mirrorFile) ([]mirrorFile, error) {
	if c.selector == nil {
		return found, nil
	}

	paths := make([]string, 0, len(found))
	byPath := make(map[string]mirrorFile, len(found))

	for _, f := range found {
		paths = append(paths, f.path)
		byPath[f.path] = f
	}

	chosen, err := c.selector(paths)
	if err != nil {
		return nil, err
	}

	selected := make([]mirrorFile, 0, len(chosen))

	for _, p := range chosen {
		f, ok := byPath[p]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrMirrorUnknownPath, p)
		}

		selected = append(selected, f)
	}

	return selected, nil
}

// copyMirrorFiles reads each file from src and writes it to dst at the same
// path, mirrorWorkers at a time. dst serializes its own writes.
func copyMirrorFiles(src, dst FS, files []mirrorFile) error {
	var g errgroup.Group

	g.SetLimit(mirrorWorkers)

	for _, f := range files {
		g.Go(func() error {
			contents, err := ReadFile(src, f.path)
			if err != nil {
				return err
			}

			return WriteFile(dst, f.path, contents, f.info.Mode())
		})
	}

	return g.Wait()
}
