// Package cas implements a content-addressable storage for git content.
//
// Blobs are copied from cloned repositories to a local store, along with trees.
// When the same content is requested again, the content is read from the local store,
// avoiding the need to clone the repository or read from the network.
package cas

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"errors"

	"golang.org/x/sync/errgroup"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// DefaultCASCloneDepth is the default shallow clone depth for CAS (git clone --depth).
const DefaultCASCloneDepth = 1

// gitScheme is the [SourceRequest.Scheme] the git ingest path travels under.
const gitScheme = "git"

// Option configures the behavior of CAS.
type Option func(*CAS)

// CloneOptions configures the behavior of a specific clone operation.
type CloneOptions struct {
	// LinkMode overrides the mode the CAS instance materializes trees in
	// for this clone alone. Nil leaves the instance's own mode in place.
	LinkMode *LinkMode

	// Dir specifies the target directory for the clone.
	// If empty, uses the repository name.
	Dir string

	// Branch specifies which branch to clone.
	// If empty, uses HEAD.
	Branch string

	// IncludedGitFiles names files from the source repository's git
	// directory to write under .git in the target directory. If empty,
	// no .git directory is written. The stored tree never carries these
	// entries; each is recorded separately against the commit so callers
	// asking for different lists share one tree and each receive only the
	// files they named.
	IncludedGitFiles []string

	// Depth limits the clone history to the given number of commits passed to git clone --depth.
	// If zero, CAS falls back to its configured clone depth (default shallow depth 1).
	// Set to -1 for full history (Terragrunt omits --depth; git rejects --depth 0).
	Depth int

	// Mutable, when true, copies blobs into the target directory instead of
	// hardlinking them from the CAS store. The destination tree becomes safe
	// to mutate without corrupting the shared store.
	Mutable bool
}

// CloneOption customizes a single [CAS.Clone] invocation. Each option
// mutates the per-call CloneOptions before the clone runs. Options are
// applied in order, so a later option overwrites fields set by an
// earlier one.
type CloneOption func(*CloneOptions)

// WithDir sets the target directory for the clone. Empty means use the
// repository name.
func WithDir(dir string) CloneOption {
	return func(o *CloneOptions) { o.Dir = dir }
}

// WithBranch sets the branch to clone. Empty means HEAD.
func WithBranch(branch string) CloneOption {
	return func(o *CloneOptions) { o.Branch = branch }
}

// WithIncludedGitFiles preserves the named files from the .git
// directory in the materialized clone.
func WithIncludedGitFiles(files []string) CloneOption {
	return func(o *CloneOptions) { o.IncludedGitFiles = files }
}

// WithDepth sets the `git clone --depth` value for this clone. Positive
// values request a shallow clone; -1 means full history (Terragrunt
// omits --depth; git rejects --depth 0). Zero falls back to the CAS-
// wide default configured via [WithCloneDepth].
func WithDepth(depth int) CloneOption {
	return func(o *CloneOptions) { o.Depth = depth }
}

// WithMutable copies blobs into the target directory instead of
// hardlinking from the CAS store, so the destination tree is safe to
// mutate without corrupting the shared store.
func WithMutable(mutable bool) CloneOption {
	return func(o *CloneOptions) { o.Mutable = mutable }
}

// WithCloneLinkMode materializes this clone in mode rather than in the mode
// the CAS instance carries. It serves a caller whose target directory has to
// hold files of a particular shape, such as one CAS itself reads back.
func WithCloneLinkMode(mode LinkMode) CloneOption {
	return func(o *CloneOptions) { o.LinkMode = &mode }
}

// CAS clones a git repository using content-addressable storage.
type CAS struct {
	blobStore         *Store
	treeStore         *Store
	synthStore        *Store
	gitFileStore      *Store
	gitStore          *GitStore
	probeCache        *ProbeCache
	storePath         string
	cloneDepth        int
	probeTTL          time.Duration
	probeMode         ProbeMode
	linkMode          LinkMode
	probeCacheEnabled bool
}

// WithStorePath specifies a custom path for the content store.
// If not set, defaults to <user cache dir>/terragrunt/cas/store,
// where the user cache dir honors XDG_CACHE_HOME on Linux and the
// platform equivalents on macOS and Windows (see os.UserCacheDir).
func WithStorePath(path string) Option {
	return func(c *CAS) {
		c.storePath = path
	}
}

// WithCloneDepth sets git clone --depth for CAS. Positive values request a
// shallow clone; -1 means full history (no --depth). Zero is invalid for git
// and is rejected by [ValidateCASCloneDepth] before reaching this option.
func WithCloneDepth(depth int) Option {
	return func(c *CAS) {
		c.cloneDepth = depth
	}
}

// WithProbeCache records every probe answer in the store and serves one
// back while it is fresh, so a later process can resolve a source without
// asking its remote again. Without it nothing is written to or read from
// `probes/` and every process probes every source, which is what a run
// gets until the offline-cas experiment is enabled.
//
// [WithProbeTTL] sets how long an answer for a source that can change
// upstream is served; a pinned source keeps [DefaultImmutableProbeTTL]
// regardless. [WithOffline] and [WithProbeRefresh] act on this cache and
// enable it themselves, having nothing to do without one.
func WithProbeCache() Option {
	return func(c *CAS) {
		c.probeCacheEnabled = true
	}
}

