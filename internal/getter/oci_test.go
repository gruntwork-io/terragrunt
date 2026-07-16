package getter_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/opencontainers/go-digest"
	specs "github.com/opencontainers/image-spec/specs-go"
	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errUnknownBlob = errors.New("unknown blob digest")

func TestOCIGetterGet(t *testing.T) {
	t.Parallel()

	moduleFiles := map[string]string{
		"main.tf":       `output "root" {}`,
		"subdir/sub.tf": `output "sub" {}`,
	}
	zipBytes := moduleZipBytes(t, moduleFiles)
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)

	testCases := []struct {
		name        string
		src         string
		wantRef     string
		wantDomain  string
		wantRepo    string
		wantPresent []string
		wantAbsent  []string
	}{
		{
			name:        "tag pin",
			src:         "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
			wantRef:     "1.0.0",
			wantDomain:  "127.0.0.1:5000",
			wantRepo:    "terraform-modules/vpc",
			wantPresent: []string{"main.tf", "subdir/sub.tf"},
		},
		{
			name:        "digest pin",
			src:         "oci://127.0.0.1:5000/terraform-modules/vpc?digest=" + manifestDesc.Digest.String(),
			wantRef:     manifestDesc.Digest.String(),
			wantDomain:  "127.0.0.1:5000",
			wantRepo:    "terraform-modules/vpc",
			wantPresent: []string{"main.tf", "subdir/sub.tf"},
		},
		{
			name:        "no query defaults to latest tag",
			src:         "oci://registry.example.com/terraform-modules/vpc",
			wantRef:     "latest",
			wantDomain:  "registry.example.com",
			wantRepo:    "terraform-modules/vpc",
			wantPresent: []string{"main.tf", "subdir/sub.tf"},
		},
		{
			name:        "multi segment repository kept whole",
			src:         "oci://registry.example.com/org/team/vpc?tag=1.0.0",
			wantRef:     "1.0.0",
			wantDomain:  "registry.example.com",
			wantRepo:    "org/team/vpc",
			wantPresent: []string{"main.tf", "subdir/sub.tf"},
		},
		{
			name:        "subdir selector extracts only the subdir contents",
			src:         "oci://127.0.0.1:5000/terraform-modules/vpc//subdir?tag=1.0.0",
			wantRef:     "1.0.0",
			wantDomain:  "127.0.0.1:5000",
			wantRepo:    "terraform-modules/vpc",
			wantPresent: []string{"sub.tf"},
			wantAbsent:  []string{"main.tf", "subdir/sub.tf"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

			var gotDomain, gotRepo string

			g := newTestOCIGetter(recordingStore(store, &gotDomain, &gotRepo))

			dst := filepath.Join(t.TempDir(), "module")

			_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
				Src:     tc.src,
				Dst:     dst,
				GetMode: gogetter.ModeDir,
			})
			require.NoError(t, err)

			assert.Equal(t, tc.wantDomain, gotDomain)
			assert.Equal(t, tc.wantRepo, gotRepo)
			require.Equal(t, []string{tc.wantRef}, store.gotRefs)

			for _, name := range tc.wantPresent {
				assert.FileExists(t, filepath.Join(dst, name))
			}

			for _, name := range tc.wantAbsent {
				assert.NoFileExists(t, filepath.Join(dst, name))
			}
		})
	}
}

func TestOCIGetterGetContentMatches(t *testing.T) {
	t.Parallel()

	moduleFiles := map[string]string{"main.tf": `output "exact" {}`}
	zipBytes := moduleZipBytes(t, moduleFiles)
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)

	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)
	g := newTestOCIGetter(staticStore(store))
	dst := filepath.Join(t.TempDir(), "module")

	_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(dst, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, moduleFiles["main.tf"], string(got))
}

