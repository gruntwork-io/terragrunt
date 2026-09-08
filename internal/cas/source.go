package cas

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ErrNoVersionMetadata reports that a SourceResolver had no usable
// version identifier for a source. Callers fall back to downloading,
// walking the result, and keying the tree by its content hash.
var ErrNoVersionMetadata = errors.New("no version metadata available for source")

// SourceResolver derives a tree-store cache key for a source from a
// cheap remote probe, so FetchSource can short-circuit the download
// when the bytes haven't changed upstream.
type SourceResolver interface {
	// Scheme returns the URL scheme this resolver handles (e.g.
	// "s3", "gcs", "http").
	Scheme() string

	// Pinned reports whether rawURL names content that cannot change
	// upstream, such as an object version or an exact module version. A
	// pinned source's recorded probe is served for
	// [DefaultImmutableProbeTTL]; everything else is held only for the
	// mutable TTL the caller set, which is zero by default.
	Pinned(rawURL string) bool

	// Probe returns a cache key for rawURL.
	//
	// Returns ErrNoVersionMetadata when the source has no cheap
	// signal; FetchSource then falls back to downloading and
	// content-hashing. Other errors are logged and treated the same
	// way, so a misconfigured probe never breaks a fetch.
	Probe(ctx context.Context, rawURL string) (cacheKey string, err error)
}

// IngestMode says whether an ingest may trust what the store already
// holds. Ingest writes a tree only after every object that tree names, so
// a tree in the store normally means its blobs are there too and the work
// behind it can be skipped.
type IngestMode int

const (
	// IngestCached takes that shortcut, and is where every fetch starts.
	IngestCached IngestMode = iota

	// IngestRepair skips no work, re-ingesting from the source so that
	// objects missing from the store are written again. [CAS.FetchSource]
	// switches to it for a single retry after a [MissingObjectError].
	IngestRepair
)

// trustsStoreHits reports whether m may skip work the store appears to
// have done already.
func (m IngestMode) trustsStoreHits() bool {
	return m == IngestCached
}

// SourceFetcher downloads and ingests a source into CAS, returning the
// tree-store key the materialized tree was written under.
//
// suggestedKey is the probe-derived cache key, or empty when the probe
// produced none. Fetchers that learn the canonical key only after
// downloading (the git rev-parse path) may ignore it and return the
// canonical key instead.
//
// mode is [IngestRepair] when the store turned out to be missing an
// object and this call is the attempt to restore it. A fetcher that
// downloads and re-ingests everything it is handed needs no special
// handling, since storing content checks the store per object anyway; a
// fetcher carrying store shortcuts of its own must skip them.
type SourceFetcher func(
	ctx context.Context, l log.Logger, v *venv.Venv, suggestedKey string, mode IngestMode,
) (treeKey string, err error)

// ProbeCaching says which layer consults and records the probe cache for
// a source.
type ProbeCaching int

const (
	// ProbeCachedByCAS lets [CAS.FetchSource] share the resolver's
	// answers: concurrent callers coalesce onto one probe, and a recorded
	// answer is served while it is fresh. This is the zero value, so a new
	// resolver gets the sharing without opting in.
	ProbeCachedByCAS ProbeCaching = iota
	// ProbeCachedByResolver leaves the resolver to it, which [CAS.Clone]
	// asks for. The git probe answers a pinned commit from the local bare
	// repository before the offline gate, files entries under the ref it
	// probed, and reads immutability from the ref ls-remote matched. The
	// generic path supports none of those.
	ProbeCachedByResolver
)

// SourceRequest is the input to CAS.FetchSource.
type SourceRequest struct {
	// Resolver probes the source for a cache key. Nil means always
	// download and hash.
	Resolver SourceResolver
	// Fetch ingests the source. Required.
	Fetch SourceFetcher
	// Attrs are scheme-specific telemetry attributes merged into the
	// cas_fetch_source span (e.g. the git path adds "branch").
	Attrs map[string]any
	// Scheme is the URL scheme of URL ("s3", "gcs", "http"). Used in
	// telemetry; resolvers that need it pull it from URL themselves.
	Scheme string
	// URL is the canonical source URL. Passed to Resolver.Probe and
	// used in error messages.
	URL string
	// ProbeCaching says whether FetchSource shares and persists this
	// resolver's probe answers or leaves that to the resolver.
	ProbeCaching ProbeCaching
}