// WithOffline forbids every call to a remote. Probes answer from the local
// git store and the persisted probe cache regardless of TTL, and content
// the store lacks fails with [OfflineMissError] instead of being fetched.
// A source with no cheap probe, such as an SMB share, has no answer to
// serve and is still fetched.
// Passing this together with [WithProbeRefresh] is a caller bug that the
// CLI rejects at flag validation; here the later option wins.
func WithOffline() Option {
	return func(c *CAS) {
		c.probeMode = ProbeModeOffline
		c.probeCacheEnabled = true
	}
}

// WithProbeRefresh ignores the persisted probe cache for this process, so
// every probe reaches the network unless it joins one already in flight.
// Answers are still recorded for later runs.
func WithProbeRefresh() Option {
	return func(c *CAS) {
		c.probeMode = ProbeModeRefresh
		c.probeCacheEnabled = true
	}
}

// WithProbeTTL sets how long a persisted probe of a source that can change
// upstream is served without asking the remote again. Zero, the default,
// re-probes in every process so a push followed by a run sees the new
// revision. A pinned source keeps [DefaultImmutableProbeTTL] regardless.
// Has no effect without [WithProbeCache].
func WithProbeTTL(ttl time.Duration) Option {
	return func(c *CAS) {
		c.probeTTL = ttl
	}
}

// WithLinkMode sets how materialized trees reach their target directories.
// Without it, CAS uses [DefaultLinkMode].
func WithLinkMode(mode LinkMode) Option {
	return func(c *CAS) {
		c.linkMode = mode
	}
}

// New creates a new CAS instance with the given options.
func New(v *venv.Venv, opts ...Option) (*CAS, error) {
	c := &CAS{linkMode: DefaultLinkMode}

	for _, opt := range opts {
		opt(c)
	}

	if c.storePath == "" {
		cacheDir, err := util.EnsureCacheDir(v)
		if err != nil {
			return nil, err
		}

		c.storePath = filepath.Join(cacheDir, "cas", "store")
	}

	c.blobStore = NewStore(filepath.Join(c.storePath, "blobs"))
	c.treeStore = NewStore(filepath.Join(c.storePath, "trees"))
	c.synthStore = NewStore(filepath.Join(c.storePath, "synth", "trees"))
	c.gitFileStore = NewStore(filepath.Join(c.storePath, "gitfiles"))
	c.gitStore = NewGitStore(filepath.Join(c.storePath, "git"))
	c.probeCache = NewProbeCache(filepath.Join(c.storePath, probeCacheDirName))

	return c, nil
}

// BlobStore returns the store for blob content.
func (c *CAS) BlobStore() *Store { return c.blobStore }

// TreeStore returns the store for git-derived tree content.
func (c *CAS) TreeStore() *Store { return c.treeStore }

// SynthStore returns the store for synthetic tree content.
func (c *CAS) SynthStore() *Store { return c.synthStore }

// GitFileStore returns the store holding one record per (commit, git
// file name) pair for the files [CloneOptions.IncludedGitFiles] can
// name. Each record is a single tree line pointing at the file's blob;
// [GitFileKey] derives the record key.
func (c *CAS) GitFileStore() *Store { return c.gitFileStore }

// StorePath returns the root directory containing every CAS store.
func (c *CAS) StorePath() string { return c.storePath }

// LinkMode returns the mode this instance materializes trees in.
func (c *CAS) LinkMode() LinkMode { return c.linkMode }

// linkTreeOptions returns the options every tree this instance materializes
// is linked with: its own mode, plus the caller's own options, which come
// last so a caller that names a mode overrides the instance's.
func (c *CAS) linkTreeOptions(opts []LinkTreeOption) []LinkTreeOption {
	return append([]LinkTreeOption{WithTreeLinkMode(c.linkMode)}, opts...)
}

// ensureStorePaths creates the store directory hierarchy on v.FS. Callers
// invoke this from any top-level entry point that may write to a store, so
// the directories appear lazily on first use rather than at construction.
func (c *CAS) ensureStorePaths(v *venv.Venv) error {
	if !vfs.IsOSFS(v.FS) {
		return ErrGitStoreFSNotOS
	}

	if err := v.FS.MkdirAll(c.storePath, DefaultDirPerms); err != nil {
		return fmt.Errorf("create CAS store path: %w", err)
	}

	for _, s := range []*Store{c.blobStore, c.treeStore, c.synthStore, c.gitFileStore} {
		if err := v.FS.MkdirAll(s.Path(), DefaultDirPerms); err != nil {
			return fmt.Errorf("create CAS store subdirectory %s: %w", s.Path(), err)
		}
	}

	return nil
}

// GitResolver is a [SourceResolver] for git URLs.
//
// Branch travels as a field rather than as a URL query parameter so
// SCP-form URLs (`git@host:path`) reach git intact: net/url.Parse
// rejects SCP form, so any encoding scheme that round-trips through
// it silently loses the branch.
type GitResolver struct {
	// Venv supplies the executor Probe derives its git runner from. Required.
	Venv *venv.Venv
	// Logger receives probe-cache diagnostics. Required when Cache is set.
	Logger log.Logger
	// Store enables an offline fast path: a commit-form
	// [GitResolver.Branch] is checked against the local store before
	// reaching ls-remote, in the shapes [GitResolver.storeProbe]
	// accepts. When nil, every Probe runs ls-remote.
	Store *GitStore
	// Cache persists ls-remote answers across processes. When nil no
	// answer is recorded or served; the in-process flight still applies.
	Cache *ProbeCache
	// Branch is the ref to query. Empty means HEAD.
	Branch string
	// MutableTTL is how long a persisted probe for a branch, HEAD, or a
	// tag that is not a version is served without ls-remote. Zero means
	// every process re-probes. See [ProbeTTL].
	MutableTTL time.Duration
	// Mode selects between the network and the persisted cache.
	Mode ProbeMode
}