func TestOCIGetterGetErrors(t *testing.T) {
	t.Parallel()

	moduleFiles := map[string]string{"main.tf": `output "root" {}`}
	zipBytes := moduleZipBytes(t, moduleFiles)
	layer := zipLayerDesc(zipBytes)

	secondZipBytes := moduleZipBytes(t, map[string]string{"other.tf": `output "other" {}`})
	secondLayer := zipLayerDesc(secondZipBytes)

	goodManifest, goodDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
	wrongTypeManifest, wrongTypeDesc := manifestFor(t, "application/vnd.example.other", layer)
	noLayerManifest, noLayerDesc := manifestFor(t, getter.ArtifactTypeModulePkg)
	twoLayerManifest, twoLayerDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer, secondLayer)

	testCases := []struct {
		store     *fakeStore
		wantErrIs error
		name      string
		src       string
	}{
		{
			name:      "unsupported query parameter",
			src:       "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0&foo=bar",
			store:     newFakeStore(goodManifest, &goodDesc, zipBytes, &layer),
			wantErrIs: getter.OCIUnsupportedQueryParamError{Param: "foo"},
		},
		{
			name:      "tag and digest are mutually exclusive",
			src:       "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0&digest=" + goodDesc.Digest.String(),
			store:     newFakeStore(goodManifest, &goodDesc, zipBytes, &layer),
			wantErrIs: getter.ErrOCITagDigestExclusive,
		},
		{
			name:      "missing registry domain",
			src:       "oci:///terraform-modules/vpc?tag=1.0.0",
			store:     newFakeStore(goodManifest, &goodDesc, zipBytes, &layer),
			wantErrIs: getter.ErrOCIMissingRegistryDomain,
		},
		{
			name:      "missing repository name",
			src:       "oci://127.0.0.1:5000?tag=1.0.0",
			store:     newFakeStore(goodManifest, &goodDesc, zipBytes, &layer),
			wantErrIs: getter.ErrOCIMissingRepositoryName,
		},
		{
			name:      "artifact type rejected",
			src:       "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
			store:     newFakeStore(wrongTypeManifest, &wrongTypeDesc, zipBytes, &layer),
			wantErrIs: getter.OCIArtifactTypeError{ArtifactType: "application/vnd.example.other"},
		},
		{
			name:      "zero module zip layers",
			src:       "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
			store:     newFakeStore(noLayerManifest, &noLayerDesc, zipBytes, &layer),
			wantErrIs: getter.OCILayerCountError{Count: 0},
		},
		{
			name:      "multiple module zip layers",
			src:       "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
			store:     newFakeStore(twoLayerManifest, &twoLayerDesc, zipBytes, &layer),
			wantErrIs: getter.OCILayerCountError{Count: 2},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			g := newTestOCIGetter(staticStore(tc.store))
			dst := filepath.Join(t.TempDir(), "module")

			_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
				Src:     tc.src,
				Dst:     dst,
				GetMode: gogetter.ModeDir,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErrIs)
		})
	}
}

func TestOCIGetterGetDigestMismatch(t *testing.T) {
	t.Parallel()

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "root" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)

	corruptedBytes := append([]byte{}, zipBytes...)
	corruptedBytes[len(corruptedBytes)-1]++

	store := newFakeStore(manifestBytes, &manifestDesc, corruptedBytes, &layer)
	g := newTestOCIGetter(staticStore(store))
	dst := filepath.Join(t.TempDir(), "module")

	_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)

	var digestErr getter.OCIDigestVerificationError

	require.ErrorAs(t, err, &digestErr)
	assert.Equal(t, layer.Digest.String(), digestErr.Digest)
}

func TestOCIGetterGetNotConfigured(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		getter *getter.OCIGetter
		name   string
	}{
		{
			name:   "missing store",
			getter: &getter.OCIGetter{Logger: logger.CreateLogger(), FS: vfs.NewOSFS()},
		},
		{
			name:   "missing logger",
			getter: &getter.OCIGetter{NewStore: staticStore(nil), FS: vfs.NewOSFS()},
		},
		{
			name:   "missing filesystem",
			getter: &getter.OCIGetter{NewStore: staticStore(nil), Logger: logger.CreateLogger()},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dst := filepath.Join(t.TempDir(), "module")

			_, err := newOCITestClient(tc.getter).Get(t.Context(), &gogetter.Request{
				Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
				Dst:     dst,
				GetMode: gogetter.ModeDir,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, getter.ErrOCIGetterNotConfigured)
		})
	}
}

