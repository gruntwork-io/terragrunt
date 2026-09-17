package cas

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	probeTagsFileName    = "tags.json"
	probeTagsMaxRecorded = 256
	probeTagsMaxNameLen  = 128
	// probeTagsMaxEntrySize bounds how much of a tags entry is read. A tag
	// with a SHA-256 hash and the longest name recorded encodes to 214 bytes
	// with its separator, so an entry holding the most tags recorded stays
	// under 54 KiB.
	probeTagsMaxEntrySize = 64 << 10
	refsTagsPrefix        = "refs/tags/"
)

type probeTagsEntry struct {
	Tags []probeTag `json:"tags"`
}

type probeTag struct {
	Hash string `json:"hash"`
	Name string `json:"name"`
}

// StoreTags records the release tags among refs, as [git.ReleaseTags] picks
// them, as the tags of url. It keeps a bounded number of the highest, each with
// a full object name and a bounded name length. The entry is written to a
// temporary file and renamed into place.
func (p *ProbeCache) StoreTags(fsys vfs.FS, url string, refs []git.LsRemoteResult) error {
	var entry probeTagsEntry

	for _, ref := range git.ReleaseTags(refs) {
		if len(entry.Tags) == probeTagsMaxRecorded {
			break
		}

		if tag, ok := recordableTag(ref); ok {
			entry.Tags = append(entry.Tags, tag)
		}
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode tags entry for %s: %w", RedactURL(url), err)
	}

	path := p.TagsPath(url)
	dir := filepath.Dir(path)

	if err := fsys.MkdirAll(dir, DefaultDirPerms); err != nil {
		return fmt.Errorf("create probe cache dir %s: %w", dir, err)
	}

	if err := vfs.WriteFileAtomic(fsys, path, data, RegularFilePerms); err != nil {
		return fmt.Errorf("write tags entry %s: %w", path, err)
	}

	return nil
}

// LookupTags returns the tags [ProbeCache.StoreTags] recorded for url, in the
// shape [git.GitRunner.LsRemote] reports them. It returns none when nothing was
// recorded or the entry cannot be read or decoded.
func (p *ProbeCache) LookupTags(fsys vfs.FS, url string) []git.LsRemoteResult {
	data, err := vfs.ReadFileLimit(fsys, p.TagsPath(url), probeTagsMaxEntrySize)
	if err != nil {
		return nil
	}

	var entry probeTagsEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}

	refs := make([]git.LsRemoteResult, 0, len(entry.Tags))

	for _, tag := range entry.Tags {
		ref := git.LsRemoteResult{Hash: tag.Hash, Ref: refsTagsPrefix + tag.Name}
		if _, ok := recordableTag(ref); ok {
			refs = append(refs, ref)
		}
	}

	return refs
}

// TagsPath returns the file [ProbeCache.StoreTags] records the tags of url in,
// in the directory that holds the entries [ProbeCache.EntryPath] names for url.
func (p *ProbeCache) TagsPath(url string) string {
	return filepath.Join(filepath.Dir(p.EntryPath(url, "")), probeTagsFileName)
}

// StoredTags lists the tags in the bare repository for url, and none when the
// store holds no repository for url. Like [GitStore.ProbeCachedCommit], it
// takes no lock.
//
// Panics when v.FS is not OS-backed; git only sees the real disk.
func (s *GitStore) StoredTags(ctx context.Context, v *venv.Venv, url string) ([]git.LsRemoteResult, error) {
	if !vfs.IsOSFS(v.FS) {
		panic(ErrGitStoreFSNotOS)
	}

	_, repoPath, _ := s.repoPaths(url)

	initialized, err := bareRepoInitialized(v.FS, repoPath)
	if err != nil {
		return nil, err
	}

	if !initialized {
		return nil, nil
	}

	runner, err := git.NewGitRunner(v)
	if err != nil {
		return nil, err
	}

	return runner.WithWorkDir(repoPath).LocalTags(ctx)
}

// RecordTags records refs, a tag listing of url's remote, so [CAS.StoredTags]
// can answer for url offline. It records nothing unless the probe cache is
// enabled, and logs a failed write.
func (c *CAS) RecordTags(l log.Logger, v *venv.Venv, url string, refs []git.LsRemoteResult) {
	if !c.probeCacheEnabled {
		return
	}

	if err := c.probeCache.StoreTags(v.FS, url, refs); err != nil {
		l.Debugf("cas: tag cache write for %s failed: %v", RedactURL(url), err)
	}
}

// StoredTags returns the tags of url the store holds, without contacting its
// remote: those [CAS.RecordTags] recorded, and those in the bare repository
// the git store keeps for url. A bare repository that cannot be listed adds
// none.
//
// Panics when v.FS is not OS-backed; git only sees the real disk.
func (c *CAS) StoredTags(ctx context.Context, l log.Logger, v *venv.Venv, url string) []git.LsRemoteResult {
	var refs []git.LsRemoteResult

	if c.probeCacheEnabled {
		refs = c.probeCache.LookupTags(v.FS, url)
	}

	stored, err := c.gitStore.StoredTags(ctx, v, url)
	if err != nil {
		l.Debugf("cas: listing tags stored for %s failed: %v", RedactURL(url), err)

		return refs
	}

	return append(refs, stored...)
}

// recordableTag returns ref as a recorded tag when its hash is a full object
// name and its tag name is at most [probeTagsMaxNameLen] bytes.
func recordableTag(ref git.LsRemoteResult) (probeTag, bool) {
	name, isTag := strings.CutPrefix(ref.Ref, refsTagsPrefix)
	if !isTag || !looksLikeFullSHA(ref.Hash) || len(name) > probeTagsMaxNameLen {
		return probeTag{}, false
	}

	return probeTag{Hash: ref.Hash, Name: name}, true
}