// probeResult is what one probe flight delivers to every caller that
// joined it.
type probeResult struct {
	key    string
	origin probeOrigin
}

// Scheme returns "git".
func (r *GitResolver) Scheme() string { return gitScheme }

// Pinned always reports false. Git decides immutability after probing,
// from the ref ls-remote matched rather than the ref that was asked for
// (see [GitResolver.recordProbe]), so it has no pre-probe answer to give.
// Nothing consults it: [CAS.Clone] asks for [ProbeCachedByResolver].
func (r *GitResolver) Pinned(_ string) bool { return false }

// Probe returns the commit SHA for r.Branch (HEAD when empty). The
// returned SHA is the cache key verbatim and doubles as the git
// object name the fetcher consumes.
//
// `git ls-remote` is authoritative; ls-remote misses (the caller
// supplied a commit-form ref directly) surface as
// [ErrNoVersionMetadata] so the fetcher canonicalizes via rev-parse.
// When [GitResolver.Store] is set, a commit r.Branch names that the
// store already holds short-circuits ls-remote (see
// [GitResolver.storeProbe]). Concurrent probes of the same (URL, ref)
// anywhere in the process share one ls-remote, and a persisted answer
// within its TTL (see [ProbeTTL]) skips ls-remote entirely.
func (r *GitResolver) Probe(ctx context.Context, rawURL string) (string, error) {
	if hash, ok := r.storeProbe(ctx, rawURL); ok {
		recordProbeOrigin(ctx, probeOriginGitStore)

		return hash, nil
	}

	res, err := flights.probe.do(ctx, r.flightKey(rawURL), func(ctx context.Context) (probeResult, error) {
		return r.probeUncoalesced(ctx, rawURL)
	})
	if err != nil {
		return "", err
	}

	recordProbeOrigin(ctx, res.origin)

	return res.key, nil
}

// storeProbe answers from the local bare repository when it already
// holds the commit r.Branch names.
//
// Online only a full-length SHA is offered to the store, so a hex-named
// branch keeps tracking its remote tip instead of freezing at the first
// commit its name prefixed. Offline there is no tip to track and the
// only other outcome is [OfflineMissError], so an abbreviated SHA is
// offered too; [GitStore.ProbeCachedCommit] rejects a name that resolved
// through ref lookup on its own.
func (r *GitResolver) storeProbe(ctx context.Context, rawURL string) (string, bool) {
	if r.Store == nil {
		return "", false
	}

	if !looksLikeFullSHA(r.Branch) && r.Mode != ProbeModeOffline {
		return "", false
	}

	return r.Store.ProbeCachedCommit(ctx, r.Venv, rawURL, r.Branch)
}

// flightKey identifies the probe of rawURL for r.Branch within the
// process. See [probeFlightKey] for how the key is built.
func (r *GitResolver) flightKey(rawURL string) string {
	root := ""
	if r.Cache != nil {
		root = r.Cache.RootPath()
	}

	return probeFlightKey(root, rawURL, r.Branch)
}

// probeFlightKey identifies the probe of url for ref within the process.
// It carries the cache root because the flight records its answer there,
// and probes over different stores must each record their own. The URL is
// redacted so the flights line up with the partition
// [ProbeCache.EntryPath] records under: two units reaching one source
// through different credentials share an answer here for the same reason
// they share the persisted one.
func probeFlightKey(cacheRoot, url, ref string) string {
	return cacheRoot + "\x00" + RedactURL(url) + "\x00" + ref
}

// probeUncoalesced answers one probe from the persisted cache or, when
// the mode allows it, from ls-remote, recording a fresh network answer
// for later processes.
func (r *GitResolver) probeUncoalesced(ctx context.Context, rawURL string) (probeResult, error) {
	if entry, ok := r.cachedProbe(rawURL); ok {
		return probeResult{key: entry.Key, origin: probeOriginProbeCache}, nil
	}

	if r.Mode == ProbeModeOffline {
		return probeResult{}, &OfflineMissError{Source: RedactURL(rawURL), Ref: probeRefName(r.Branch)}
	}

	runner, err := git.NewGitRunner(r.Venv)
	if err != nil {
		return probeResult{}, err
	}

	results, err := runner.LsRemote(ctx, rawURL, r.Branch)
	if err != nil {
		if errors.Is(err, git.ErrNoMatchingReference) {
			return probeResult{}, ErrNoVersionMetadata
		}

		return probeResult{}, err
	}

	if len(results) == 0 {
		return probeResult{}, ErrNoVersionMetadata
	}

	r.recordProbe(rawURL, results[0])

	return probeResult{key: results[0].Hash, origin: probeOriginLsRemote}, nil
}

// cachedProbe returns the persisted answer for rawURL when the mode and
// TTL allow serving it.
func (r *GitResolver) cachedProbe(rawURL string) (ProbeEntry, bool) {
	if r.Cache == nil || r.Mode == ProbeModeRefresh {
		return ProbeEntry{}, false
	}

	entry, ok := r.Cache.Lookup(r.Venv.FS, rawURL, r.Branch)
	if !ok || !looksLikeFullSHA(entry.Key) {
		return ProbeEntry{}, false
	}

	if r.Mode == ProbeModeOffline {
		return entry, true
	}

	ttl := ProbeTTL(&entry, r.MutableTTL)
	if ttl <= 0 || time.Since(entry.ProbedAt) >= ttl {
		return ProbeEntry{}, false
	}

	return entry, true
}

