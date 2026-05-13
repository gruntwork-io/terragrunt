package cas

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"golang.org/x/sync/errgroup"
)

// unixPermMask isolates the user/group/other rwx bits from a git tree mode.
const unixPermMask = os.FileMode(0o777)

// Git tree entry mode constants. Git stores the entry type in the high bits of
// a six-digit octal mode; gitTypeMask isolates them so a symlink blob (120000)
// can be distinguished from a regular blob (100644 / 100755) at materialization
// time.
const (
	gitTypeMask    = uint64(0o170000)
	gitTypeSymlink = uint64(0o120000)
)

// LinkTreeOption configures a LinkTree call.
type LinkTreeOption func(*linkTreeOpts)

type linkTreeOpts struct {
	forceCopy bool
}

// WithForceCopy makes LinkTree copy blobs from the CAS store into the target
// directory instead of hardlinking them. The destination tree becomes safe to
// mutate without affecting the shared store, at the cost of extra I/O.
func WithForceCopy() LinkTreeOption {
	return func(o *linkTreeOpts) { o.forceCopy = true }
}

// LinkTree writes the tree to a target directory.
// blobStore is used to resolve blob entries, treeStore is used to resolve subtree entries.
func LinkTree(
	ctx context.Context,
	blobStore *Store,
	treeStore *Store,
	t *git.Tree,
	targetDir string,
	opts ...LinkTreeOption,
) error {
	var o linkTreeOpts
	for _, opt := range opts {
		opt(&o)
	}

	return linkTree(ctx, blobStore, treeStore, t, targetDir, targetDir, &o)
}

// linkTree is the recursive implementation behind LinkTree. rootDir is the
// top-level target the caller asked to materialize and stays constant across
// subtree recursion; targetDir is the directory the current tree is being
// written into. Splitting them lets symlink validation reject targets that
// resolve outside the original tree even when the link sits in a subdirectory.
func linkTree(
	ctx context.Context,
	blobStore *Store,
	treeStore *Store,
	t *git.Tree,
	rootDir string,
	targetDir string,
	o *linkTreeOpts,
) error {
	blobContent := NewContent(blobStore)
	treeContent := NewContent(treeStore)

	var linkOpts []LinkOption
	if o.forceCopy {
		linkOpts = append(linkOpts, WithLinkForceCopy())
	}

	dirsToCreate := make(map[string]struct{}, len(t.Entries()))

	type workItem struct {
		itemType string
		entry    git.TreeEntry
		path     string
		dirPath  string
	}

	workItems := make([]workItem, 0, len(t.Entries()))

	for _, entry := range t.Entries() {
		entryPath := filepath.Join(targetDir, entry.Path)
		dirPath := filepath.Dir(entryPath)

		dirsToCreate[dirPath] = struct{}{}

		// If the parent directory is in dirsToCreate,
		// we can remove it, since it will be created
		// when creating the subtree anyways.
		parentDirPath := filepath.Dir(dirPath)
		delete(dirsToCreate, parentDirPath)

		// Create work items based on entry type. Git stores symlinks as blobs
		// whose content is the link target; the entry mode (120000) is the
		// only signal that distinguishes them from regular files, so dispatch
		// on the mode here instead of treating every blob as a file to copy.
		switch entry.Type {
		case "blob":
			itemType := "link"
			if gitEntryIsSymlink(entry.Mode) {
				itemType = "symlink"
			}

			workItems = append(workItems, workItem{
				itemType: itemType,
				entry:    entry,
				path:     entryPath,
				dirPath:  dirPath,
			})
		case "tree":
			workItems = append(workItems, workItem{
				itemType: "subtree",
				entry:    entry,
				path:     entryPath,
				dirPath:  dirPath,
			})
		}
	}

	fs := blobStore.FS()

	for dirPath := range dirsToCreate {
		if err := fs.MkdirAll(dirPath, DefaultDirPerms); err != nil {
			return fmt.Errorf("mkdir %s: %w", dirPath, err)
		}
	}

	// Use errgroup for concurrent processing
	g, ctx := errgroup.WithContext(ctx)

	// Use half the available CPUs (at least 1) to avoid saturating I/O during tree materialization.
	scalingFactor := 2
	maxWorkers := max(1, runtime.GOMAXPROCS(0)/scalingFactor)
	g.SetLimit(maxWorkers)

	// Process work items concurrently
	for _, work := range workItems {
		g.Go(func() error {
			switch work.itemType {
			case "link":
				err := blobContent.Link(ctx, work.entry.Hash, work.path, gitFilePerm(work.entry.Mode), linkOpts...)
				if err != nil {
					return fmt.Errorf("link blob %s: %w", work.path, err)
				}
			case "symlink":
				target, err := blobContent.Read(work.entry.Hash)
				if err != nil {
					return fmt.Errorf("read symlink blob %s: %w", work.entry.Hash, err)
				}

				if err := vfs.ValidateSymlinkTarget(rootDir, work.path, string(target)); err != nil {
					return err
				}

				if err := vfs.Symlink(fs, string(target), work.path); err != nil {
					return fmt.Errorf("symlink %s -> %s: %w", work.path, string(target), err)
				}
			case "subtree":
				treeData, err := treeContent.Read(work.entry.Hash)
				if err != nil {
					return fmt.Errorf("read tree %s: %w", work.entry.Hash, err)
				}

				subTree, err := git.ParseTree(treeData, work.path)
				if err != nil {
					return fmt.Errorf("parse tree %s: %w", work.entry.Hash, err)
				}

				err = linkTree(ctx, blobStore, treeStore, subTree, rootDir, work.path, o)
				if err != nil {
					return fmt.Errorf("link subtree %s: %w", work.path, err)
				}
			}

			return nil
		})
	}

	// Wait for all goroutines to complete and return first error if any
	return g.Wait()
}

// gitFilePerm extracts the unix permission bits from a git tree entry mode
// string. Git tree modes are six-digit octal: "100644" or "100755" for blobs.
// Returns RegularFilePerms when the mode is missing or unparsable so callers
// always have a sane default.
func gitFilePerm(mode string) os.FileMode {
	if mode == "" {
		return RegularFilePerms
	}

	n, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return RegularFilePerms
	}

	return os.FileMode(n) & unixPermMask
}

// gitEntryIsSymlink reports whether the given git tree entry mode is the
// symlink type (120000). Git tree modes encode the entry type in their high
// bits, so the permission-only view used by gitFilePerm cannot distinguish a
// symlink blob from a regular blob.
func gitEntryIsSymlink(mode string) bool {
	if mode == "" {
		return false
	}

	n, err := strconv.ParseUint(mode, 8, 32)
	if err != nil {
		return false
	}

	return n&gitTypeMask == gitTypeSymlink
}
