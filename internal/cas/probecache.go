package cas

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/semver"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
)

// ProbeMode selects how [GitResolver.Probe] treats the network and the
// persisted probe cache.
type ProbeMode int

const (
	// ProbeModeOnline serves a persisted probe that is still within its
	// TTL and runs ls-remote otherwise.
	ProbeModeOnline ProbeMode = iota
	// ProbeModeRefresh never reads the persisted cache, so every probe
	// reaches the network unless it joins one already in flight. Answers
	// are still recorded for later runs.
	ProbeModeRefresh
	// ProbeModeOffline never runs ls-remote. The persisted cache answers
	// regardless of TTL and a miss fails with [OfflineMissError].
	ProbeModeOffline
)

// DefaultImmutableProbeTTL is how long a persisted probe of a source that
// cannot change upstream is served without probing again. A semver tag and
// a version-pinned object are held in place by convention, and nothing
// enforces it, so the answer is re-checked daily. [ProbeTTL] gives every
// other source the caller's mutable TTL.
const DefaultImmutableProbeTTL = 24 * time.Hour

// probeDigestAlgorithm hashes every name the probe cache derives and names
// the directory every entry is filed under, so the two cannot drift.
const probeDigestAlgorithm = HashSHA256

const (
	probeCacheDirName = "probes"
	probeCacheKeyLen  = 16
	// probeCacheMaxEntrySize bounds how much of an entry file is read. A
	// well-formed entry measures 189 bytes with a SHA-1 commit hash and 213
	// with a SHA-256 one, plus up to 10 more for a timestamp carrying
	// fractional seconds. Anything approaching this bound is damage and is
	// read as a miss.
	probeCacheMaxEntrySize = 512
)

// ProbeEntry is one persisted probe answer. Every field is fixed width,
// so an entry is a couple of hundred bytes whatever the source was called
// and nothing a remote or a configuration chose is written verbatim.
type ProbeEntry struct {
	// ProbedAt is when the probe returned Key.
	ProbedAt time.Time `json:"probed_at"`
	// RefDigest confirms which ref the entry answers for, against the
	// shorter digest [ProbeCache.EntryPath] files it under. [ProbeCache]
	// fills it; callers leave it alone.
	RefDigest string `json:"ref_digest"`
	// Key is the cache key the probe produced: a commit hash for git, a
	// tree-store key from [ContentKey] or [OpaqueKey] for every other
	// resolver. Hex either way.
	Key string `json:"key"`
	// Immutable reports that the source this answers for cannot change
	// upstream, so the entry is served for [DefaultImmutableProbeTTL]
	// rather than the caller's mutable TTL.
	Immutable bool `json:"immutable"`
}

// ProbeCache persists what each probe resolved a source to under the CAS
// store, so a later process can skip the probe for a source whose TTL has
// not run out and so --cas-offline has something to answer from. Entries
// are partitioned per URL; see [ProbeCache.EntryPath] for how one is
// addressed.
//
// Expired entries stay on disk; nothing here removes them. An expired
// entry is still a usable one: --cas-offline serves any entry whatever
// its age, and the default mutable TTL of zero expires a branch's entry
// the moment it is written. A TTL here is a freshness question, and
// eviction is out of scope.
type ProbeCache struct {
	rootPath string
}

// NewProbeCache returns a [ProbeCache] rooted at rootPath. Directories
// are created on first write.
func NewProbeCache(rootPath string) *ProbeCache {
	return &ProbeCache{rootPath: rootPath}
}

// RootPath returns the directory that contains every per-URL entry.
func (p *ProbeCache) RootPath() string {
	return p.rootPath
}

// Lookup returns the entry recorded for (url, ref). ok is false when
// nothing was recorded or the entry cannot be read or decoded; a damaged
// entry is never fatal because the next successful probe overwrites it.
func (p *ProbeCache) Lookup(fsys vfs.FS, url, ref string) (ProbeEntry, bool) {
	data, err := vfs.ReadFileLimit(fsys, p.EntryPath(url, ref), probeCacheMaxEntrySize)
	if err != nil {
		return ProbeEntry{}, false
	}

	var entry ProbeEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return ProbeEntry{}, false
	}

	// The path digest is truncated, so a second ref can land on one entry
	// file. The full digest here tells them apart; a mismatch is treated
	// as a miss and the next successful probe overwrites the entry.
	if entry.RefDigest != RefDigest(ref) || entry.Key == "" {
		return ProbeEntry{}, false
	}

	return entry, true
}