// recordProbe persists a network answer. A failed write costs only the
// next process a probe, so it is logged rather than failing a probe that
// already succeeded.
func (r *GitResolver) recordProbe(rawURL string, res git.LsRemoteResult) {
	if r.Cache == nil {
		return
	}

	// Immutability follows the ref ls-remote matched, not the name asked
	// for: a branch named v1.2.3 shadows a tag of that name in ls-remote's
	// output and must keep re-probing like any other branch.
	entry := ProbeEntry{
		ProbedAt:  time.Now(),
		Key:       res.Hash,
		Immutable: IsSemverTag(res.Ref),
	}

	if err := r.Cache.Store(r.Venv.FS, rawURL, r.Branch, &entry); err != nil {
		r.Logger.Debugf("cas: probe cache write for %s failed: %v", RedactURL(rawURL), err)
	}
}

// newGitResolver builds the [GitResolver] for branch with this CAS's
// stores, probe cache, and mode.
func (c *CAS) newGitResolver(l log.Logger, v *venv.Venv, branch string) *GitResolver {
	cache := c.probeCache
	if !c.probeCacheEnabled {
		cache = nil
	}

	return &GitResolver{
		Venv:       v,
		Logger:     l,
		Store:      c.gitStore,
		Cache:      cache,
		Branch:     branch,
		MutableTTL: c.probeTTL,
		Mode:       c.probeMode,
	}
}

// Clone fetches url into the target directory through the CAS, using a
// [GitResolver] for the probe and ingesting via `git ls-tree -r` /
// `git cat-file` so the native git blob and tree formats reach the
// stores intact. Callers customize the clone by passing options such as
// [WithDir], [WithBranch], or [WithDepth]; calling Clone with no
// options runs against the zero CloneOptions.
//
// Requires v.FS for store I/O and v.Exec for the git runner the
// underlying clone derives. Panics with [venv.ErrVenvFSUnset] or
// [venv.ErrVenvExecUnset] if either is unset.
func (c *CAS) Clone(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	url string,
	options ...CloneOption,
) error {
	v.RequireFS()
	v.RequireExec()

	opts := CloneOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	clonedOpts := opts
	clonedOpts.Dir = c.prepareTargetDirectory(opts.Dir, url)

	return c.FetchSource(ctx, l, v, &clonedOpts, SourceRequest{
		Scheme:       gitScheme,
		URL:          url,
		Resolver:     c.newGitResolver(l, v, opts.Branch),
		Fetch:        c.gitFetcher(url, &opts),
		Attrs:        map[string]any{"branch": opts.Branch},
		ProbeCaching: ProbeCachedByResolver,
	})
}

// EnsureBlob stores the blob named by hash unless the store already has
// it, streaming its content straight from batch into the store's temp
// file instead of going through [Content.Store]. The stored blob takes the
// git tree mode with the write bits cleared, so the default-link path can
// hardlink it without changing whether it is executable.
func (c *CAS) EnsureBlob(
	v *venv.Venv,
	batch *git.CatFileBatch,
	hash string,
	gitPerm os.FileMode,
) (err error) {
	v.RequireGOOS()

	needsWrite, unlock := c.blobStore.EnsureWithWait(v, hash)
	defer unlock()

	if !needsWrite {
		return nil
	}

	content := NewContent(c.blobStore)

	tmpHandle, err := content.GetTmpHandle(v, hash)
	if err != nil {
		return err
	}

	tmpPath := tmpHandle.Name()

	defer func() {
		if _, statErr := v.FS.Stat(tmpPath); statErr == nil {
			err = errors.Join(err, v.FS.Remove(tmpPath))
		}
	}()

	if err = streamBlob(v, batch, hash, tmpHandle); err != nil {
		return err
	}

	// Symlink entries (git mode 120000) have no permission bits, but the blob
	// stores the link target string and must stay readable so linkTree can
	// resolve the symlink at materialization time.
	storedPerm := gitPerm.Perm() &^ WriteBitMask
	if storedPerm == 0 {
		storedPerm = StoredFilePerms
	}

	// The mode is set before the rename so a reader never sees the object at
	// the temp file's mode between the two steps.
	if err = v.FS.Chmod(tmpPath, storedPerm); err != nil {
		return err
	}

	return content.publish(v, tmpPath, hash)
}

// streamBlob reads the blob named by hash from batch into tmpHandle and
// closes the handle whatever the outcome. The caller removes or renames
// the file next, and Windows refuses both while a handle is open.
func streamBlob(
	v *venv.Venv,
	batch *git.CatFileBatch,
	hash string,
	tmpHandle vfs.File,
) (err error) {
	defer func() {
		err = errors.Join(err, tmpHandle.Close())
	}()

	if err := batch.ReadBlob(hash, tmpHandle); err != nil {
		return err
	}

	if v.Platform.GOOS == WindowsOS {
		return tmpHandle.Sync()
	}

	return nil
}

// gitFetcher returns a SourceFetcher that ingests through the git-native
// path (cat-file + ls-tree). A non-empty suggestedKey is the canonical
// commit SHA from ls-remote; empty means ls-remote produced no match and
// rev-parse against the central GitStore canonicalizes the user ref after
// fetching.
func (c *CAS) gitFetcher(url string, opts *CloneOptions) SourceFetcher {
	return func(
		ctx context.Context,
		l log.Logger,
		v *venv.Venv,
		suggestedKey string,
		mode IngestMode,
	) (string, error) {
		var ref resolvedRef
		if suggestedKey != "" {
			ref = &symbolicRef{URL: url, Branch: opts.Branch, Hash: suggestedKey}
		} else {
			ref = &commitRef{URL: url, RawRef: opts.Branch}
		}

		return c.populateTreeFromRef(ctx, l, v, opts, ref, mode)
	}
}

