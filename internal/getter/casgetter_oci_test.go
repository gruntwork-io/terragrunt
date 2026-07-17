package getter_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCASGetterOCIMovedTagNeverPoisonsProbeKey pins the probe/fetch race fix:
// a tag that moves during the download must not store the fetched bytes under
// the pre-move probe key, or a digest-pinned request for that key would
// silently receive the moved content.
func TestCASGetterOCIMovedTagNeverPoisonsProbeKey(t *testing.T) {
	t.Parallel()

	keyA := tgcas.ContentKey("oci-manifest", "sha256:aaaa")
	keyB := tgcas.ContentKey("oci-manifest", "sha256:bbbb")

	resolver := &movingTagResolver{tagKey: keyA, digestKey: keyA}
	// The first download re-pushes the tag mid-fetch.
	fetcher := &countingModuleGetter{onFirstGet: func() { resolver.setTag(keyB) }}

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithGenericFetchers(map[string]gogetter.Getter{
			getter.SchemeOCI: fetcher,
		}),
		getter.WithGenericResolvers(map[string]getter.SourceResolver{
			getter.SchemeOCI: resolver,
		}),
	)
	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	tagSrc := "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0"
	digestSrc := "oci://127.0.0.1:5000/terraform-modules/vpc?digest=sha256:aaaa"

	// The tag moves from A to B while this download runs, so the fetched
	// bytes must not land under key A.
	dstOne := filepath.Join(t.TempDir(), "one")

	_, err = client.Get(t.Context(), &gogetter.Request{Src: tagSrc, Dst: dstOne, GetMode: gogetter.ModeDir})
	require.NoError(t, err)
	require.EqualValues(t, 1, fetcher.gets.Load())

	// A digest-pinned request for A must re-fetch: a hit here would mean the
	// moved tag poisoned key A with the wrong content.
	dstTwo := filepath.Join(t.TempDir(), "two")

	_, err = client.Get(t.Context(), &gogetter.Request{Src: digestSrc, Dst: dstTwo, GetMode: gogetter.ModeDir})
	require.NoError(t, err)
	require.EqualValues(t, 2, fetcher.gets.Load(), "a hit would mean the moved tag poisoned the digest key")

	// The digest fetch keyed A legitimately, so a repeat is a pure cache hit.
	dstThree := filepath.Join(t.TempDir(), "three")

	_, err = client.Get(t.Context(), &gogetter.Request{Src: digestSrc, Dst: dstThree, GetMode: gogetter.ModeDir})
	require.NoError(t, err)
	assert.EqualValues(t, 2, fetcher.gets.Load(), "a stable digest pin must be served from CAS")
	assert.FileExists(t, filepath.Join(dstThree, "main.tf"))
}

// TestCASGetterOCIReProbeFailureDropsSuggestedKey pins the guard's error
// path: when the post-fetch re-probe fails, the fetched bytes must not be
// stored under the pre-fetch probe key.
func TestCASGetterOCIReProbeFailureDropsSuggestedKey(t *testing.T) {
	t.Parallel()

	keyA := tgcas.ContentKey("oci-manifest", "sha256:aaaa")

	resolver := &movingTagResolver{tagKey: keyA, digestKey: keyA}
	// The first download breaks tag probing mid-fetch.
	fetcher := &countingModuleGetter{onFirstGet: func() { resolver.setTagFailure() }}

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{},
		getter.WithGenericFetchers(map[string]gogetter.Getter{
			getter.SchemeOCI: fetcher,
		}),
		getter.WithGenericResolvers(map[string]getter.SourceResolver{
			getter.SchemeOCI: resolver,
		}),
	)
	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	dstOne := filepath.Join(t.TempDir(), "one")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dstOne,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, fetcher.gets.Load())

	// Key A must be unpopulated after the failed re-probe, so a digest pin
	// for A has to fetch instead of hitting unverifiable content.
	dstTwo := filepath.Join(t.TempDir(), "two")

	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?digest=sha256:aaaa",
		Dst:     dstTwo,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 2, fetcher.gets.Load(), "a hit would mean the failed re-probe kept the stale key")
}

// TestCASGetterDoesNotClaimOCIWithoutFetcher pins that oci:// stays unclaimed
// when the oci fetcher is not registered.
func TestCASGetterDoesNotClaimOCIWithoutFetcher(t *testing.T) {
	t.Parallel()

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(logger.CreateLogger(), c, v, &tgcas.CloneOptions{})

	req := &gogetter.Request{
		Src: "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Pwd: t.TempDir(),
	}

	// The URL falls through to the file detector and fails there, which is
	// exactly what proves the oci scheme was not claimed.
	_, err = g.Detect(req)
	require.ErrorContains(t, err, "directory not found")
	assert.NotEqual(t, getter.SchemeOCI, req.Forced, "oci must not be claimed without a registered oci fetcher")
}

// movingTagResolver models a mutable tag: digest probes are deterministic,
// tag probes follow the current tag position or fail on demand.
type movingTagResolver struct {
	tagKey    string
	digestKey string
	failTag   bool
	mu        sync.Mutex
}

func (r *movingTagResolver) Scheme() string { return getter.SchemeOCI }

func (r *movingTagResolver) Probe(_ context.Context, rawURL string) (string, error) {
	if strings.Contains(rawURL, "digest=") {
		return r.digestKey, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.failTag {
		return "", errUnknownBlob
	}

	return r.tagKey, nil
}

func (r *movingTagResolver) setTag(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tagKey = key
}

func (r *movingTagResolver) setTagFailure() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.failTag = true
}

// countingModuleGetter writes a one-file module, counts downloads, and runs
// onFirstGet during the first download to simulate a mid-fetch re-push.
type countingModuleGetter struct {
	onFirstGet func()
	gets       atomic.Int32
}

func (f *countingModuleGetter) Get(_ context.Context, req *gogetter.Request) error {
	if f.gets.Add(1) == 1 && f.onFirstGet != nil {
		f.onFirstGet()
	}

	return os.WriteFile(filepath.Join(req.Dst, "main.tf"), []byte(`output "fetched" {}`), 0o644)
}

func (f *countingModuleGetter) GetFile(_ context.Context, _ *gogetter.Request) error {
	return getter.ErrOCIGetFileUnsupported
}

func (f *countingModuleGetter) Mode(_ context.Context, _ *url.URL) (gogetter.Mode, error) {
	return gogetter.ModeDir, nil
}

func (f *countingModuleGetter) Detect(_ *gogetter.Request) (bool, error) {
	return true, nil
}