// Store records entry as the answer for (url, ref), stamping its
// timestamp and ref digest in place first. The entry is written to a
// temporary file and renamed into place, so a concurrent reader sees
// either the previous entry or this one, never a torn write.
func (p *ProbeCache) Store(fsys vfs.FS, url, ref string, entry *ProbeEntry) error {
	path := p.EntryPath(url, ref)
	dir := filepath.Dir(path)

	if err := fsys.MkdirAll(dir, DefaultDirPerms); err != nil {
		return fmt.Errorf("create probe cache dir %s: %w", dir, err)
	}

	entry.ProbedAt = entry.ProbedAt.UTC()
	entry.RefDigest = RefDigest(ref)

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode probe entry for %s: %w", path, err)
	}

	tmp, err := vfs.CreateTemp(fsys, dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create probe cache temp file in %s: %w", dir, err)
	}

	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write %s: %w", tmpPath, err), tmp.Close(), fsys.Remove(tmpPath))
	}

	if err := tmp.Close(); err != nil {
		return errors.Join(fmt.Errorf("close %s: %w", tmpPath, err), fsys.Remove(tmpPath))
	}

	if err := fsys.Rename(tmpPath, path); err != nil {
		return errors.Join(fmt.Errorf("rename %s to %s: %w", tmpPath, path, err), fsys.Remove(tmpPath))
	}

	return nil
}

// EntryPath returns the file the answer for (url, ref) is recorded in:
// <root>/<algorithm>/<url hash>/<ref hash>.json. Both hashed components
// are hashed because a URL carries characters that are not path-safe and a
// ref may contain slashes. The URL is redacted first, so a token rotated
// between runs still finds the answer recorded for the repository it
// addresses instead of leaving an entry nothing reads again.
//
// The algorithm names the directory rather than prefixing each file, which
// keeps the names legal on Windows, where a colon cannot appear in a path.
// A later algorithm gets its own subtree, so entries never collide across
// the two and old ones stay identifiable.
func (p *ProbeCache) EntryPath(url, ref string) string {
	name := RefDigest(ref)[:probeCacheKeyLen] + ".json"
	root := filepath.Join(p.rootPath, string(probeDigestAlgorithm))

	return filepath.Join(EntryPathForURL(root, RedactURL(url), probeDigestAlgorithm), name)
}

// RefDigest returns the digest an entry records for ref and the name
// [ProbeCache.EntryPath] truncates for its file. It carries no algorithm
// label of its own: the directory the entry sits in names the algorithm.
// Exported so tests can build an entry the cache will accept.
func RefDigest(ref string) string {
	return probeDigestAlgorithm.Sum([]byte(probeRefName(ref)))
}

// ProbeTTL returns how long entry stays fresh: [DefaultImmutableProbeTTL]
// when it records an immutable source, mutableTTL for everything else
// (git branches and non-version tags, and every object or endpoint whose
// URL does not pin a version).
func ProbeTTL(entry *ProbeEntry, mutableTTL time.Duration) time.Duration {
	if entry.Immutable {
		return DefaultImmutableProbeTTL
	}

	return mutableTTL
}

// schemeProbeRef namespaces the entries of a resolver whose URL is the
// whole identity of a source, so they stay clear of the git entries for
// the same URL. A git ref cannot contain a colon, so no ref reaches this
// namespace.
func schemeProbeRef(scheme string) string {
	return "scheme:" + scheme
}

// IsSemverTag reports whether ref is a full tag name (refs/tags/...) for
// a semantic version, with or without a leading "v". A bare name such as
// v1.2.3 does not qualify: ls-remote lists a branch of that name ahead
// of the tag, so only the resolved ref says which one answered.
func IsSemverTag(ref string) bool {
	name, isTag := strings.CutPrefix(ref, "refs/tags/")
	if !isTag {
		return false
	}

	return semver.IsExact(name)
}

// probeRefName maps the empty ref to the name ls-remote resolves it as,
// so an empty branch and an explicit HEAD share one cache entry.
func probeRefName(ref string) string {
	if ref == "" {
		return "HEAD"
	}

	return ref
}