// populateTreeFromRef returns the canonical commit hash for ref, ingesting
// its tree and opts.IncludedGitFiles when the store lacks either.
// Concurrent ingests of the same ref and list anywhere in the process
// share one flight; the flight runs with the first caller's logger, venv,
// and clone options, and later callers only wait for what it stores.
//
// Under [IngestRepair] the store hit is passed over, so the tree is
// ingested again and the objects it names are written back.
func (c *CAS) populateTreeFromRef(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *CloneOptions,
	ref resolvedRef,
	mode IngestMode,
) (string, error) {
	if hash := ref.knownHash(); mode.trustsStoreHits() && hash != "" && !c.needsIngest(v, hash, opts) {
		return hash, nil
	}

	if c.probeMode == ProbeModeOffline {
		url, requested := ref.origin()

		return "", &OfflineMissError{Source: RedactURL(url), Ref: probeRefName(requested)}
	}

	return flights.ingest.do(ctx, c.ingestFlightKey(ref, mode, opts), func(ctx context.Context) (string, error) {
		return c.ingestRef(ctx, l, v, opts, ref, mode)
	})
}

// ingestFlightKey identifies what an ingest writes into this store: the
// tree for the commit hash when it is already known, otherwise for the
// (URL, ref) pair that will resolve to it, plus the records for
// opts.IncludedGitFiles. The list is part of the key because a caller
// joining a flight only waits for it, so a flight run with another
// caller's list would leave the joiner's .git files unrecorded.
//
// The mode is part of the key because a repair that joined a cached
// ingest would get back an ingest that trusted the store hit the repair
// was started to correct.
func (c *CAS) ingestFlightKey(ref resolvedRef, mode IngestMode, opts *CloneOptions) string {
	scope := c.storePath + "\x00" + strconv.Itoa(int(mode))
	files := "\x00files\x00" + strings.Join(opts.IncludedGitFiles, "\x00")

	if hash := ref.knownHash(); hash != "" {
		return scope + "\x00commit\x00" + hash + files
	}

	url, requested := ref.origin()

	return scope + "\x00ref\x00" + url + "\x00" + requested + files
}

// ingestRef dispatches by ref kind to the populate that fills the store.
// Returns the canonical commit hash.
func (c *CAS) ingestRef(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *CloneOptions,
	ref resolvedRef,
	mode IngestMode,
) (string, error) {
	switch ref := ref.(type) {
	case *symbolicRef:
		if err := c.populateTreeFromSymbolicRef(ctx, l, v, opts, ref, mode); err != nil {
			return "", err
		}

		return ref.Hash, nil

	case *commitRef:
		return c.populateTreeFromCommitRef(ctx, l, v, opts, ref, mode)

	default:
		return "", fmt.Errorf("unsupported resolved ref type %T", ref)
	}
}

// populateTreeFromSymbolicRef stores the tree and reachable blobs
// for ref.Hash in the CAS. Tries the central [GitStore] first; on
// any error from it, logs a warning and falls back to a bare clone
// in a temporary directory.
func (c *CAS) populateTreeFromSymbolicRef(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *CloneOptions,
	ref *symbolicRef,
	mode IngestMode,
) error {
	gitRunner, err := git.NewGitRunner(v)
	if err != nil {
		return err
	}

	depth := resolveCloneDepth(opts.Depth, c.cloneDepth)

	repo, err := c.gitStore.EnsureRef(ctx, l, v, ref.URL, ref.Branch, ref.Hash, depth)
	if err == nil {
		defer repo.Release(l)

		runner := gitRunner.WithWorkDir(repo.Path)

		return c.storeRootTreeFrom(ctx, l, v, runner, ref.URL, ref.Hash, opts, mode)
	}

	l.Warnf(
		"central git store unavailable for %s, falling back to temporary clone: %v",
		ref.URL,
		err,
	)
	RecordFallback(ctx, l, FallbackReasonGitStoreUnavailable, map[string]any{"url": ref.URL})

	tempDir, cleanup, err := c.makeFallbackCloneDir(l, v)
	if err != nil {
		return err
	}

	defer cleanup()

	runner := gitRunner.WithWorkDir(tempDir)

	if err := runner.Clone(ctx, ref.URL, true, depth, ref.Branch); err != nil {
		return err
	}

	return c.storeRootTreeFrom(ctx, l, v, runner, ref.URL, ref.Hash, opts, mode)
}

