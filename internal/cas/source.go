package cas

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"maps"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/telemetry"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

	// Probe returns a cache key for rawURL.
	//
	// Returns ErrNoVersionMetadata when the source has no cheap
	// signal; FetchSource then falls back to downloading and
	// content-hashing. Other errors are logged and treated the same
	// way, so a misconfigured probe never breaks a fetch.
	Probe(ctx context.Context, rawURL string) (cacheKey string, err error)
}

// SourceFetcher downloads and ingests a source into CAS, returning the
// tree-store key the materialized tree was written under.
//
// suggestedKey is the probe-derived cache key, or empty when the probe
// produced none. Fetchers that learn the canonical key only after
// downloading (the git rev-parse path) may ignore it and return the
// canonical key instead.
type SourceFetcher func(ctx context.Context, l log.Logger, v Venv, suggestedKey string) (treeKey string, err error)

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
}

// FetchSource routes src through the CAS. On a probe hit it links the
// cached tree into opts.Dir without invoking Fetch. On a probe miss it
// calls Fetch and links the resulting tree.
//
// opts.Dir is the destination. opts.Mutable selects copy vs hardlink
// for the final link, matching the git path.
func (c *CAS) FetchSource(
	ctx context.Context,
	l log.Logger,
	v Venv,
	opts *CloneOptions,
	src SourceRequest,
) error {
	if src.Fetch == nil {
		return ErrFetchClosureRequired
	}

	if err := c.ensureStorePaths(v); err != nil {
		return err
	}

	attrs := map[string]any{
		"url":    src.URL,
		"scheme": src.Scheme,
	}

	maps.Copy(attrs, src.Attrs)

	tlm := telemetry.TelemeterFromContext(ctx)

	return tlm.Collect(ctx, "cas_fetch_source", attrs, func(childCtx context.Context) error {
		suggestedKey := c.probeSource(childCtx, l, src)

		if suggestedKey != "" && !c.treeStore.NeedsWrite(v, suggestedKey) {
			recordFetchOutcome(childCtx, true)

			return c.linkStoredTree(childCtx, v, opts, suggestedKey)
		}

		recordFetchOutcome(childCtx, false)

		treeKey, err := src.Fetch(childCtx, l, v, suggestedKey)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", src.URL, err)
		}

		return c.linkStoredTree(childCtx, v, opts, treeKey)
	})
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
func (c *CAS) MakeFetchTempDir(l log.Logger, v Venv) (string, func(), error) {
	tempDir, err := vfs.MkdirTemp(v.FS, "", "terragrunt-cas-fetch-*")
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
func (c *CAS) IngestDirectory(l log.Logger, v Venv, sourceDir, suggestedKey string) (string, error) {
	hash, treeData, err := c.buildLocalTree(v, sourceDir, DefaultLocalHashAlgorithm)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", sourceDir, err)
	}

	treeKey := suggestedKey
	if treeKey == "" {
		treeKey = hash
	}

	if err := c.storeFetchedContent(l, v, sourceDir, treeKey, treeData, DefaultLocalHashAlgorithm); err != nil {
		return "", fmt.Errorf("store %s: %w", sourceDir, err)
	}

	return treeKey, nil
}

// probeSource invokes the resolver and returns its cache key, or empty
// when no resolver is configured or the probe failed. See
// [SourceResolver.Probe] for the fallback contract.
func (c *CAS) probeSource(ctx context.Context, l log.Logger, src SourceRequest) string {
	if src.Resolver == nil {
		return ""
	}

	key, err := src.Resolver.Probe(ctx, src.URL)
	if err != nil {
		if !errors.Is(err, ErrNoVersionMetadata) {
			l.Debugf("cas: source probe for %s failed (falling back to content hash): %v", src.URL, err)
		}

		return ""
	}

	return key
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

// linkStoredTree materializes the tree at key into opts.Dir.
func (c *CAS) linkStoredTree(ctx context.Context, v Venv, opts *CloneOptions, key string) error {
	treeContent := NewContent(c.treeStore)

	treeData, err := treeContent.Read(v, key)
	if err != nil {
		return fmt.Errorf("read cached tree %s: %w", key, err)
	}

	tree, err := git.ParseTree(treeData, opts.Dir)
	if err != nil {
		return fmt.Errorf("parse cached tree %s: %w", key, err)
	}

	var linkOpts []LinkTreeOption
	if opts.Mutable {
		linkOpts = append(linkOpts, WithForceCopy())
	}

	return LinkTree(ctx, v, c.blobStore, c.treeStore, tree, opts.Dir, linkOpts...)
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
	v Venv,
	sourceDir, treeKey string,
	treeData []byte,
	alg HashAlgorithm,
) error {
	blobContent := NewContent(c.blobStore)

	walkErr := vfs.WalkDir(v.FS, sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
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

			if err := blobContent.Ensure(l, v, blobHash, []byte(target)); err != nil {
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
	if err := treeContent.EnsureWithWait(l, v, treeKey, treeData); err != nil {
		return fmt.Errorf("store tree %s: %w", treeKey, err)
	}

	return nil
}