// FetchSource routes src through the CAS. On a probe hit it links the
// cached tree into opts.Dir without invoking Fetch. On a probe miss it
// calls Fetch and links the resulting tree.
//
// A store that turns out to be missing an object the cached tree names
// costs one extra pass: src is ingested again under [IngestRepair], which
// writes the missing object back, and the link is retried. The second
// failure is returned as it stands, so a source that can no longer supply
// the object reports [MissingObjectError] rather than looping. Under
// [WithOffline] there is no second pass: the miss is returned as an
// [OfflineRepairError] instead of asking the remote the flag forbids.
//
// opts.Dir is the destination. opts.Mutable selects copy vs hardlink
// for the final link, matching the git path. opts.IncludedGitFiles are
// served from the records the git ingest leaves in [CAS.GitFileStore];
// a probe hit requires every named file to be recorded, and a fetcher
// that records none must be called with an empty list or the link step
// fails with [ErrGitFileNotStored].
//
// Requires v.FS for store I/O. v.Exec is only consulted by fetchers that
// shell out to git (e.g. the closure built by [CAS.Clone]); other
// fetchers are free to leave it unset.
func (c *CAS) FetchSource(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *CloneOptions,
	src SourceRequest,
) error {
	v.RequireFS()

	if src.Fetch == nil {
		return ErrFetchClosureRequired
	}

	if err := c.ensureStorePaths(v); err != nil {
		return err
	}

	attrs := map[string]any{
		"url":    RedactURL(src.URL),
		"scheme": src.Scheme,
	}

	maps.Copy(attrs, src.Attrs)

	tlm := telemetry.TelemeterFromContext(ctx)

	return tlm.Collect(
		ctx,
		l,
		"cas_fetch_source",
		attrs,
		func(childCtx context.Context, l log.Logger) error {
			suggestedKey, err := c.probeSource(childCtx, l, v, src)
			if err != nil {
				return err
			}

			err = c.fetchAndLink(childCtx, l, v, opts, src, suggestedKey, IngestCached)

			var missing *MissingObjectError
			if !errors.As(err, &missing) {
				return err
			}

			if c.probeMode == ProbeModeOffline {
				return &OfflineRepairError{Missing: missing, Source: RedactURL(src.URL)}
			}

			l.Warnf(
				"cas: store is missing object %s, re-ingesting %s to restore it",
				missing.Hash,
				RedactURL(src.URL),
			)
			RecordFallback(childCtx, l, FallbackReasonStoreRepair, map[string]any{
				"url":    RedactURL(src.URL),
				"scheme": src.Scheme,
				"hash":   missing.Hash,
			})

			// One attempt. A source that answered the re-ingest without
			// producing the object cannot produce it on a third pass
			// either, so the second failure is the one the caller sees.
			if err := c.fetchAndLink(
				childCtx, l, v, opts, src, suggestedKey, IngestRepair,
			); err != nil {
				return fmt.Errorf("re-ingest %s: %w", RedactURL(src.URL), err)
			}

			return nil
		},
	)
}

// fetchAndLink ingests src unless the store already holds the tree
// suggestedKey names, then materializes that tree into opts.Dir. Under
// [IngestRepair] the store hit is passed over, so the fetcher runs and
// writes back whatever the store has lost.
func (c *CAS) fetchAndLink(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *CloneOptions,
	src SourceRequest,
	suggestedKey string,
	mode IngestMode,
) error {
	if mode.trustsStoreHits() && suggestedKey != "" && !c.needsIngest(v, suggestedKey, opts) {
		recordFetchOutcome(ctx, true)

		return c.linkStoredTree(ctx, l, v, opts, src.Scheme, suggestedKey)
	}

	recordFetchOutcome(ctx, false)

	treeKey, err := src.Fetch(ctx, l, v, suggestedKey, mode)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", RedactURL(src.URL), err)
	}

	return c.linkStoredTree(ctx, l, v, opts, src.Scheme, treeKey)
}