// populateTreeFromCommitRef resolves ref via [GitStore.EnsureCommit]
// (a cache miss fetches through [fetchPinnedCommit]) and stores its tree
// in the CAS. Returns the canonical commit hash. Falls back to a
// temporary bare clone if the central [GitStore] is unavailable.
func (c *CAS) populateTreeFromCommitRef(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *CloneOptions,
	ref *commitRef,
	mode IngestMode,
) (string, error) {
	gitRunner, err := git.NewGitRunner(v)
	if err != nil {
		return "", err
	}

	repo, err := c.gitStore.EnsureCommit(ctx, l, v, ref.URL, ref.RawRef, ref.Hash)
	if err == nil {
		defer repo.Release(l)

		if mode.trustsStoreHits() && !c.needsIngest(v, repo.Hash, opts) {
			return repo.Hash, nil
		}

		runner := gitRunner.WithWorkDir(repo.Path)

		if err := c.storeRootTreeFrom(ctx, l, v, runner, ref.URL, repo.Hash, opts, mode); err != nil {
			return "", err
		}

		return repo.Hash, nil
	}

	if errors.Is(err, git.ErrNoMatchingReference) {
		return "", err
	}

	l.Warnf(
		"central git store unavailable for %s, falling back to temporary clone: %v",
		ref.URL,
		err,
	)
	RecordFallback(ctx, l, FallbackReasonGitStoreUnavailable, map[string]any{"url": ref.URL})

	tempDir, cleanup, err := c.makeFallbackCloneDir(l, v)
	if err != nil {
		return "", err
	}

	defer cleanup()

	runner := gitRunner.WithWorkDir(tempDir)

	if err := runner.Clone(ctx, ref.URL, true, 0, ""); err != nil {
		return "", err
	}

	canonicalHash, err := runner.RevParseCommit(ctx, ref.RawRef)
	if err != nil {
		if errors.Is(err, git.ErrUnknownRevision) {
			return "", &git.WrappedError{
				Op:      "git_clone_resolve",
				Context: fmt.Sprintf("%q in %s", ref.RawRef, ref.URL),
				Err:     git.ErrNoMatchingReference,
			}
		}

		return "", err
	}

	if mode.trustsStoreHits() && !c.needsIngest(v, canonicalHash, opts) {
		return canonicalHash, nil
	}

	if err := c.storeRootTreeFrom(ctx, l, v, runner, ref.URL, canonicalHash, opts, mode); err != nil {
		return "", err
	}

	return canonicalHash, nil
}

// makeFallbackCloneDir creates a temporary directory for a bare clone
// fallback and returns a cleanup function that removes it.
func (c *CAS) makeFallbackCloneDir(l log.Logger, v *venv.Venv) (string, func(), error) {
	tempDir, err := vfs.MkdirTemp(v.FS, "", "terragrunt-cas-fallback-")
	if err != nil {
		return "", nil, fmt.Errorf(
			"create fallback clone dir: %w",
			errors.Join(ErrFallbackCloneDir, err),
		)
	}

	cleanup := func() {
		if rmErr := v.FS.RemoveAll(tempDir); rmErr != nil {
			l.Warnf("cleanup error: %v", rmErr)
		}
	}

	return tempDir, cleanup, nil
}

func resolveCloneDepth(optDepth, casDepth int) int {
	depth := optDepth
	if depth == 0 {
		depth = casDepth
	}

	if depth == 0 {
		depth = DefaultCASCloneDepth
	}

	if depth < 0 {
		depth = 0
	}

	return depth
}

func (c *CAS) prepareTargetDirectory(dir, url string) string {
	targetDir := dir
	if targetDir == "" {
		targetDir = git.ExtractRepoName(url)
	}

	return filepath.Clean(targetDir)
}

// resolvedRef is a sealed sum type returned by [CAS.resolveReference]:
// [symbolicRef] when ls-remote canonicalized the input, [commitRef]
// otherwise.
type resolvedRef interface {
	CommitHash() string
	// knownHash returns the canonical commit hash when resolution already
	// produced one, and "" when only a fetch can produce it.
	knownHash() string
	// origin returns the remote URL and the ref the caller asked for.
	origin() (url, ref string)
}

// symbolicRef carries an ls-remote-resolved branch, tag, or HEAD.
type symbolicRef struct {
	URL    string
	Branch string // ref name, used for the per-ref fetch
	Hash   string // canonical commit hash
}

// CommitHash returns the canonical commit hash ls-remote resolved.
func (r *symbolicRef) CommitHash() string { return r.Hash }

func (r *symbolicRef) knownHash() string { return r.Hash }

func (r *symbolicRef) origin() (string, string) { return r.URL, r.Branch }

// commitRef carries a ref ls-remote did not canonicalize. The central git
// store resolves it later via rev-parse, fetching through
// [fetchPinnedCommit] on a cache miss.
type commitRef struct {
	// URL is the remote repository URL.
	URL string
	// RawRef is the user-supplied ref. Any form `git rev-parse` accepts
	// works (full SHA, abbreviated SHA, name ls-remote did not match).
	RawRef string
	// Hash is the canonical SHA when [GitStore.ProbeCachedCommit]
	// resolved RawRef locally before reaching ls-remote; empty
	// otherwise. Lets downstream code skip a redundant rev-parse.
	Hash string
}

// CommitHash returns the user-supplied ref, not r.Hash. Stacks key the
// CAS on this value, and the key must not depend on whether the central
// git store happened to have the commit cached.
func (r *commitRef) CommitHash() string { return r.RawRef }

func (r *commitRef) knownHash() string { return r.Hash }

func (r *commitRef) origin() (string, string) { return r.URL, r.RawRef }

// resolveReference resolves branch into a [resolvedRef] via [GitResolver].
//
// Full-length SHAs (40 or 64 hex chars) are checked against the local git
// store first so previously-cloned commits resolve offline; online an
// abbreviated SHA skips that check to avoid mistaking a hex-named branch
// tip for the SHA prefix and freezing the branch at its first-fetched tip
// (see [looksLikeFullSHA]). Offline there is no moving tip to track and
// the alternative is a failed run, so the store answers an abbreviated
// SHA too, still as a [commitRef] so the stack key stays the ref the user
// wrote.
func (c *CAS) resolveReference(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	url, branch string,
) (resolvedRef, error) {
	if looksLikeFullSHA(branch) || c.probeMode == ProbeModeOffline {
		if hash, ok := c.gitStore.ProbeCachedCommit(ctx, v, url, branch); ok {
			return &commitRef{URL: url, RawRef: branch, Hash: hash}, nil
		}
	}

	key, err := c.newGitResolver(l, v, branch).Probe(ctx, url)
	if err != nil {
		if errors.Is(err, ErrNoVersionMetadata) {
			return &commitRef{URL: url, RawRef: branch}, nil
		}

		return nil, err
	}

	return &symbolicRef{URL: url, Branch: branch, Hash: key}, nil
}

