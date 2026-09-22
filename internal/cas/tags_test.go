package cas_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/redact"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// TestProbeCache_StoreTagsKeepsReleaseTagsHighestFirst pins that a tags entry
// holds only the release tags of a listing, highest first, and drops a tag
// whose hash is not a full object name.
func TestProbeCache_StoreTagsKeepsReleaseTagsHighestFirst(t *testing.T) {
	t.Parallel()

	fsys := venvtest.New().FS
	p := cas.NewProbeCache(probeCacheRoot)
	url := "https://example.com/tags.git"

	require.NoError(t, p.StoreTags(fsys, redact.NewURL(url), []git.LsRemoteResult{
		{Hash: testTagHash(1), Ref: "refs/tags/v1.2.0"},
		{Hash: testTagHash(2), Ref: "refs/tags/v1.10.0"},
		{Hash: testTagHash(3), Ref: "refs/tags/v1.10.0^{}"},
		{Hash: testTagHash(4), Ref: "refs/tags/v2.0.0-rc1"},
		{Hash: testTagHash(5), Ref: "refs/tags/not-semver"},
		{Hash: "abc123", Ref: "refs/tags/v3.0.0"},
	}))

	assert.Equal(t, []git.LsRemoteResult{
		{Hash: testTagHash(2), Ref: "refs/tags/v1.10.0"},
		{Hash: testTagHash(1), Ref: "refs/tags/v1.2.0"},
	}, p.LookupTags(fsys, redact.NewURL(url)))
	assert.Empty(t, p.LookupTags(fsys, redact.NewURL("https://example.com/other.git")))
}

// TestProbeCache_StoreTagsBoundsEntry pins that a listing with more release
// tags than an entry keeps is recorded with its highest tags.
func TestProbeCache_StoreTagsBoundsEntry(t *testing.T) {
	t.Parallel()

	fsys := venvtest.New().FS
	p := cas.NewProbeCache(probeCacheRoot)
	url := "https://example.com/many-tags.git"

	refs := make([]git.LsRemoteResult, 0, 300)
	for i := range 300 {
		refs = append(refs, git.LsRemoteResult{Hash: testTagHash(i), Ref: fmt.Sprintf("refs/tags/v1.%d.0", i)})
	}

	require.NoError(t, p.StoreTags(fsys, redact.NewURL(url), refs))

	got := p.LookupTags(fsys, redact.NewURL(url))
	require.Len(t, got, 256)
	assert.Equal(t, "refs/tags/v1.299.0", got[0].Ref)
	assert.Equal(t, "refs/tags/v1.44.0", got[len(got)-1].Ref)
}

// TestProbeCache_LookupTagsDamagedEntry pins that an entry that does not
// decode is read as no tags.
func TestProbeCache_LookupTagsDamagedEntry(t *testing.T) {
	t.Parallel()

	fsys := venvtest.New().FS
	p := cas.NewProbeCache(probeCacheRoot)
	url := "https://example.com/damaged-tags.git"

	path := p.TagsPath(redact.NewURL(url))
	require.NoError(t, vfs.WriteFileAtomic(fsys, path, []byte(`{"tags":[`), cas.RegularFilePerms))

	assert.Empty(t, p.LookupTags(fsys, redact.NewURL(url)))
}

// TestCAS_StoredTagsServesRecordedTags pins that tags recorded by one process
// are served by a later offline one, and that nothing is recorded without the
// probe cache.
func TestCAS_StoredTagsServesRecordedTags(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	v := venvtest.NewOSWithEmptyEnv()
	url := "https://example.com/recorded-tags.git"
	refs := []git.LsRemoteResult{{Hash: testTagHash(7), Ref: "refs/tags/v1.4.0"}}

	uncachedDir := t.TempDir()

	uncached, err := cas.New(v, cas.WithStorePath(uncachedDir))
	require.NoError(t, err)

	uncached.RecordTags(l, v, redact.NewURL(url), refs)

	offlineUncached, err := cas.New(v, cas.WithStorePath(uncachedDir), cas.WithOffline())
	require.NoError(t, err)
	assert.Empty(t, offlineUncached.StoredTags(t.Context(), l, v, redact.NewURL(url)))

	cachedDir := t.TempDir()

	online, err := cas.New(v, cas.WithStorePath(cachedDir), cas.WithProbeCache())
	require.NoError(t, err)

	online.RecordTags(l, v, redact.NewURL(url), refs)

	offline, err := cas.New(v, cas.WithStorePath(cachedDir), cas.WithOffline())
	require.NoError(t, err)
	assert.Equal(t, refs, offline.StoredTags(t.Context(), l, v, redact.NewURL(url)))
}

// testTagHash returns a full SHA-1 object name distinct for each i.
func testTagHash(i int) string {
	return fmt.Sprintf("%040x", i)
}