// ContentKey derives a cache key for a probe token that is a content
// hash of the source bytes (S3 x-amz-checksum-sha256, GCS md5Hash, Hg
// node hash, ...). The scheme and URL drop out so identical bytes at
// different URLs share one entry.
func ContentKey(alg, token string) string {
	h := sha256.New()
	h.Write([]byte("content\x00"))
	h.Write([]byte(alg))
	h.Write([]byte{0})
	h.Write([]byte(token))

	return hex.EncodeToString(h.Sum(nil))
}

// OpaqueKey derives a URL-scoped cache key for a probe token that is
// not a content hash (ETag, Last-Modified). The token alone does not
// identify the bytes, so the scheme and URL stay in the key.
func OpaqueKey(scheme, url, token string) string {
	h := sha256.New()
	h.Write([]byte("source\x00"))
	h.Write([]byte(scheme))
	h.Write([]byte{0})
	h.Write([]byte(url))
	h.Write([]byte{0})
	h.Write([]byte(token))

	return hex.EncodeToString(h.Sum(nil))
}

// MakeFetchTempDir creates a scratch directory for a [SourceFetcher] and
// returns the path with a cleanup closure that logs failures rather than
// returning them. Exported so out-of-package [SourceFetcher] implementations
// share the same temp-dir layout.
//
// Requires v.FS.
func (c *CAS) MakeFetchTempDir(l log.Logger, v *venv.Venv) (string, func(), error) {
	v.RequireFS()

	tempDir, err := vfs.MkdirTemp(v.FS, "", "terragrunt-cas-fetch-")
	if err != nil {
		return "", nil, fmt.Errorf("create source fetch dir: %w", err)
	}

	cleanup := func() {
		if rmErr := v.FS.RemoveAll(tempDir); rmErr != nil {
			l.Warnf("cleanup error for %s: %v", tempDir, rmErr)
		}
	}

	return tempDir, cleanup, nil
}

// IngestDirectory hashes sourceDir under [DefaultLocalHashAlgorithm] and
// stores the tree and blobs in CAS. The returned tree key is suggestedKey
// when non-empty (probe-derived); otherwise it is the content hash of the
// tree. Exported so out-of-package [SourceFetcher] implementations ingest
// through the same path the local-source flow uses.
//
// Requires v.FS.
func (c *CAS) IngestDirectory(
	l log.Logger,
	v *venv.Venv,
	sourceDir, suggestedKey string,
) (string, error) {
	v.RequireFS()

	hash, treeData, err := c.buildLocalTree(v, sourceDir, DefaultLocalHashAlgorithm)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", sourceDir, err)
	}

	treeKey := suggestedKey
	if treeKey == "" {
		treeKey = hash
	}

	if err := c.storeFetchedContent(
		l,
		v,
		sourceDir,
		treeKey,
		treeData,
		DefaultLocalHashAlgorithm,
	); err != nil {
		return "", fmt.Errorf("store %s: %w", sourceDir, err)
	}

	return treeKey, nil
}

// probeSource invokes the resolver and returns its cache key, or empty
// when no resolver is configured or the probe failed, stamping where the
// answer came from on the cas_fetch_source span this runs under. See
// [SourceResolver.Probe] for the fallback contract. Two probe errors are
// returned rather than absorbed: an offline miss, because the fallback
// fetch would contact the network the user forbade, and a caller whose
// context has ended, because the fallback would start work nobody is
// waiting for.
func (c *CAS) probeSource(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	src SourceRequest,
) (string, error) {
	if src.Resolver == nil {
		return "", nil
	}

	probeCtx, origin := withProbeOriginSink(ctx)

	key, err := c.resolveProbe(probeCtx, l, v, src)
	if err != nil {
		if errors.Is(err, ErrCASOffline) || ctx.Err() != nil {
			return "", err
		}

		// ErrNoVersionMetadata is the resolver's documented "no cheap
		// signal" answer, not a degradation, so only real probe errors
		// count as fallbacks.
		if !errors.Is(err, ErrNoVersionMetadata) {
			l.Debugf(
				"cas: source probe for %s failed (falling back to content hash): %v",
				RedactURL(src.URL),
				err,
			)
			RecordFallback(ctx, l, FallbackReasonProbeFailure, map[string]any{
				"url":    RedactURL(src.URL),
				"scheme": src.Scheme,
			})
		}

		return "", nil
	}

	origin.stamp(ctx)

	return key, nil
}