// looksLikeFullSHA reports whether s is exactly 40 or 64 hex characters,
// the canonical lengths for SHA-1 and SHA-256 commit hashes. Abbreviations
// are intentionally rejected; see the offline-probe rationale on
// [CAS.resolveReference].
func looksLikeFullSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}

	_, err := hex.DecodeString(s)

	return err == nil
}

// storeRootTreeFrom reads the recursive tree at hash from the supplied
// runner's working repository and stores its tree and reachable blobs in
// the CAS, then records each of opts.IncludedGitFiles against hash. The
// runner must already have its WorkDir pointed at a bare repo (or
// worktree) that contains the requested object. url is the remote the
// repository came from; submodule ingestion resolves relative .gitmodules
// URLs against it.
func (c *CAS) storeRootTreeFrom(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	runner *git.GitRunner,
	url, hash string,
	opts *CloneOptions,
	mode IngestMode,
) error {
	tree, err := runner.LsTreeRecursive(ctx, hash)
	if err != nil {
		return err
	}

	if err = c.storeTreeRecursive(ctx, l, v, runner, url, hash, tree, mode); err != nil {
		return err
	}

	return c.storeIncludedGitFiles(l, v, runner.WorkDir, hash, opts.IncludedGitFiles)
}

// needsIngest reports whether the store lacks the tree at hash or a
// record for any of opts.IncludedGitFiles against it. Checking the tree
// alone would let an earlier ingest with a shorter list short-circuit a
// later caller out of the .git files it asked for.
func (c *CAS) needsIngest(v *venv.Venv, hash string, opts *CloneOptions) bool {
	if c.treeStore.NeedsWrite(v, hash) {
		return true
	}

	for _, name := range opts.IncludedGitFiles {
		if c.gitFileStore.NeedsWrite(v, GitFileKey(hash, name)) {
			return true
		}
	}

	return false
}

// storeIncludedGitFiles copies each named file from the git directory
// at repoDir into the blob store and records it against hash. Names
// already recorded are skipped, so a later caller with a longer list
// only adds the files the earlier ingest did not cover. The record is
// written after the blob so a record hit implies the blob is present.
func (c *CAS) storeIncludedGitFiles(
	l log.Logger,
	v *venv.Venv,
	repoDir, hash string,
	names []string,
) error {
	blobContent := NewContent(c.blobStore)
	recordContent := NewContent(c.gitFileStore)

	for _, name := range names {
		key := GitFileKey(hash, name)
		if !c.gitFileStore.NeedsWrite(v, key) {
			continue
		}

		srcPath := filepath.Join(repoDir, name)

		info, err := v.FS.Stat(srcPath)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return &WrappedError{Op: "store_git_file", Path: srcPath, Err: ErrIncludedGitFileIsDir}
		}

		blobHash, err := hashFile(v.FS, srcPath)
		if err != nil {
			return err
		}

		if err := blobContent.EnsureCopy(l, v, blobHash, srcPath); err != nil {
			return err
		}

		record := fmt.Appendf(nil, "%06o blob %s\t%s\n", info.Mode().Perm(), blobHash, name)

		if err := recordContent.EnsureWithWait(l, v, key, record, StoredFilePerms); err != nil {
			return err
		}
	}

	return nil
}

// GitFileKey returns the [CAS.GitFileStore] key for the git directory
// file name recorded against the commit at hash.
func GitFileKey(hash, name string) string {
	h := sha256.New()
	h.Write([]byte("gitfile\x00"))
	h.Write([]byte(hash))
	h.Write([]byte{0})
	h.Write([]byte(name))

	return hex.EncodeToString(h.Sum(nil))
}

// storeTreeRecursive stores a tree fetched from git ls-tree -r. The tree
// object is written last so a tree-store hit implies every blob and
// submodule tree it references is already present.
//
// [IngestRepair] is what to pass once that implication has been shown
// false. The listing is walked again and every object it names that the
// store lacks is written, so a store missing one blob is read back out of
// the repository one blob at a time, not wholesale.
func (c *CAS) storeTreeRecursive(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	runner *git.GitRunner,
	url, hash string,
	tree *git.Tree,
	mode IngestMode,
) error {
	if mode.trustsStoreHits() && !c.treeStore.NeedsWrite(v, hash) {
		return nil
	}

	if err := c.storeBlobs(ctx, v, runner, tree.Entries()); err != nil {
		return err
	}

	if err := c.storeSubmodules(ctx, l, v, runner, url, tree, mode); err != nil {
		return err
	}

	treeContent := NewContent(c.treeStore)
	if err := treeContent.EnsureWithWait(l, v, hash, tree.Data(), StoredFilePerms); err != nil {
		return err
	}

	return nil
}

