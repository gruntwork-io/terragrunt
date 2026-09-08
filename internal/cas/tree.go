package cas

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
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

// LinkFallback classifies why a tree could not be materialized in the mode it
// was asked for. It travels as the fallback attribute on the cas_link_tree
// span.
type LinkFallback string

const (
	// LinkFallbackNone reports that the requested mode served the whole tree.
	LinkFallbackNone LinkFallback = ""

	// LinkFallbackCloneUnsupported reports that the filesystem holding the
	// target has no copy-on-write clone, so the tree was hard linked or
	// copied instead.
	LinkFallbackCloneUnsupported LinkFallback = "clone_unsupported"

	// LinkFallbackHardlinkUnsupported reports that the target could not be
	// given a second name for every stored blob, because it sits on another
	// filesystem or a blob does not carry the permissions git recorded, so
	// the tree was copied instead.
	LinkFallbackHardlinkUnsupported LinkFallback = "hardlink_unsupported"
)

// linkFallbackByMode names the fallback a tree reports when the mode it was
// asked for did not serve every file in it. [LinkModeCopy] has no entry: it
// is the mode every other one degrades to.
var linkFallbackByMode = map[LinkMode]LinkFallback{
	LinkModeHardlink: LinkFallbackHardlinkUnsupported,
	LinkModeClone:    LinkFallbackCloneUnsupported,
}

// LinkTreeOption configures a LinkTree call.
type LinkTreeOption func(*linkTreeOpts)

type linkTreeOpts struct {
	maxDepth  int
	mode      LinkMode
	forceCopy bool
	fsWorkers int
}

// WithForceCopy tells LinkTree the target directory is going to be edited, so
// blobs must not be materialized as the CAS store's own files. The
// destination tree becomes safe to mutate. A tree asked for in
// [LinkModeHardlink] is cloned instead, so the extra I/O is a copy per file
// only where the filesystem has no copy-on-write clone.
func WithForceCopy() LinkTreeOption {
	return func(o *linkTreeOpts) { o.forceCopy = true }
}

// WithTreeLinkMode selects how blobs reach the target directory. Without it
// LinkTree uses [DefaultLinkMode]; the CAS entry points pass the mode their
// instance was built with.
func WithTreeLinkMode(mode LinkMode) LinkTreeOption {
	return func(o *linkTreeOpts) { o.mode = mode }
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
//
// The whole tree, subtrees included, is reported as one cas_link_tree span
// carrying the mode that served it and what it cost, so a tree of thousands
// of files stays one span rather than thousands.
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
	o := linkTreeOpts{mode: DefaultLinkMode}
	for _, opt := range opts {
		opt(&o)
	}

	// Probed once here rather than per subtree to avoid paying the
	// probe cost on every subtree.
	o.fsWorkers = vfs.FSWorkersFor(v.FS, targetDir)

	mode := resolveLinkMode(o.mode, o.forceCopy)

	linker := &treeLinker{
		blobContent: NewContent(blobStore),
		treeContent: NewContent(treeStore),
		treeStore:   treeStore,
		rootDir:     targetDir,
		maxDepth:    o.maxTreeDepth(),
		mode:        mode,
		forceCopy:   o.forceCopy,
	}

	return telemetry.TelemeterFromContext(ctx).Collect(ctx, nil, "cas_link_tree", map[string]any{
		"path": targetDir,
		"mode": mode.String(),
	}, func(childCtx context.Context, _ log.Logger) error {
		err := linkTree(l, v, linker, t, targetDir, o.fsWorkers)

		linker.report(childCtx)

		return err
	})
}