// resolveProbe runs the resolver's probe, coalescing it across every
// caller in the process and answering from the persisted cache when a
// recorded answer is still fresh, so a run over many units pointing at one
// source costs one request rather than one per unit. A resolver that owns
// its cache is called directly and does all of that itself.
func (c *CAS) resolveProbe(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	src SourceRequest,
) (string, error) {
	if src.ProbeCaching == ProbeCachedByResolver {
		return src.Resolver.Probe(ctx, src.URL)
	}

	ref := schemeProbeRef(src.Resolver.Scheme())

	res, err := flights.probe.do(
		ctx,
		probeFlightKey(c.probeCache.RootPath(), src.URL, ref),
		func(ctx context.Context) (probeResult, error) {
			return c.probeSourceUncoalesced(ctx, l, v, src, ref)
		},
	)
	if err != nil {
		return "", err
	}

	recordProbeOrigin(ctx, res.origin)

	return res.key, nil
}

// probeSourceUncoalesced answers one source probe from the persisted
// cache or, when the mode allows it, from the resolver, recording a fresh
// answer for later processes.
func (c *CAS) probeSourceUncoalesced(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	src SourceRequest,
	ref string,
) (probeResult, error) {
	if entry, ok := c.cachedSourceProbe(v, src.URL, ref); ok {
		return probeResult{key: entry.Key, origin: probeOriginProbeCache}, nil
	}

	if c.probeMode == ProbeModeOffline {
		return probeResult{}, &OfflineMissError{Source: RedactURL(src.URL)}
	}

	key, err := src.Resolver.Probe(ctx, src.URL)
	if err != nil {
		return probeResult{}, err
	}

	c.recordSourceProbe(l, v, src, ref, key)

	return probeResult{key: key, origin: probeOriginResolver}, nil
}

// cachedSourceProbe returns the recorded answer for (url, ref) when the
// mode allows serving one and it has not aged past its TTL.
func (c *CAS) cachedSourceProbe(v *venv.Venv, url, ref string) (ProbeEntry, bool) {
	if !c.probeCacheEnabled || c.probeMode == ProbeModeRefresh {
		return ProbeEntry{}, false
	}

	entry, ok := c.probeCache.Lookup(v.FS, url, ref)
	if !ok {
		return ProbeEntry{}, false
	}

	if c.probeMode == ProbeModeOffline {
		return entry, true
	}

	ttl := ProbeTTL(&entry, c.probeTTL)
	if ttl <= 0 || time.Since(entry.ProbedAt) >= ttl {
		return ProbeEntry{}, false
	}

	return entry, true
}

// recordSourceProbe files key as the answer for (url, ref). A write
// failure only costs the next run a probe, so it is logged rather than
// returned.
func (c *CAS) recordSourceProbe(l log.Logger, v *venv.Venv, src SourceRequest, ref, key string) {
	if !c.probeCacheEnabled {
		return
	}

	entry := ProbeEntry{
		ProbedAt:  time.Now(),
		Key:       key,
		Immutable: src.Resolver.Pinned(src.URL),
	}

	if err := c.probeCache.Store(v.FS, src.URL, ref, &entry); err != nil {
		l.Debugf("cas: probe cache write for %s failed: %v", RedactURL(src.URL), err)
	}
}

// recordFetchOutcome stamps cache_hit on the active cas_fetch_source span
// so dashboards can distinguish probe short-circuits from network fetches.
func recordFetchOutcome(ctx context.Context, cacheHit bool) {
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.SetAttributes(attribute.Bool("cache_hit", cacheHit))
}

// probeOrigin names where a probe answer came from. It travels as the
// probe_origin attribute on the cas_fetch_source span.
type probeOrigin string