// storeBlobs stores blobs in the CAS. Gitlink entries (type "commit")
// name objects that live in another repository entirely, so only blob
// entries are written. Submodule contents arrive via
// [CAS.storeSubmodules]. Every blob the store lacks is read through a
// `git cat-file --batch` process, started only when there is something to
// read.
//
// The pending blobs are split across [BlobShards] shards, which overlaps
// the store's own writes. Shards never outnumber the blobs they serve,
// so a small tree still starts a single git process.
func (c *CAS) storeBlobs(
	ctx context.Context,
	v *venv.Venv,
	runner *git.GitRunner,
	entries []git.TreeEntry,
) error {
	pending := c.blobsNeedingWrite(v, entries)
	if len(pending) == 0 {
		return nil
	}

	shards := BlobShards(v.FS, c.storePath, len(pending))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(shards)

	for shard := range shards {
		g.Go(func() error {
			return c.storeBlobShard(ctx, v, runner, pending, shard, shards)
		})
	}

	return g.Wait()
}

// overlayIngestShards is what ingest peaks at on overlayfs, well under
// the sixteen concurrent writers that filesystem absorbs for a walk.
//
// ext4 runs this same subprocess-per-shard workload fastest
// at 16 shards on 2, 4, 8 and 16 cores, and overlayfs stays at
// four across the same range.
const overlayIngestShards = 4

// BlobShards reports how many shards ingest should split pending blobs
// across when the store lives at storePath on fsys, never more than
// there are blobs to serve.
func BlobShards(fsys vfs.FS, storePath string, pending int) int {
	kind := vfs.DetectFSKind(fsys, storePath)

	shards := vfs.FSWorkersForKind(kind)
	if kind == vfs.FSOverlay {
		shards = overlayIngestShards
	}

	return min(shards, pending)
}

// storeBlobShard stores the pending blobs at every index congruent to
// shard modulo shards. A batch answers requests in order on one pair of
// pipes, so it serves a single goroutine. Each shard therefore starts and
// closes its own.
func (c *CAS) storeBlobShard(
	ctx context.Context,
	v *venv.Venv,
	runner *git.GitRunner,
	pending []git.TreeEntry,
	shard int,
	shards int,
) (err error) {
	batch, err := runner.StartCatFileBatch(ctx)
	if err != nil {
		return err
	}

	defer func() {
		err = errors.Join(err, batch.Close())
	}()

	for i := shard; i < len(pending); i += shards {
		entry := pending[i]
		if err := c.EnsureBlob(v, batch, entry.Hash, gitFilePerm(entry.Mode)); err != nil {
			return err
		}
	}

	return nil
}

// blobsNeedingWrite filters entries down to the blobs the store lacks.
func (c *CAS) blobsNeedingWrite(v *venv.Venv, entries []git.TreeEntry) []git.TreeEntry {
	var pending []git.TreeEntry

	for _, entry := range entries {
		if entry.Type != git.EntryTypeBlob {
			continue
		}

		if !c.blobStore.NeedsWrite(v, entry.Hash) {
			continue
		}

		pending = append(pending, entry)
	}

	return pending
}

// storeSubmodules ingests the repositories behind gitlink entries so the
// materializer can later link each submodule's tree by its pinned commit
// hash, which is exactly the key the recursive ingest stores it under.
// Submodule URLs come from the tree's .gitmodules blob, with relative
// URLs resolved against url. Recursion bottoms out naturally: nested
// gitlinks pin commits by hash, and a hash cycle cannot be constructed.
//
// Gitlinks without a .gitmodules entry (the shape left behind by
// accidentally committing a nested repository) are skipped, and
// materialization leaves an empty directory for them, matching
// `git clone`.
func (c *CAS) storeSubmodules(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	runner *git.GitRunner,
	url string,
	tree *git.Tree,
	mode IngestMode,
) error {
	var gitlinks []git.TreeEntry

	for _, entry := range tree.Entries() {
		if entry.Type == git.EntryTypeCommit {
			gitlinks = append(gitlinks, entry)
		}
	}

	if len(gitlinks) == 0 {
		return nil
	}

	urls, err := submoduleURLs(ctx, runner, tree)
	if err != nil {
		return err
	}

	for _, entry := range gitlinks {
		subURL, ok := urls[entry.Path]
		if !ok {
			l.Debugf(
				"cas: gitlink %s has no .gitmodules entry, leaving an empty directory",
				entry.Path,
			)

			continue
		}

		resolvedURL := git.ResolveSubmoduleURL(url, subURL)

		ref := &commitRef{URL: resolvedURL, RawRef: entry.Hash, Hash: entry.Hash}
		if _, err := c.populateTreeFromRef(ctx, l, v, &CloneOptions{}, ref, mode); err != nil {
			return fmt.Errorf("fetch submodule %s from %s: %w", entry.Path, resolvedURL, err)
		}
	}

	return nil
}

// submoduleURLs reads the submodule path → URL table from the tree's
// .gitmodules blob. A tree without one yields an empty table.
func submoduleURLs(
	ctx context.Context,
	runner *git.GitRunner,
	tree *git.Tree,
) (map[string]string, error) {
	for _, entry := range tree.Entries() {
		if entry.Path == git.GitmodulesPath && entry.Type == git.EntryTypeBlob {
			return runner.SubmoduleURLs(ctx, entry.Hash)
		}
	}

	return nil, nil
}

func hashFile(fsys vfs.FS, path string) (string, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return "", err
	}

	h := sha1.New()

	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}

	hash := hex.EncodeToString(h.Sum(nil))

	if err := file.Close(); err != nil {
		return hash, fmt.Errorf(
			"hash of %s successfully computed as "+
				"%s, but closing the file failed: %w",
			path,
			hash,
			err,
		)
	}

	return hash, nil
}