// linkTree materializes t and everything nested below it, one level of the
// tree at a time.
func linkTree(
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
		if idx := linker.probeIndex(level); idx >= 0 {
			if err := linker.link(l, v, &level[idx]); err != nil {
				return err
			}

			level = slices.Delete(level, idx, idx+1)
		}

		opened := make([][]treeWork, len(level))

		var g errgroup.Group

		g.SetLimit(fsWorkers)

		for i := range level {
			g.Go(func() error {
				children, err := linker.materialize(l, v, &level[i])
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
// materializes. Its counters are shared across the workers every level runs,
// so LinkTree can report the tree as a single span.
type treeLinker struct {
	blobContent      *Content
	treeContent      *Content
	treeStore        *Store
	rootDir          string
	maxDepth         int
	linked           atomic.Int64
	cloned           atomic.Int64
	copied           atomic.Int64
	bytesCopied      atomic.Int64
	mode             LinkMode
	cloneUnsupported atomic.Bool
	forceCopy        bool
}

// materialize writes work to disk. For a subtree or submodule it opens the
// tree the entry stands for and returns the work its own entries represent.
func (tl *treeLinker) materialize(l log.Logger, v *venv.Venv, work *treeWork) ([]treeWork, error) {
	switch work.kind {
	case entryLink:
		return nil, tl.link(l, v, work)
	case entrySymlink:
		return nil, tl.symlink(v, work)
	case entrySubtree:
		return tl.subtree(v, work)
	case entrySubmodule:
		return tl.submodule(v, work)
	}

	return nil, nil
}

// link materializes one blob entry and counts how it arrived.
func (tl *treeLinker) link(l log.Logger, v *venv.Venv, work *treeWork) error {
	outcome, err := tl.blobContent.Link(
		l,
		v,
		work.entry.Hash,
		work.path,
		gitFilePerm(work.entry.Mode),
		tl.linkOptions()...)
	if err != nil {
		return fmt.Errorf("link blob %s: %w", work.path, err)
	}

	if tl.mode == LinkModeClone && outcome.Mode != LinkModeClone {
		tl.cloneUnsupported.Store(true)
	}

	tl.record(outcome)

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

// probeIndex returns the blob in level to materialize ahead of the others, or
// -1 when nothing is to be learned from doing so. Only a clone has an answer
// worth settling: every other mode either serves every file or is already
// what a clone degrades to.
func (tl *treeLinker) probeIndex(level []treeWork) int {
	if tl.mode != LinkModeClone || tl.cloneUnsupported.Load() {
		return -1
	}

	return slices.IndexFunc(level, func(work treeWork) bool {
		return work.kind == entryLink
	})
}

// linkOptions returns the options one blob is materialized with.
//
// Once a clone has come back as something else, which is how a filesystem
// with no copy-on-write clone answers, the rest of the tree takes the same
// fallback without asking again. A single entry the filesystem cannot clone
// where others clone fine, such as one reached across a mount point, still
// falls back on its own inside [Content.Link].
func (tl *treeLinker) linkOptions() []LinkOption {
	opts := []LinkOption{WithFileLinkMode(tl.mode)}

	if tl.forceCopy {
		opts = append(opts, WithLinkForceCopy())
	}

	if tl.cloneUnsupported.Load() {
		opts = append(opts, WithoutCloneAttempt())
	}

	return opts
}

// record counts one materialized blob.
func (tl *treeLinker) record(outcome LinkOutcome) {
	switch outcome.Mode {
	case LinkModeHardlink:
		tl.linked.Add(1)
	case LinkModeClone:
		tl.cloned.Add(1)
	case LinkModeCopy:
		tl.copied.Add(1)
		tl.bytesCopied.Add(outcome.BytesCopied)
	}
}

// report stamps what the tree cost onto the span ctx carries. A file that
// arrived in a mode other than the one asked for met a filesystem that could
// not serve the request, which is worth reporting even though every file is
// in place.
func (tl *treeLinker) report(ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	counts := map[LinkMode]int64{
		LinkModeHardlink: tl.linked.Load(),
		LinkModeClone:    tl.cloned.Load(),
		LinkModeCopy:     tl.copied.Load(),
	}

	var total int64
	for _, n := range counts {
		total += n
	}

	fallback := LinkFallbackNone
	if counts[tl.mode] < total {
		fallback = linkFallbackByMode[tl.mode]
	}

	span.SetAttributes(
		attribute.Int64("files_linked", counts[LinkModeHardlink]),
		attribute.Int64("files_cloned", counts[LinkModeClone]),
		attribute.Int64("files_copied", counts[LinkModeCopy]),
		attribute.Int64("bytes_copied", tl.bytesCopied.Load()),
		attribute.String("fallback", string(fallback)),
	)
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