const (
	// probeOriginGitStore is a pinned SHA already present in the local
	// bare repository.
	probeOriginGitStore probeOrigin = "git_store"
	// probeOriginProbeCache is a persisted ls-remote answer within its TTL
	// (or any persisted answer when offline).
	probeOriginProbeCache probeOrigin = "probe_cache"
	// probeOriginLsRemote is a network answer, whether this caller ran
	// ls-remote or joined a flight that did.
	probeOriginLsRemote probeOrigin = "ls_remote"
	// probeOriginResolver is an answer a non-git resolver produced, whether
	// this caller probed or joined a flight that did.
	probeOriginResolver probeOrigin = "resolver"
)

// probeOriginSink collects what a resolver reports about its answer.
// [CAS.probeSource] puts one in the context it probes under, so a probe
// run outside a fetch, such as the one that resolves a stack component's
// ref, reports into nothing instead of stamping probe_origin on whichever
// span its caller happens to be inside.
type probeOriginSink struct {
	origin probeOrigin
}

// probeOriginSinkKey addresses the sink a context carries.
type probeOriginSinkKey struct{}

// withProbeOriginSink returns ctx carrying a fresh sink, plus the sink.
func withProbeOriginSink(ctx context.Context) (context.Context, *probeOriginSink) {
	sink := &probeOriginSink{}

	return context.WithValue(ctx, probeOriginSinkKey{}, sink), sink
}

// recordProbeOrigin reports where a probe answer came from to the sink
// ctx carries, if it carries one.
func recordProbeOrigin(ctx context.Context, origin probeOrigin) {
	sink, ok := ctx.Value(probeOriginSinkKey{}).(*probeOriginSink)
	if !ok {
		return
	}

	sink.origin = origin
}

// stamp puts the reported origin on the active cas_fetch_source span so
// dashboards can tell cached probes from network ones.
func (s *probeOriginSink) stamp(ctx context.Context) {
	if s.origin == "" {
		return
	}

	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	span.SetAttributes(attribute.String("probe_origin", string(s.origin)))
}

// linkStoredTree materializes the tree at key into opts.Dir, then the
// files opts.IncludedGitFiles names into opts.Dir/.git. A tree ingested
// under [gitScheme] is read back through [dropGitDirEntries], which is
// where a .git entry is residue rather than content.
func (c *CAS) linkStoredTree(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *CloneOptions,
	scheme, key string,
) error {
	treeContent := NewContent(c.treeStore)

	treeData, err := treeContent.Read(v, key)
	if err != nil {
		return fmt.Errorf("read cached tree %s: %w", key, err)
	}

	if scheme == gitScheme {
		treeData = dropGitDirEntries(treeData)
	}

	tree, err := git.ParseTree(treeData, opts.Dir)
	if err != nil {
		return fmt.Errorf("parse cached tree %s: %w", key, err)
	}

	var linkOpts []LinkTreeOption

	if opts.LinkMode != nil {
		linkOpts = append(linkOpts, WithTreeLinkMode(*opts.LinkMode))
	}

	if opts.Mutable {
		linkOpts = append(linkOpts, WithForceCopy())
	}

	linkOpts = c.linkTreeOptions(linkOpts)

	if err := LinkTree(ctx, l, v, c.blobStore, c.treeStore, tree, opts.Dir, linkOpts...); err != nil {
		return err
	}

	return c.linkIncludedGitFiles(ctx, l, v, opts, key, linkOpts)
}

// gitDirEntryPrefix is the tab that opens a tree line's path field
// followed by the directory a git repository keeps its own state in.
var gitDirEntryPrefix = []byte("\t.git/")

// dropGitDirEntries returns data without the lines naming a path under
// .git. Git refuses to record such a path, so the only way one reaches a
// tree ingested from a repository is a store written before the files
// [CloneOptions.IncludedGitFiles] names moved to records of their own:
// those releases folded the list one caller asked for into the commit's
// tree, where every later caller against the same store inherits it.
// Dropping the lines on read serves each caller its own list without
// discarding the cached commit.
func dropGitDirEntries(data []byte) []byte {
	if !bytes.Contains(data, gitDirEntryPrefix) {
		return data
	}

	kept := make([]byte, 0, len(data))

	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		if len(line) == 0 {
			continue
		}

		if tab := bytes.IndexByte(line, '\t'); tab >= 0 &&
			bytes.HasPrefix(line[tab:], gitDirEntryPrefix) {
			continue
		}

		kept = append(kept, line...)
		kept = append(kept, '\n')
	}

	return kept
}

