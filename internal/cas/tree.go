package cas

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// unixPermMask isolates the user/group/other rwx bits from a git tree mode.
const unixPermMask = os.FileMode(0o777)

// defaultMaxTreeDepth stops a descent that the repository being materialized
// would otherwise decide the length of.
const defaultMaxTreeDepth = 64

// Git stores the entry type in the high bits of a six-digit octal mode;
// gitTypeMask isolates them so a symlink blob (120000) can be distinguished
// from a regular blob (100644 / 100755) at materialization time.
const (
	gitTypeMask    = uint64(0o170000)
	gitTypeSymlink = uint64(0o120000)
)

// LinkTreeOption configures a LinkTree call.
type LinkTreeOption func(*linkTreeOpts)

type linkTreeOpts struct {
	maxDepth  int
	forceCopy bool
	fsWorkers int
}

// WithForceCopy makes LinkTree copy blobs from the CAS store into the target
// directory instead of hardlinking them. The destination tree becomes safe to
// mutate without affecting the shared store, at the cost of extra I/O.
func WithForceCopy() LinkTreeOption {
	return func(o *linkTreeOpts) { o.forceCopy = true }
}

// WithMaxTreeDepth sets how deep a tree is followed before materialization
// gives up, in place of [defaultMaxTreeDepth].
func WithMaxTreeDepth(depth int) LinkTreeOption {
	return func(o *linkTreeOpts) { o.maxDepth = depth }
}

// maxTreeDepth returns the nesting bound to enforce, falling back to
// [defaultMaxTreeDepth] when the caller leaves it unset.
func (o *linkTreeOpts) maxTreeDepth() int {
	if o.maxDepth > 0 {
		return o.maxDepth
	}

	return defaultMaxTreeDepth
}

// treeEntryKind names what a git tree entry becomes on disk.
type treeEntryKind uint8

const (
	entryLink treeEntryKind = iota
	entrySymlink
	entrySubtree
	entrySubmodule
)

// treeWork is one entry waiting to be materialized, paired with the nesting
// depth of the tree that listed it.
type treeWork struct {
	entry git.TreeEntry
	path  string
	kind  treeEntryKind
	depth int
}

// LinkTree writes the tree to a target directory.
// blobStore is used to resolve blob entries, treeStore is used to resolve subtree entries.
func LinkTree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
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

	// Probed once here rather than per subtree to avoid paying the
	// probe cost on every subtree.
	o.fsWorkers = vfs.FSWorkersFor(v.FS, targetDir)

	linker := &treeLinker{
		blobContent: NewContent(blobStore),
		treeContent: NewContent(treeStore),
		treeStore:   treeStore,
		rootDir:     targetDir,
		maxDepth:    o.maxTreeDepth(),
	}

	if o.forceCopy {
		linker.linkOpts = append(linker.linkOpts, WithLinkForceCopy())
	}

	return linkTree(ctx, l, v, linker, t, targetDir, o.fsWorkers)
}

// linkTree materializes t and everything nested below it, one level of the
// tree at a time.
func linkTree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	linker *treeLinker,
	t *git.Tree,
	targetDir string,
	fsWorkers int,
) error {
	level, err := planTree(v, t, targetDir, 0)
	if err != nil {
		return err
	}

	for len(level) > 0 {
		opened := make([][]treeWork, len(level))

		g, gCtx := errgroup.WithContext(ctx)

		g.SetLimit(fsWorkers)

		for i := range level {
			g.Go(func() error {
				children, err := linker.materialize(gCtx, l, v, &level[i])
				if err != nil {
					return err
				}

				opened[i] = children

				return nil
			})
		}

		if err := g.Wait(); err != nil {
			return err
		}

		level = slices.Concat(opened...)
	}

	return nil
}

// planTree creates the directories t's entries need and returns the work each
// entry represents at t's own nesting depth.
func planTree(v *venv.Venv, t *git.Tree, targetDir string, depth int) ([]treeWork, error) {
	dirsToCreate := make(map[string]struct{}, len(t.Entries()))
	work := make([]treeWork, 0, len(t.Entries()))

	for _, entry := range t.Entries() {
		entryPath := filepath.Join(targetDir, entry.Path)
		dirPath := filepath.Dir(entryPath)

		dirsToCreate[dirPath] = struct{}{}

		// If the parent directory is in dirsToCreate,
		// we can remove it, since it will be created
		// when creating the subtree anyways.
		delete(dirsToCreate, filepath.Dir(dirPath))

		kind, ok := treeEntryKindOf(entry)
		if !ok {
			continue
		}

		work = append(work, treeWork{
			entry: entry,
			path:  entryPath,
			kind:  kind,
			depth: depth,
		})
	}

	for dirPath := range dirsToCreate {
		if err := v.FS.MkdirAll(dirPath, DefaultDirPerms); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dirPath, err)
		}
	}

	return work, nil
}