func TestOCIGetterGetQueryValidation(t *testing.T) {
	t.Parallel()

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "root" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)

	testCases := []struct {
		wantErrIs error
		name      string
		src       string
	}{
		{
			name:      "duplicate tag",
			src:       "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0&tag=2.0.0",
			wantErrIs: getter.OCIDuplicateQueryParamError{Param: "tag"},
		},
		{
			name: "duplicate digest",
			src: "oci://127.0.0.1:5000/terraform-modules/vpc?digest=" +
				manifestDesc.Digest.String() + "&digest=" + manifestDesc.Digest.String(),
			wantErrIs: getter.OCIDuplicateQueryParamError{Param: "digest"},
		},
		{
			name:      "empty tag",
			src:       "oci://127.0.0.1:5000/terraform-modules/vpc?tag=",
			wantErrIs: getter.OCIEmptyQueryParamError{Param: "tag"},
		},
		{
			name:      "empty digest",
			src:       "oci://127.0.0.1:5000/terraform-modules/vpc?digest=",
			wantErrIs: getter.OCIEmptyQueryParamError{Param: "digest"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)
			g := newTestOCIGetter(staticStore(store))
			dst := filepath.Join(t.TempDir(), "module")

			_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
				Src:     tc.src,
				Dst:     dst,
				GetMode: gogetter.ModeDir,
			})
			require.ErrorIs(t, err, tc.wantErrIs)
			assert.Empty(t, store.gotRefs, "validation must fail before any resolution")
		})
	}
}

func TestOCIGetterGetInvalidRefValues(t *testing.T) {
	t.Parallel()

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "root" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)

	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)
	g := newTestOCIGetter(staticStore(store))

	// A digest value that does not parse must never be resolved as a ref,
	// since a registry could interpret it as a mutable tag.
	_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?digest=prod",
		Dst:     filepath.Join(t.TempDir(), "module"),
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)

	var digestErr getter.OCIInvalidDigestError

	require.ErrorAs(t, err, &digestErr)
	assert.Equal(t, "prod", digestErr.Value)
	assert.Empty(t, store.gotRefs)

	_, err = newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=.invalid",
		Dst:     filepath.Join(t.TempDir(), "module"),
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)

	var tagErr getter.OCIInvalidTagError

	require.ErrorAs(t, err, &tagErr)
	assert.Equal(t, ".invalid", tagErr.Value)
	assert.Empty(t, store.gotRefs)
}

func TestOCIGetterGetStoreError(t *testing.T) {
	t.Parallel()

	errStore := errors.New("store construction failed")
	g := newTestOCIGetter(func(_ context.Context, _, _ string) (getter.OCIRepositoryStore, error) {
		return nil, errStore
	})
	dst := filepath.Join(t.TempDir(), "module")

	_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errStore)
}

func TestOCIGetterGetFileUnsupported(t *testing.T) {
	t.Parallel()

	g := newTestOCIGetter(nil)

	err := g.GetFile(t.Context(), &gogetter.Request{})
	require.Error(t, err)
	assert.ErrorIs(t, err, getter.ErrOCIGetFileUnsupported)
}