// linkIncludedGitFiles assembles the records for opts.IncludedGitFiles
// against key into one tree and links it into opts.Dir/.git. A missing
// record surfaces as [ErrGitFileNotStored]: the caller asked for a file
// the ingest never recorded, and writing a partial .git directory would
// hide that.
func (c *CAS) linkIncludedGitFiles(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	opts *CloneOptions,
	key string,
	linkOpts []LinkTreeOption,
) error {
	if len(opts.IncludedGitFiles) == 0 {
		return nil
	}

	recordContent := NewContent(c.gitFileStore)

	var treeData []byte

	for _, name := range opts.IncludedGitFiles {
		record, err := recordContent.Read(v, GitFileKey(key, name))
		if err != nil {
			// Content.Read reports a missing record as a
			// [MissingObjectError], the miss the repair path re-ingests
			// for. A record is different: it is the ingest's own bookkeeping,
			// and its absence means the fetcher never wrote it, so it
			// surfaces as [ErrGitFileNotStored] and no repair runs.
			if errors.Is(err, fs.ErrNotExist) {
				err = errors.Join(ErrGitFileNotStored, fs.ErrNotExist)
			}

			return &WrappedError{
				Op:      "link_git_file",
				Path:    name,
				Context: key,
				Err:     err,
			}
		}

		treeData = append(treeData, record...)
	}

	gitDir := filepath.Join(opts.Dir, util.GitDir)

	tree, err := git.ParseTree(treeData, gitDir)
	if err != nil {
		return fmt.Errorf("parse git file records for %s: %w", key, err)
	}

	return LinkTree(ctx, l, v, c.blobStore, c.treeStore, tree, gitDir, linkOpts...)
}

// storeFetchedContent stores every blob referenced by the tree, then
// the tree object itself, under treeKey. Order matters: a racing
// reader that sees the tree must find every referenced blob. Writing
// the tree last means a treeStore.NeedsWrite hit implies the blobs
// are already present.
//
// Symlink entries are stored as blobs whose content is the link target
// string, matching git's representation. [hashLocalEntry] validates the
// target stays inside sourceDir so the CAS never persists an escape.
//
// treeKey is either a probe-derived key or, when no probe applies, the
// content hash from buildLocalTree.
func (c *CAS) storeFetchedContent(
	l log.Logger,
	v *venv.Venv,
	sourceDir, treeKey string,
	treeData []byte,
	alg HashAlgorithm,
) error {
	blobContent := NewContent(c.blobStore)

	walkErr := vfs.WalkDir(v.FS, sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if ignoredSourceEntry(sourceDir, path, d) {
			return dropIgnoredSourceEntry(d)
		}

		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", path, err)
		}

		mode, blobHash, err := hashLocalEntry(v.FS, sourceDir, path, info, alg)
		if err != nil {
			return err
		}

		switch mode {
		case "":
			return nil
		case gitSymlinkMode:
			target, err := vfs.Readlink(v.FS, path)
			if err != nil {
				return fmt.Errorf("read symlink %s: %w", path, err)
			}

			if err := blobContent.Ensure(l, v, blobHash, []byte(target), StoredFilePerms); err != nil {
				return fmt.Errorf("store symlink blob %s: %w", path, err)
			}
		default:
			if err := blobContent.EnsureCopy(l, v, blobHash, path); err != nil {
				return fmt.Errorf("store blob %s: %w", path, err)
			}
		}

		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	treeContent := NewContent(c.treeStore)
	if err := treeContent.EnsureWithWait(l, v, treeKey, treeData, StoredFilePerms); err != nil {
		return fmt.Errorf("store tree %s: %w", treeKey, err)
	}

	return nil
}
