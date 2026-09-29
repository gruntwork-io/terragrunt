package discovery

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// prunedWalk walks the part of the working directory that
// [filter.Classifier.WalkRoots] names.
//
// It starts at the literal prefix of each root, the directories before the
// first wildcard segment, and walks from there, skipping every directory no
// root admits. Each prefix directory is looked up in its parent's listing, so
// a name that differs from the filter's only in case does not match, even on a
// case-insensitive filesystem. A start above walkRoot moves down to it, and a
// start outside walkRoot is dropped. A directory symlink is followed only when
// followLinks is set.
//
// A directory that can hold a match only at itself, such as each match of
// "./apps/*", is not walked. The walk records it, and once the walk ends every
// recorded directory is listed concurrently.
type prunedWalk struct {
	// visit handles each entry the walk reaches.
	visit fs.WalkDirFunc
	// workingDir is the directory the roots are relative to.
	workingDir string
	// walkRoot is the directory at or below workingDir that the walk stays
	// within, spelled under workingDir.
	walkRoot string
	// numWorkers bounds the concurrent listings.
	numWorkers int
	// followLinks reports whether the walk descends into directory symlinks,
	// as discovery does under the symlinks experiment.
	followLinks bool
}

// run walks the directories roots match below the working directory on fsys.
func (w *prunedWalk) run(ctx context.Context, fsys vfs.FS, roots []glob.Root) error {
	w.workingDir = filepath.Clean(w.workingDir)
	w.walkRoot = filepath.Clean(w.walkRoot)

	info, err := vfs.Lstat(fsys, w.workingDir)
	if err != nil {
		return w.visit(w.workingDir, nil, err)
	}

	rootEntry := fs.FileInfoToDirEntry(info)

	if err := w.visit(w.workingDir, rootEntry, nil); err != nil || !rootEntry.IsDir() {
		if errors.Is(err, filepath.SkipDir) {
			return nil
		}

		return err
	}

	pruner := glob.NewPruner(roots)

	var starts []string

	for _, root := range roots {
		start, ok, err := w.literalPrefix(fsys, root.Dir)
		if err != nil {
			return err
		}

		if ok {
			starts = append(starts, start)
		}
	}

	var leaves []string

	visit := w.pruning(pruner, &leaves)

	for _, start := range outermostDirs(w.narrowToWalkRoot(starts)) {
		if err := w.walk(fsys, start, visit); err != nil {
			return err
		}
	}

	listings, err := w.listLeaves(ctx, fsys, leaves)
	if err != nil {
		return err
	}

	return w.visitLeaves(leaves, listings)
}

// literalPrefix joins the segments of dirPattern before its first wildcard onto
// the working directory, finding each in its parent's listing. It reports false
// when a segment is missing or names something the walk would not descend
// into.
func (w *prunedWalk) literalPrefix(fsys vfs.FS, dirPattern string) (string, bool, error) {
	dir := w.workingDir

	if dirPattern == "." {
		return dir, true, nil
	}

	for segment := range strings.SplitSeq(dirPattern, "/") {
		if strings.ContainsAny(segment, "*?") {
			break
		}

		entries, err := vfs.ReadDir(fsys, dir)
		if isMissing(err) {
			return "", false, nil
		}

		if err != nil {
			return "", false, err
		}

		i, found := slices.BinarySearchFunc(entries, segment, func(entry fs.DirEntry, name string) int {
			return strings.Compare(entry.Name(), name)
		})
		if !found {
			return "", false, nil
		}

		dir = filepath.Join(dir, segment)

		ok, err := w.descends(fsys, dir, entries[i])
		if err != nil || !ok {
			return "", false, err
		}
	}

	return dir, true, nil
}

// narrowToWalkRoot keeps each of starts at or below the walk root, moves each
// one above the walk root down to it, and drops the rest.
func (w *prunedWalk) narrowToWalkRoot(starts []string) []string {
	narrowed := make([]string, 0, len(starts))

	for _, start := range starts {
		switch {
		case withinDir(start, w.walkRoot):
			narrowed = append(narrowed, start)
		case withinDir(w.walkRoot, start):
			narrowed = append(narrowed, w.walkRoot)
		}
	}

	return narrowed
}