func TestOCIGetterGetManifestHardening(t *testing.T) {
	t.Parallel()

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "root" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)

	wrongDescMediaType := manifestDesc
	wrongDescMediaType.MediaType = "application/vnd.example.other"

	oversized := manifestDesc
	oversized.Size = 5 << 20

	negativeSize := manifestDesc
	negativeSize.Size = -1

	mismatchedManifest := ociv1.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    "application/vnd.example.other",
		ArtifactType: getter.ArtifactTypeModulePkg,
		Config:       ociv1.DescriptorEmptyJSON,
		Layers:       []ociv1.Descriptor{layer},
	}
	mismatchedBytes, err := json.Marshal(mismatchedManifest)
	require.NoError(t, err)

	mismatchedDesc := ociv1.Descriptor{
		MediaType: ociv1.MediaTypeImageManifest,
		Digest:    digest.FromBytes(mismatchedBytes),
		Size:      int64(len(mismatchedBytes)),
	}

	testCases := []struct { //nolint:govet // fieldalignment: keyed literals; readability over 8 bytes
		wantErrIs     error
		name          string
		manifestBytes []byte
		manifestDesc  ociv1.Descriptor
	}{

		{
			name:          "descriptor media type rejected before fetch",
			manifestBytes: manifestBytes,
			manifestDesc:  wrongDescMediaType,
			wantErrIs:     getter.OCIManifestMediaTypeError{MediaType: "application/vnd.example.other"},
		},
		{
			name:          "oversized manifest rejected before fetch",
			manifestBytes: manifestBytes,
			manifestDesc:  oversized,
			wantErrIs:     getter.OCIManifestSizeError{Size: 5 << 20},
		},
		{
			name:          "negative manifest size rejected before fetch",
			manifestBytes: manifestBytes,
			manifestDesc:  negativeSize,
			wantErrIs:     getter.OCIManifestSizeError{Size: -1},
		},
		{
			name:          "decoded media type must match the descriptor",
			manifestBytes: mismatchedBytes,
			manifestDesc:  mismatchedDesc,
			wantErrIs:     getter.OCIManifestMediaTypeError{MediaType: "application/vnd.example.other"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newFakeStore(tc.manifestBytes, &tc.manifestDesc, zipBytes, &layer)
			g := newTestOCIGetter(staticStore(store))
			dst := filepath.Join(t.TempDir(), "module")

			_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
				Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
				Dst:     dst,
				GetMode: gogetter.ModeDir,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErrIs)
		})
	}
}

func TestOCIGetterGetRemovesStaleFiles(t *testing.T) {
	t.Parallel()

	dst := filepath.Join(t.TempDir(), "module")

	fetch := func(files map[string]string) {
		zipBytes := moduleZipBytes(t, files)
		layer := zipLayerDesc(zipBytes)
		manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
		store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

		_, err := newOCITestClient(newTestOCIGetter(staticStore(store))).Get(t.Context(), &gogetter.Request{
			Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
			Dst:     dst,
			GetMode: gogetter.ModeDir,
		})
		require.NoError(t, err)
	}

	fetch(map[string]string{"main.tf": `output "v1" {}`, "obsolete.tf": `output "gone" {}`})
	require.FileExists(t, filepath.Join(dst, "obsolete.tf"))

	fetch(map[string]string{"main.tf": `output "v2" {}`})

	got, err := os.ReadFile(filepath.Join(dst, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, `output "v2" {}`, string(got), "the second version's content must replace the first")
	assert.NoFileExists(t, filepath.Join(dst, "obsolete.tf"), "files removed between versions must not survive")
	assert.NoFileExists(t, filepath.Join(dst, ".tgmanifest"), "the copy manifest must not leak into the destination")
}

func TestOCIGetterGetNoManifestLeak(t *testing.T) {
	t.Parallel()

	zipBytes := moduleZipBytes(t, map[string]string{
		"main.tf":             `output "root" {}`,
		"subdir/sub.tf":       `output "sub" {}`,
		"subdir/deep/deep.tf": `output "deep" {}`,
	})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

	dst := filepath.Join(t.TempDir(), "module")

	_, err := newOCITestClient(newTestOCIGetter(staticStore(store))).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)

	// Assert the nested tree landed, so the walk below cannot pass vacuously.
	require.FileExists(t, filepath.Join(dst, "subdir", "deep", "deep.tf"))

	err = filepath.WalkDir(dst, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		assert.NotEqual(t, ".tgmanifest", d.Name(), "copy manifest must not leak into the module tree")

		return nil
	})
	require.NoError(t, err)
}

func TestOCIGetterGetFailedExtractionPreservesDestination(t *testing.T) {
	t.Parallel()

	dst := filepath.Join(t.TempDir(), "module")
	sentinel := filepath.Join(dst, "keep.tf")
	require.NoError(t, os.MkdirAll(dst, 0o755))
	require.NoError(t, os.WriteFile(sentinel, []byte(`output "keep" {}`), 0o644))

	// First entry extracts, the second trips the zip-slip guard mid-loop; staging keeps both out of dst.
	zipBytes := orderedModuleZipBytes(t, []zipEntry{
		{name: "main.tf", body: `output "leaked" {}`},
		{name: "../escape.tf", body: `output "escape" {}`},
	})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

	_, err := newOCITestClient(newTestOCIGetter(staticStore(store))).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)
	assert.FileExists(t, sentinel, "a failed extraction must not corrupt the destination")
	assert.NoFileExists(t, filepath.Join(dst, "main.tf"), "a failed extraction must not leak partial contents")
}