// treeEntryKindOf reports how to materialize entry, and false for an entry
// type that materialization skips.
func treeEntryKindOf(entry git.TreeEntry) (treeEntryKind, bool) {
	switch entry.Type {
	case git.EntryTypeBlob:
		// Git encodes a symlink as a blob whose body is the link target, so
		// the mode is the only thing separating it from a regular file.
		if gitEntryIsSymlink(entry.Mode) {
			return entrySymlink, true
		}

		return entryLink, true
	case git.EntryTypeTree:
		return entrySubtree, true
	case git.EntryTypeCommit:
		return entrySubmodule, true
	}

	return entryLink, false
}

// treeLinker is the state one [LinkTree] call shares across every entry it
// materializes.
type treeLinker struct {
	blobContent *Content
	treeContent *Content
	treeStore   *Store
	rootDir     string
	linkOpts    []LinkOption
	maxDepth    int
}

// materialize writes work to disk. For a subtree or submodule it opens the
// tree the entry stands for and returns the work its own entries represent.
func (tl *treeLinker) materialize(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	work *treeWork,
) ([]treeWork, error) {
	switch work.kind {
	case entryLink:
		return nil, tl.link(ctx, l, v, work)
	case entrySymlink:
		return nil, tl.symlink(v, work)
	case entrySubtree:
		return tl.subtree(v, work)
	case entrySubmodule:
		return tl.submodule(v, work)
	}

	return nil, nil
}

func (tl *treeLinker) link(ctx context.Context, l log.Logger, v *venv.Venv, work *treeWork) error {
	err := tl.blobContent.Link(
		ctx,
		l,
		v,
		work.entry.Hash,
		work.path,
		gitFilePerm(work.entry.Mode),
		tl.linkOpts...)
	if err != nil {
		return fmt.Errorf("link blob %s: %w", work.path, err)
	}

	return nil
}

func (tl *treeLinker) symlink(v *venv.Venv, work *treeWork) error {
	target, err := tl.blobContent.Read(v, work.entry.Hash)
	if err != nil {
		return fmt.Errorf("read symlink blob %s: %w", work.entry.Hash, err)
	}

	if err := vfs.ValidateSymlinkTarget(tl.rootDir, work.path, string(target)); err != nil {
		return err
	}

	if err := v.FS.RemoveAll(work.path); err != nil {
		return fmt.Errorf("clear existing entry before symlink %s: %w", work.path, err)
	}

	if err := vfs.Symlink(v.FS, string(target), work.path); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", work.path, string(target), err)
	}

	return nil
}

func (tl *treeLinker) subtree(v *venv.Venv, work *treeWork) ([]treeWork, error) {
	depth := work.depth + 1
	if depth > tl.maxDepth {
		return nil, &TreeDepthExceededError{MaxDepth: tl.maxDepth, Path: work.path}
	}

	treeData, err := tl.treeContent.Read(v, work.entry.Hash)
	if err != nil {
		return nil, fmt.Errorf("read tree %s: %w", work.entry.Hash, err)
	}

	subTree, err := git.ParseTree(treeData, work.path)
	if err != nil {
		return nil, fmt.Errorf("parse tree %s: %w", work.entry.Hash, err)
	}

	children, err := planTree(v, subTree, work.path, depth)
	if err != nil {
		return nil, fmt.Errorf("link subtree %s: %w", work.path, err)
	}

	return children, nil
}

func (tl *treeLinker) submodule(v *venv.Venv, work *treeWork) ([]treeWork, error) {
	depth := work.depth + 1
	if depth > tl.maxDepth {
		return nil, &TreeDepthExceededError{MaxDepth: tl.maxDepth, Path: work.path}
	}

	// A gitlink stands in for the submodule's whole working tree, which
	// ingestion stored keyed by the pinned commit hash. The directory is
	// created up front: a gitlink with no stored tree had no .gitmodules
	// entry to fetch it by, and `git clone` leaves an empty directory there
	// too.
	//
	// Which is also why a submodule tree deleted from the store reads as that
	// same skip and leaves an empty directory rather than a
	// [MissingObjectError] that repair could act on. Telling the two apart
	// needs the .gitmodules blob parsed here, at materialization time.
	if err := v.FS.MkdirAll(work.path, DefaultDirPerms); err != nil {
		return nil, fmt.Errorf("mkdir submodule %s: %w", work.path, err)
	}

	if tl.treeStore.NeedsWrite(v, work.entry.Hash) {
		return nil, nil
	}

	treeData, err := tl.treeContent.Read(v, work.entry.Hash)
	if err != nil {
		return nil, fmt.Errorf("read submodule tree %s: %w", work.entry.Hash, err)
	}

	subTree, err := git.ParseTree(treeData, work.path)
	if err != nil {
		return nil, fmt.Errorf("parse submodule tree %s: %w", work.entry.Hash, err)
	}

	children, err := planTree(v, subTree, work.path, depth)
	if err != nil {
		return nil, fmt.Errorf("link submodule %s: %w", work.path, err)
	}

	return children, nil
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

// gitEntryIsSymlink reports whether mode encodes the git symlink type
// (120000). The high bits of a six-digit octal mode carry the entry type;
// permission-only inspection cannot distinguish a symlink blob from a regular
// blob.
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