// descends reports whether the walk would descend into dir, listed as entry. A
// directory symlink counts only when followLinks is set. A directory goes
// through visit, which applies the ignorable and hidden directory rules and
// cancellation.
func (w *prunedWalk) descends(fsys vfs.FS, dir string, entry fs.DirEntry) (bool, error) {
	if entry.Type()&fs.ModeSymlink != 0 && w.followLinks {
		info, err := fsys.Stat(dir)
		if isMissing(err) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		entry = fs.FileInfoToDirEntry(info)
	}

	if !entry.IsDir() {
		return false, nil
	}

	if err := w.visit(dir, entry, nil); err != nil {
		if errors.Is(err, filepath.SkipDir) {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// pruning wraps visit so the walk skips every directory pruner does not
// admit, along with the files in it. A directory that can hold a match only at
// itself is appended to leaves and skipped too, so it is listed after the walk
// rather than during it.
func (w *prunedWalk) pruning(pruner *glob.Pruner, leaves *[]string) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return w.visit(path, d, err)
		}

		rel, relErr := filepath.Rel(w.workingDir, path)
		if relErr != nil {
			return relErr
		}

		rel = filepath.ToSlash(rel)

		if !pruner.Admits(rel) {
			return filepath.SkipDir
		}

		if err := w.visit(path, d, nil); err != nil {
			return err
		}

		if pruner.AdmitsBelow(rel) {
			return nil
		}

		*leaves = append(*leaves, path)

		return filepath.SkipDir
	}
}

// listLeaves lists every one of dirs concurrently. The result holds one
// listing per dir, empty for a dir that no longer exists.
func (w *prunedWalk) listLeaves(ctx context.Context, fsys vfs.FS, dirs []string) ([][]fs.DirEntry, error) {
	listings := make([][]fs.DirEntry, len(dirs))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(w.numWorkers)

	for i, dir := range dirs {
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}

			entries, err := vfs.ReadDir(fsys, dir)
			if isMissing(err) {
				return nil
			}

			if err != nil {
				return err
			}

			listings[i] = entries

			return nil
		})
	}

	return listings, g.Wait()
}

// visitLeaves hands visit every entry of each listing that is not a directory,
// as the walk does in a directory it enters. Visiting goes in dir and name
// order after every listing finishes, so the results and the first coexistence
// error do not depend on which listing returned first.
func (w *prunedWalk) visitLeaves(dirs []string, listings [][]fs.DirEntry) error {
	for i, dir := range dirs {
		for _, entry := range listings[i] {
			if entry.IsDir() {
				continue
			}

			if err := w.visit(filepath.Join(dir, entry.Name()), entry, nil); err != nil {
				return err
			}
		}
	}

	return nil
}

// walk walks root on fsys, descending into directory symlinks when
// followLinks is set.
func (w *prunedWalk) walk(fsys vfs.FS, root string, fn fs.WalkDirFunc) error {
	if w.followLinks {
		return vfs.WalkDirWithSymlinks(fsys, root, fn)
	}

	return vfs.WalkDir(fsys, root, fn)
}

// isMissing reports whether err says a path does not exist, including a path
// that runs through a file.
func isMissing(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)
}

// outermostDirs sorts and deduplicates dirs, dropping any directory below
// another one in the list.
func outermostDirs(dirs []string) []string {
	slices.Sort(dirs)

	outermost := make([]string, 0, len(dirs))

	for _, dir := range slices.Compact(dirs) {
		if slices.ContainsFunc(outermost, func(kept string) bool { return withinDir(dir, kept) }) {
			continue
		}

		outermost = append(outermost, dir)
	}

	return outermost
}

// withinDir reports whether path is dir or sits below it, comparing the
// spellings as text.
func withinDir(path, dir string) bool {
	prefix := strings.TrimSuffix(dir, string(filepath.Separator)) + string(filepath.Separator)

	return path == dir || strings.HasPrefix(path, prefix)
}