// TestOCIGetterGetRejectsTooManyFiles: a digest-valid archive must not exhaust inodes.
func TestOCIGetterGetRejectsTooManyFiles(t *testing.T) {
	t.Parallel()

	entries := []zipEntry{
		{name: "a.tf", body: ""},
		{name: "b.tf", body: ""},
		{name: "c.tf", body: ""},
	}

	zipBytes := orderedModuleZipBytes(t, entries)
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

	g := newTestOCIGetter(staticStore(store))
	g.MaxDecompressedFiles = 2

	dst := filepath.Join(t.TempDir(), "module")

	_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)
	assert.NoDirExists(t, dst, "an over-limit archive must not land in the destination")
}

// TestOCIGetterGetRejectsOversizedContent: a digest-valid archive must not exhaust the disk.
func TestOCIGetterGetRejectsOversizedContent(t *testing.T) {
	t.Parallel()

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "oversized" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

	g := newTestOCIGetter(staticStore(store))
	g.MaxDecompressedSize = 8

	dst := filepath.Join(t.TempDir(), "module")

	_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)
	assert.NoDirExists(t, dst, "an oversized archive must not land in the destination")
}

// TestOCIGetterGetRejectsLayerSize: invalid declared layer sizes fail before any fetch.
func TestOCIGetterGetRejectsLayerSize(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		size int64
	}{
		{name: "oversized layer", size: 1 << 40},
		{name: "zero size layer", size: 0},
		{name: "negative size layer", size: -1},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "sized" {}`})
			layer := zipLayerDesc(zipBytes)
			layer.Size = tc.size
			manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
			store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)
			// Drop the layer blob so any premature fetch fails the assertion below.
			delete(store.blobs, layer.Digest.String())

			g := newTestOCIGetter(staticStore(store))
			dst := filepath.Join(t.TempDir(), "module")

			_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
				Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
				Dst:     dst,
				GetMode: gogetter.ModeDir,
			})
			require.ErrorIs(t, err, getter.OCILayerSizeError{Size: tc.size})
		})
	}
}

// TestOCIGetterGetKeepsBackupWhenRestoreFails: the displaced module must survive a double rename failure.
func TestOCIGetterGetKeepsBackupWhenRestoreFails(t *testing.T) {
	t.Parallel()

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "next" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

	parentDir := t.TempDir()
	dst := filepath.Join(parentDir, "module")
	require.NoError(t, os.MkdirAll(dst, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "old.tf"), []byte(`output "old" {}`), 0o600))

	g := newTestOCIGetter(staticStore(store))
	g.FS = &renameFailFromFS{FS: g.FS, failFrom: 2}

	_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)

	var restoreErr getter.OCIRestoreError

	require.ErrorAs(t, err, &restoreErr)
	assert.NotEmpty(t, restoreErr.BackupPath)

	backups, globErr := filepath.Glob(filepath.Join(parentDir, ".terragrunt-oci*", "previous", "old.tf"))
	require.NoError(t, globErr)
	require.Len(t, backups, 1, "the previous module must remain recoverable")
}

// TestOCIGetterGetHonorsUmask: the promoted module root must respect the request umask.
func TestOCIGetterGetHonorsUmask(t *testing.T) {
	t.Parallel()

	moduleFiles := map[string]string{
		"main.tf":       `output "root" {}`,
		"subdir/sub.tf": `output "sub" {}`,
	}
	zipBytes := moduleZipBytes(t, moduleFiles)
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)

	testCases := []struct {
		name string
		src  string
	}{
		{name: "whole module", src: "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0"},
		{name: "subdir selector", src: "oci://127.0.0.1:5000/terraform-modules/vpc//subdir?tag=1.0.0"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)
			dst := filepath.Join(t.TempDir(), "module")

			_, err := newOCITestClient(newTestOCIGetter(staticStore(store))).Get(t.Context(), &gogetter.Request{
				Src:     tc.src,
				Dst:     dst,
				GetMode: gogetter.ModeDir,
				Umask:   0o077,
			})
			require.NoError(t, err)

			info, statErr := os.Stat(dst)
			require.NoError(t, statErr)
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "module root must respect the umask")
		})
	}
}

// TestOCIGetterGetRestoresDestinationWhenPromotionFails: the previous module must
// survive a failure in the swap that replaces it.
func TestOCIGetterGetRestoresDestinationWhenPromotionFails(t *testing.T) {
	t.Parallel()

	dst := filepath.Join(t.TempDir(), "module")
	sentinel := filepath.Join(dst, "keep.tf")
	require.NoError(t, os.MkdirAll(dst, 0o755))
	require.NoError(t, os.WriteFile(sentinel, []byte(`output "keep" {}`), 0o644))

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "new" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

	g := newTestOCIGetter(staticStore(store))
	// Fail the swap that moves the staged tree in, after the previous tree moved out.
	g.FS = &renameFailFS{FS: g.FS, failOn: 2}

	_, err := newOCITestClient(g).Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.ErrorIs(t, err, errRenameFailed)
	assert.FileExists(t, sentinel, "a failed promotion must restore the previous module")
	assert.NoFileExists(t, filepath.Join(dst, "main.tf"), "a failed promotion must not leave the new module behind")
}

func TestNewClientWithOCIDetectOrdering(t *testing.T) {
	t.Parallel()

	zipBytes := moduleZipBytes(t, map[string]string{"main.tf": `output "wired" {}`})
	layer := zipLayerDesc(zipBytes)
	manifestBytes, manifestDesc := manifestFor(t, getter.ArtifactTypeModulePkg, layer)
	store := newFakeStore(manifestBytes, &manifestDesc, zipBytes, &layer)

	client := getter.NewClient(
		getter.WithLogger(logger.CreateLogger()),
		getter.WithOCI(newTestOCIGetter(staticStore(store))),
	)

	dst := filepath.Join(t.TempDir(), "module")

	// The full default protocol set (git, http(s), s3, gcs, file) is
	// registered; a successful fake-store download proves the OCI getter
	// claimed the source before any generic getter shadowed it.
	_, err := client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?tag=1.0.0",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"1.0.0"}, store.gotRefs)
	assert.FileExists(t, filepath.Join(dst, "main.tf"))
}

func TestNewClientWithoutOCIRejectsOCISources(t *testing.T) {
	t.Parallel()

	client := getter.NewClient(getter.WithLogger(logger.CreateLogger()))
	dst := filepath.Join(t.TempDir(), "module")

	_, err := client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://127.0.0.1:5000/terraform-modules/vpc?bogus=1",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.Error(t, err)
	// Without WithOCI no getter recognizes the scheme, so the typed OCI
	// validation error never appears.
	assert.NotErrorIs(t, err, getter.OCIUnsupportedQueryParamError{Param: "bogus"})
}

func TestDefaultGenericFetchersExcludeOCI(t *testing.T) {
	t.Parallel()

	_, found := getter.DefaultGenericFetchers()[getter.SchemeOCI]
	assert.False(t, found, "generic dispatch must never route oci sources")
}

// errRenameFailed is the injected fault renameFailFS returns.
var errRenameFailed = errors.New("rename failed")

// renameFailFS fails the failOn-th Rename, to drive a promotion failure.
type renameFailFS struct {
	vfs.FS
	failOn int
	calls  int
}

func (fs *renameFailFS) Rename(oldname, newname string) error {
	fs.calls++

	if fs.calls == fs.failOn {
		return errRenameFailed
	}

	return fs.FS.Rename(oldname, newname)
}

// fakeStore is an in-memory OCIRepositoryStore serving one manifest and its
// layer blobs; it records every resolved ref.
type fakeStore struct {
	blobs         map[string][]byte
	manifestDesc  ociv1.Descriptor
	manifestBytes []byte
	gotRefs       []string
}

func newFakeStore(
	manifestBytes []byte,
	manifestDesc *ociv1.Descriptor,
	blobBytes []byte,
	blobDesc *ociv1.Descriptor,
) *fakeStore {
	return &fakeStore{
		manifestBytes: manifestBytes,
		manifestDesc:  *manifestDesc,
		blobs:         map[string][]byte{blobDesc.Digest.String(): blobBytes},
	}
}

func (s *fakeStore) Resolve(_ context.Context, ref string) (ociv1.Descriptor, error) {
	s.gotRefs = append(s.gotRefs, ref)

	return s.manifestDesc, nil
}

//nolint:gocritic // hugeParam: by-value descriptor is mandated by the OCIRepositoryStore seam
func (s *fakeStore) Fetch(_ context.Context, desc ociv1.Descriptor) (io.ReadCloser, error) {
	if desc.Digest == s.manifestDesc.Digest {
		return io.NopCloser(bytes.NewReader(s.manifestBytes)), nil
	}

	blob, found := s.blobs[desc.Digest.String()]
	if !found {
		return nil, errUnknownBlob
	}

	return io.NopCloser(bytes.NewReader(blob)), nil
}

// recordingStore returns a NewStore closure that yields store and records
// the registry domain and repository name it was asked for.
func recordingStore(store *fakeStore, gotDomain, gotRepo *string) getter.OCINewStoreFunc {
	return func(_ context.Context, registryDomain, repositoryName string) (getter.OCIRepositoryStore, error) {
		*gotDomain = registryDomain
		*gotRepo = repositoryName

		return store, nil
	}
}

// staticStore returns a NewStore closure that always yields store.
func staticStore(store *fakeStore) getter.OCINewStoreFunc {
	return func(_ context.Context, _, _ string) (getter.OCIRepositoryStore, error) {
		return store, nil
	}
}

func newTestOCIGetter(newStore getter.OCINewStoreFunc) *getter.OCIGetter {
	return &getter.OCIGetter{
		NewStore: newStore,
		Logger:   logger.CreateLogger(),
		FS:       vfs.NewOSFS(),
	}
}

func newOCITestClient(g *getter.OCIGetter) *gogetter.Client {
	return getter.NewClient(getter.WithCustomGettersPrepended(g))
}

// moduleZipBytes builds an in-memory zip holding files keyed by relative path.
func moduleZipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for name, body := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)

		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}

	require.NoError(t, zw.Close())

	return buf.Bytes()
}

// zipEntry is one file in an order-preserving zip.
type zipEntry struct {
	name string
	body string
}

// orderedModuleZipBytes builds a zip that preserves entry order.
func orderedModuleZipBytes(t *testing.T, entries []zipEntry) []byte {
	t.Helper()

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for _, entry := range entries {
		w, err := zw.Create(entry.name)
		require.NoError(t, err)

		_, err = w.Write([]byte(entry.body))
		require.NoError(t, err)
	}

	require.NoError(t, zw.Close())

	return buf.Bytes()
}

// zipLayerDesc describes zipBytes as a module-zip layer.
func zipLayerDesc(zipBytes []byte) ociv1.Descriptor {
	return ociv1.Descriptor{
		MediaType: getter.MediaTypeModuleZip,
		Digest:    digest.FromBytes(zipBytes),
		Size:      int64(len(zipBytes)),
	}
}

// manifestFor marshals an OCI image manifest with the given artifact type and
// layers, returning the manifest bytes and their descriptor.
func manifestFor(t *testing.T, artifactType string, layers ...ociv1.Descriptor) ([]byte, ociv1.Descriptor) {
	t.Helper()

	manifest := ociv1.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ociv1.MediaTypeImageManifest,
		ArtifactType: artifactType,
		Config:       ociv1.DescriptorEmptyJSON,
		Layers:       layers,
	}

	manifestBytes, err := json.Marshal(manifest)
	require.NoError(t, err)

	return manifestBytes, ociv1.Descriptor{
		MediaType: ociv1.MediaTypeImageManifest,
		Digest:    digest.FromBytes(manifestBytes),
		Size:      int64(len(manifestBytes)),
	}
}

// renameFailFromFS fails every Rename from call number failFrom onward.
type renameFailFromFS struct {
	vfs.FS
	calls    int
	failFrom int
}

func (f *renameFailFromFS) Rename(oldname, newname string) error {
	f.calls++
	if f.calls >= f.failFrom {
		return errRenameFailed
	}

	return f.FS.Rename(oldname, newname)
}
