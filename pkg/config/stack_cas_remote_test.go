package config_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ociv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/git"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worker"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/ociregistry"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
)

// The repositories and tag the catalog tests publish under.
const (
	casCatalogRepository     = "catalog"
	casCatalogRootRepository = "catalog-root"
	casCatalogTag            = "v1"
)

// The quoted cas:: reference a rewritten stack source and a rewritten unit source take.
const (
	casStackRefPattern = `"cas::sha256:[a-f0-9]{64}"`
	casUnitRefPattern  = `"cas::sha256:[a-f0-9]{64}//modules/vpc"`
)

// casCatalogFiles is a catalog whose stack and unit reach their siblings through update_source_with_cas.
var casCatalogFiles = map[string]string{
	"stacks/svc/terragrunt.stack.hcl": `unit "a" {
  source                 = "../..//units/a"
  update_source_with_cas = true
  path                   = "a"
}
`,
	"units/a/terragrunt.hcl": `terraform {
  source                 = "../..//modules/vpc"
  update_source_with_cas = true
}
`,
	"modules/vpc/main.tf": `resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`,
	"stacks/svc/README.md": "The svc stack.\n",
	"stacks/svc-other/terragrunt.stack.hcl": `unit "b" {
  source                 = "../..//units/a"
  update_source_with_cas = true
  path                   = "b"
}
`,
}

// casCatalogRootFiles is a catalog whose root is the stack itself, consumed without a //subdir.
var casCatalogRootFiles = map[string]string{
	"terragrunt.stack.hcl": `unit "a" {
  source                 = "./units/a"
  update_source_with_cas = true
  path                   = "a"
}
`,
	"units/a/terragrunt.hcl": `terraform {
  source = "."
}
`,
}

// TestGenerateStackOCIRewritesUpdateSourceWithCAS: an oci:// catalog gets its update_source_with_cas sources rewritten.
func TestGenerateStackOCIRewritesUpdateSourceWithCAS(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	manifest := registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)
	registry.PushModule(t, casCatalogRootRepository, casCatalogTag, casCatalogRootFiles)

	testCases := []struct {
		name      string
		kind      string
		source    string
		generated string
		wantRef   string
		stale     string
	}{
		{
			name:      "stack block by tag",
			kind:      "stack",
			source:    ociCatalogSource(registry, "stacks/svc"),
			generated: config.DefaultStackFile,
			wantRef:   casStackRefPattern,
			stale:     `"../..//units/a"`,
		},
		{
			name:      "unit block keeps the subdir tail of terraform.source",
			kind:      "unit",
			source:    ociCatalogSource(registry, "units/a"),
			generated: config.DefaultTerragruntConfigPath,
			wantRef:   casUnitRefPattern,
			stale:     `"../..//modules/vpc"`,
		},
		{
			name: "stack block by digest",
			kind: "stack",
			source: "oci://" + registry.Address() + "/" + casCatalogRepository +
				"//stacks/svc?digest=" + manifest.Digest.String(),
			generated: config.DefaultStackFile,
			wantRef:   casStackRefPattern,
			stale:     `"../..//units/a"`,
		},
		{
			name: "stack block in the forced form",
			kind: "stack",
			source: "oci::https://" + registry.Address() + "/" + casCatalogRepository +
				"//stacks/svc?tag=" + casCatalogTag,
			generated: config.DefaultStackFile,
			wantRef:   casStackRefPattern,
			stale:     `"../..//units/a"`,
		},
		{
			name: "stack block in the uppercase forced form",
			kind: "stack",
			source: "OCI::https://" + registry.Address() + "/" + casCatalogRepository +
				"//stacks/svc?tag=" + casCatalogTag,
			generated: config.DefaultStackFile,
			wantRef:   casStackRefPattern,
			stale:     `"../..//units/a"`,
		},
		{
			name:      "stack block without a subdir",
			kind:      "stack",
			source:    "oci://" + registry.Address() + "/" + casCatalogRootRepository + "?tag=" + casCatalogTag,
			generated: config.DefaultStackFile,
			wantRef:   casStackRefPattern,
			stale:     `"./units/a"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := ociCatalogVenv(t, registry)
			stackPath := catalogLiveStack(t, v, tc.kind, tc.source)

			err := generateCatalogStack(t, v, stackPath)
			require.NoError(t, err)

			content, err := vfs.ReadFile(v.FS, filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", tc.generated))
			require.NoError(t, err)

			assert.Regexp(t, tc.wantRef, string(content))
			assert.NotContains(t, string(content), tc.stale)
			assert.Contains(t, string(content), "update_source_with_cas")
		})
	}
}

// TestGenerateStackHTTPArchiveRewritesUpdateSourceWithCAS: every archive selection form takes the CAS walk.
func TestGenerateStackHTTPArchiveRewritesUpdateSourceWithCAS(t *testing.T) {
	t.Parallel()

	archive := catalogZip(t, casCatalogFiles)
	server := newArchiveServer(t, archive)
	sum := sha256.Sum256(archive)
	digest := hex.EncodeToString(sum[:])

	testCases := []struct {
		name   string
		source string
	}{
		{name: "archive by extension", source: server.URL + "/catalog.zip//stacks/svc"},
		{name: "archive by explicit format", source: server.URL + "/catalog//stacks/svc?archive=zip"},
		{name: "archive with matching checksum", source: server.URL + "/catalog.zip//stacks/svc?checksum=sha256:" + digest},
		{
			name:   "archive with a checksum file",
			source: server.URL + "/catalog.zip//stacks/svc?checksum=file:" + catalogChecksumFile(t, digest),
		},
		{name: "wildcard selecting one directory", source: server.URL + "/catalog.zip//stacks/*vc"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := casCatalogVenv(t)
			stackPath := catalogLiveStack(t, v, "stack", tc.source)

			err := generateCatalogStack(t, v, stackPath)
			require.NoError(t, err)

			content, err := vfs.ReadFile(v.FS, filepath.Join(
				filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile,
			))
			require.NoError(t, err)

			assert.Regexp(t, casStackRefPattern, string(content))
			assert.NotContains(t, string(content), `"../..//units/a"`)
		})
	}
}

// TestGenerateStackHTTPArchiveSelectionFailures: a bad checksum or an unusable wildcard fails generation.
func TestGenerateStackHTTPArchiveSelectionFailures(t *testing.T) {
	t.Parallel()

	archive := catalogZip(t, casCatalogFiles)

	testCases := []struct {
		target  any
		wantErr error
		name    string
		path    string
	}{
		{
			target: new(*getter.ChecksumError),
			name:   "mismatched checksum",
			path:   "/catalog.zip//stacks/svc?checksum=sha256:" + strings.Repeat("0", 64),
		},
		{
			target: new(*getter.ChecksumError),
			name:   "mismatched checksum file",
			path:   "/catalog.zip//stacks/svc?checksum=file:" + catalogChecksumFile(t, strings.Repeat("0", 64)),
		},
		{
			wantErr: getter.ErrSubdirNotResolved,
			name:    "wildcard without a match",
			path:    "/catalog.zip//stacks/x*",
		},
		{
			wantErr: getter.ErrSubdirNotResolved,
			name:    "wildcard selecting several directories",
			path:    "/catalog.zip//stacks/s*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := newArchiveServer(t, archive)
			v := casCatalogVenv(t)
			stackPath := catalogLiveStack(t, v, "stack", server.URL+tc.path)

			err := generateCatalogStack(t, v, stackPath)
			require.Error(t, err)

			if tc.target != nil {
				require.ErrorAs(t, err, tc.target)
			}

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}

			assert.NoFileExists(t, filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile))
			assert.Equal(t, int32(1), server.downloads.Load(), "the standard getter must not repeat the failed download")
		})
	}
}

// TestGenerateStackHTTPArchiveChecksumHonorsOffline: a pinned catalog generates offline from a warm store only.
func TestGenerateStackHTTPArchiveChecksumHonorsOffline(t *testing.T) {
	t.Parallel()

	archive := catalogZip(t, casCatalogFiles)

	server := newArchiveServer(t, archive)

	sum := sha256.Sum256(archive)
	source := server.URL + "/catalog.zip//stacks/svc?checksum=sha256:" + hex.EncodeToString(sum[:])

	recording := func(pctx *config.ParsingContext) {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OfflineCAS))
	}
	offline := func(pctx *config.ParsingContext) {
		recording(pctx)
		pctx.CASOffline = true
	}

	v := casCatalogVenv(t)

	stackPath := catalogLiveStack(t, v, "stack", source)
	err := generateCatalogStack(t, v, stackPath, offline)
	require.ErrorIs(t, err, cas.ErrCASOffline, "an empty store must fail instead of downloading")
	assert.Equal(t, int32(0), server.downloads.Load(), "the offline miss must not download the archive")
	assert.Equal(t, int32(0), server.heads.Load(), "the offline miss must not send a HEAD request either")

	stackPath = catalogLiveStack(t, v, "stack", source)
	err = generateCatalogStack(t, v, stackPath, recording)
	require.NoError(t, err)

	fetched, headed := server.downloads.Load(), server.heads.Load()
	require.Positive(t, fetched, "the online run must download the archive")

	stackPath = catalogLiveStack(t, v, "stack", source)
	err = generateCatalogStack(t, v, stackPath, offline)
	require.NoError(t, err, "a warm store must serve the pinned archive offline")

	content, err := vfs.ReadFile(v.FS, filepath.Join(
		filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile,
	))
	require.NoError(t, err)

	assert.Regexp(t, casStackRefPattern, string(content))
	assert.Equal(t, fetched, server.downloads.Load(), "the offline generation must not download the archive again")
	assert.Equal(t, headed, server.heads.Load(), "the offline generation must not send a HEAD request")
}

// TestGenerateStackHTTPArchiveChecksumOfflineWithoutRecordedProbe: a literal pin needs no recorded probe offline.
func TestGenerateStackHTTPArchiveChecksumOfflineWithoutRecordedProbe(t *testing.T) {
	t.Parallel()

	archive := catalogZip(t, casCatalogFiles)
	server := newArchiveServer(t, archive)

	sum := sha256.Sum256(archive)
	source := server.URL + "/catalog.zip//stacks/svc?checksum=sha256:" + hex.EncodeToString(sum[:])

	v := casCatalogVenv(t)

	stackPath := catalogLiveStack(t, v, "stack", source)
	require.NoError(t, generateCatalogStack(t, v, stackPath), "the run without offline-cas records no probe")

	fetched := server.downloads.Load()
	require.Positive(t, fetched, "the online run must download the archive")

	stackPath = catalogLiveStack(t, v, "stack", source)
	err := generateCatalogStack(t, v, stackPath, func(pctx *config.ParsingContext) {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OfflineCAS))
		pctx.CASOffline = true
	})
	require.NoError(t, err, "the checksum must key the stored archive offline")
	assert.Equal(t, fetched, server.downloads.Load(), "the offline generation must not download the archive")
}

// TestGenerateStackBucketShorthandStagesThroughCAS: s3 and gcs shorthand catalogs take the staged package path.
func TestGenerateStackBucketShorthandStagesThroughCAS(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		source string
	}{
		{name: "s3", source: "example-bucket.s3.amazonaws.com/catalog.zip//stacks/svc"},
		{name: "s3 virtual host", source: "https://example-bucket.s3.amazonaws.com/catalog.zip//stacks/svc"},
		{name: "gcs", source: "www.googleapis.com/storage/v1/example-bucket/catalog.zip//stacks/svc"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := casCatalogVenv(t)
			stackPath := catalogLiveStack(t, v, "stack", tc.source)

			err := generateCatalogStack(t, v, stackPath, func(pctx *config.ParsingContext) {
				require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OfflineCAS))
				pctx.CASOffline = true
			})
			require.ErrorIs(t, err, cas.ErrCASOffline)

			var packageErr config.StackPackageFetchError
			require.ErrorAs(t, err, &packageErr)
		})
	}
}

// TestGenerateStackHTTPArchiveOfflineNeverFallsBack: offline, a late CAS failure never reaches the standard getter.
func TestGenerateStackHTTPArchiveOfflineNeverFallsBack(t *testing.T) {
	t.Parallel()

	broken := maps.Clone(casCatalogFiles)
	broken["stacks/svc/terragrunt.stack.hcl"] = `unit "a" {`

	archive := catalogZip(t, broken)
	server := newArchiveServer(t, archive)

	sum := sha256.Sum256(archive)
	source := server.URL + "/catalog.zip//stacks/svc?checksum=sha256:" + hex.EncodeToString(sum[:])

	recording := func(pctx *config.ParsingContext) {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OfflineCAS))
	}
	offline := func(pctx *config.ParsingContext) {
		recording(pctx)
		pctx.CASOffline = true
	}

	v := casCatalogVenv(t)

	stackPath := catalogLiveStack(t, v, "stack", source)
	err := generateCatalogStack(t, v, stackPath, recording)
	require.NoError(t, err, "the online run falls back to the standard getter after the walk fails to parse")

	fetched := server.downloads.Load()
	require.Positive(t, fetched, "the online run must download the archive")

	stackPath = catalogLiveStack(t, v, "stack", source)
	err = generateCatalogStack(t, v, stackPath, offline)
	require.Error(t, err)
	assert.Equal(t, fetched, server.downloads.Load(), "the offline run must not fetch through the standard getter")
}

// TestGenerateStackOCIRepeatGenerationSkipsDownload: a second generation links the stored package, no blob download.
func TestGenerateStackOCIRepeatGenerationSkipsDownload(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)

	testCases := []struct {
		name   string
		source string
	}{
		{name: "oci scheme", source: ociCatalogSource(registry, "stacks/svc")},
		{
			name:   "forced form",
			source: "oci::https://" + registry.Address() + "/" + casCatalogRepository + "//stacks/svc?tag=" + casCatalogTag,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v, transport := countingOCIVenv(t, registry)

			stackPath := catalogLiveStack(t, v, "stack", tc.source)
			require.NoError(t, generateCatalogStack(t, v, stackPath))

			blobs := transport.count("/blobs/")
			require.Positive(t, blobs, "the first generation must download the package")

			stackPath = catalogLiveStack(t, v, "stack", tc.source)
			require.NoError(t, generateCatalogStack(t, v, stackPath))
			assert.Equal(t, blobs, transport.count("/blobs/"), "the second generation must link the stored package")
		})
	}
}

// TestGenerateStackOCIMutableKeepsReferences: mutable = true and the default yield the same cas:: reference.
func TestGenerateStackOCIMutableKeepsReferences(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)

	refs := make([]string, 0, 2)

	for _, attrs := range [][]string{nil, {"mutable = true"}} {
		v := ociCatalogVenv(t, registry)
		stackPath := catalogLiveStack(t, v, "stack", ociCatalogSource(registry, "stacks/svc"), attrs...)
		require.NoError(t, generateCatalogStack(t, v, stackPath))

		content, err := vfs.ReadFile(v.FS, filepath.Join(
			filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile,
		))
		require.NoError(t, err)

		refs = append(refs, regexp.MustCompile(casStackRefPattern).FindString(string(content)))
	}

	require.NotEmpty(t, refs[0])
	assert.Equal(t, refs[0], refs[1], "mutable must not change the cas:: references")
}

// TestGenerateStackHTTPArchiveChecksumFileNeverAnsweredOffline: a checksum=file: pin is refused offline unfetched.
func TestGenerateStackHTTPArchiveChecksumFileNeverAnsweredOffline(t *testing.T) {
	t.Parallel()

	server := newArchiveServer(t, catalogZip(t, casCatalogFiles))
	source := server.URL + "/catalog.zip//stacks/svc?checksum=file:" + catalogChecksumFile(t, strings.Repeat("0", 64))

	v := casCatalogVenv(t)
	stackPath := catalogLiveStack(t, v, "stack", source)

	err := generateCatalogStack(t, v, stackPath, func(pctx *config.ParsingContext) {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OfflineCAS))
		pctx.CASOffline = true
	})

	var unrecordable *cas.OfflineUnrecordableError
	require.ErrorAs(t, err, &unrecordable)
	assert.Equal(t, int32(0), server.downloads.Load(), "a checksum file reference must not be fetched offline")
}

// TestGenerateStackGitMutableControlsWriteBits: mutable = true yields a writable git copy; the default is read-only.
func TestGenerateStackGitMutableControlsWriteBits(t *testing.T) {
	t.Parallel()

	repoURL := catalogGitURL(t)

	testCases := []struct {
		name         string
		attrs        []string
		wantWritable bool
	}{
		{name: "mutable", attrs: []string{"mutable = true"}, wantWritable: true},
		{name: "default", wantWritable: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := casCatalogVenv(t)
			stackPath := catalogLiveStack(t, v, "stack", repoURL+"//stacks/svc?ref=main", tc.attrs...)

			err := generateCatalogStack(t, v, stackPath)
			require.NoError(t, err)

			for _, name := range []string{"README.md", config.DefaultStackFile} {
				info, err := v.FS.Stat(filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", name))
				require.NoError(t, err)

				assert.Equal(t, tc.wantWritable, info.Mode().Perm()&0o200 != 0, "%s mode %v", name, info.Mode())
			}
		})
	}
}

// TestGenerateStackHTTPArchiveMissingNestedSourceFailsOnce: a marked source the package lacks fails, no fallback copy.
func TestGenerateStackHTTPArchiveMissingNestedSourceFailsOnce(t *testing.T) {
	t.Parallel()

	missing := maps.Clone(casCatalogFiles)
	missing["stacks/svc/terragrunt.stack.hcl"] = strings.ReplaceAll(
		casCatalogFiles["stacks/svc/terragrunt.stack.hcl"], "units/a", "units/missing",
	)

	server := newArchiveServer(t, catalogZip(t, missing))

	v := casCatalogVenv(t)
	stackPath := catalogLiveStack(t, v, "stack", server.URL+"/catalog.zip//stacks/svc")

	err := generateCatalogStack(t, v, stackPath)
	require.ErrorIs(t, err, cas.ErrRewriteIncomplete)
	assert.Equal(t, int32(1), server.downloads.Load(), "the standard getter must not copy the component unrewritten")
	assert.NoFileExists(t, filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile))
}

// TestGenerateStackOCIMutableControlsWriteBits: mutable = true yields a writable package; the default is read-only.
func TestGenerateStackOCIMutableControlsWriteBits(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)

	testCases := []struct {
		name         string
		attrs        []string
		wantWritable bool
	}{
		{name: "mutable", attrs: []string{"mutable = true"}, wantWritable: true},
		{name: "default", wantWritable: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := ociCatalogVenv(t, registry)
			stackPath := catalogLiveStack(t, v, "stack", ociCatalogSource(registry, "stacks/svc"), tc.attrs...)

			err := generateCatalogStack(t, v, stackPath)
			require.NoError(t, err)

			for _, name := range []string{"README.md", config.DefaultStackFile} {
				info, err := v.FS.Stat(filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", name))
				require.NoError(t, err)

				assert.Equal(t, tc.wantWritable, info.Mode().Perm()&0o200 != 0, "%s mode %v", name, info.Mode())
			}
		})
	}
}

// TestGenerateStackBareGitURLClonesWithGit: an http URL that names no archive is cloned by git, with or without .git.
func TestGenerateStackBareGitURLClonesWithGit(t *testing.T) {
	t.Parallel()

	repoURL := catalogGitURL(t)

	testCases := []struct {
		name string
		base string
	}{
		{name: "repository URL ending in .git", base: repoURL},
		{name: "repository URL without .git", base: strings.TrimSuffix(repoURL, ".git")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := casCatalogVenv(t)
			stackPath := catalogLiveStack(t, v, "stack", tc.base+"//stacks/svc?ref=main")

			err := generateCatalogStack(t, v, stackPath)
			require.NoError(t, err)

			content, err := vfs.ReadFile(v.FS, filepath.Join(
				filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile,
			))
			require.NoError(t, err)

			assert.Regexp(t, `"cas::sha1:[a-f0-9]{40}"`, string(content))
		})
	}
}

// TestGenerateStackOCICatalogNestedUnitGenerates: the second wave materializes the unit the first wave rewrote.
func TestGenerateStackOCICatalogNestedUnitGenerates(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)

	v := ociCatalogVenv(t, registry)
	stackPath := catalogLiveStack(t, v, "stack", ociCatalogSource(registry, "stacks/svc"))

	err := generateCatalogStack(t, v, stackPath)
	require.NoError(t, err)

	nestedStackPath := filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile)

	err = generateCatalogStack(t, v, nestedStackPath)
	require.NoError(t, err)

	content, err := vfs.ReadFile(v.FS, filepath.Join(
		filepath.Dir(nestedStackPath), config.StackDir, "a", config.DefaultTerragruntConfigPath,
	))
	require.NoError(t, err)

	assert.Regexp(t, casUnitRefPattern, string(content))
}

// TestGenerateStackOCINoCASKeepsRelativeSources: --no-cas keeps the standard getter, copying the package verbatim.
func TestGenerateStackOCINoCASKeepsRelativeSources(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)

	v := ociCatalogVenv(t, registry)
	stackPath := catalogLiveStack(t, v, "stack", ociCatalogSource(registry, "stacks/svc"))

	err := generateCatalogStack(t, v, stackPath, func(pctx *config.ParsingContext) { pctx.NoCAS = true })
	require.NoError(t, err)

	content, err := vfs.ReadFile(v.FS, filepath.Join(
		filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile,
	))
	require.NoError(t, err)

	assert.Contains(t, string(content), `"../..//units/a"`)
	assert.NotContains(t, string(content), "cas::")
}

// TestGenerateStackOCICASOfflineMissFails: --cas-offline refuses to contact the registry for a package the store lacks.
func TestGenerateStackOCICASOfflineMissFails(t *testing.T) {
	t.Parallel()

	registry := ociregistry.Start(t)
	registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)

	v := ociCatalogVenv(t, registry)
	stackPath := catalogLiveStack(t, v, "stack", ociCatalogSource(registry, "stacks/svc"))

	err := generateCatalogStack(t, v, stackPath, func(pctx *config.ParsingContext) {
		pctx.CASOffline = true
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.OfflineCAS))
	})
	require.ErrorIs(t, err, cas.ErrCASOffline)

	assert.NoFileExists(t, filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile))
}

// TestGenerateStackOCIRegistryFailuresSurfaceGetterError: a registry failure surfaces the getter's typed error once.
func TestGenerateStackOCIRegistryFailuresSurfaceGetterError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		target   any
		prepare  func(t *testing.T, registry *ociregistry.Registry)
		source   func(registry *ociregistry.Registry) string
		name     string
		request  string
		requests int
	}{
		{
			target:  new(getter.OCIReferenceResolutionError),
			prepare: func(_ *testing.T, _ *ociregistry.Registry) {},
			source: func(registry *ociregistry.Registry) string {
				return "oci://" + registry.Address() + "/" + casCatalogRepository + "//stacks/svc?tag=missing"
			},
			name:     "unknown tag",
			request:  "HEAD /v2/" + casCatalogRepository + "/manifests/missing",
			requests: 3,
		},
		{
			target: new(getter.OCIDigestVerificationError),
			prepare: func(t *testing.T, registry *ociregistry.Registry) {
				t.Helper()

				layer := ociCatalogLayer(t, registry)
				registry.OverwriteBlob(t, casCatalogRepository, layer.Digest, make([]byte, layer.Size))
			},
			source: func(registry *ociregistry.Registry) string {
				return ociCatalogSource(registry, "stacks/svc")
			},
			name:     "corrupted layer",
			request:  "GET /v2/" + casCatalogRepository + "/blobs/",
			requests: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registry := ociregistry.Start(t)
			registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)
			tc.prepare(t, registry)

			v, transport := countingOCIVenv(t, registry)
			stackPath := catalogLiveStack(t, v, "stack", tc.source(registry))

			err := generateCatalogStack(t, v, stackPath)
			require.ErrorAs(t, err, tc.target)

			var packageErr config.StackPackageFetchError
			require.ErrorAs(t, err, &packageErr)
			assert.Equal(t, tc.requests, transport.count(tc.request), "the standard getter must not repeat the failed fetch")

			assert.NoFileExists(t, filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile))
		})
	}
}

// ociCatalogSource builds the oci:// source selecting subdir from the published catalog.
func ociCatalogSource(registry *ociregistry.Registry, subdir string) string {
	return "oci://" + registry.Address() + "/" + casCatalogRepository + "//" + subdir + "?tag=" + casCatalogTag
}

// casCatalogVenv builds an OS-backed venv with a private CAS store and a hermetic home.
func casCatalogVenv(t *testing.T) *venv.Venv {
	t.Helper()

	home := t.TempDir()
	cacheDir := t.TempDir()

	return venvtest.NewOSWithEmptyEnv().
		WithEnv(map[string]string{"HOME": home}).
		WithUserHomeDir(func() (string, error) { return home, nil }).
		WithUserCacheDir(func() (string, error) { return cacheDir, nil })
}

// ociCatalogVenv is casCatalogVenv resolving the in-process registry.
func ociCatalogVenv(t *testing.T, registry *ociregistry.Registry) *venv.Venv {
	t.Helper()

	v := casCatalogVenv(t)
	v.HTTP = registry.Client()

	return v
}

// catalogLiveStack writes a live stack whose single component of kind is sourced from source, with any extra attrs.
func catalogLiveStack(t *testing.T, v *venv.Venv, kind, source string, attrs ...string) string {
	t.Helper()

	liveDir := filepath.Join(helpers.TmpDirWOSymlinks(t), "live")
	stackPath := filepath.Join(liveDir, config.DefaultStackFile)

	lines := make([]string, 0, len(attrs)+5)
	lines = append(lines, kind+` "svc" {`, `  source = "`+source+`"`, `  path   = "svc"`)

	for _, attr := range attrs {
		lines = append(lines, "  "+attr)
	}

	body := strings.Join(append(lines, "}", ""), "\n")

	require.NoError(t, v.FS.MkdirAll(liveDir, 0o755))
	require.NoError(t, vfs.WriteFile(v.FS, stackPath, []byte(body), 0o644))

	return stackPath
}

// generateCatalogStack generates stackPath on v and returns the joined generation and pool errors.
func generateCatalogStack(
	t *testing.T,
	v *venv.Venv,
	stackPath string,
	configure ...func(pctx *config.ParsingContext),
) error {
	t.Helper()

	l := logger.CreateLogger()
	l.SetOptions(log.WithOutput(io.Discard))

	pctx := config.NewParsingContext(config.WithStrictControls(controls.New()))
	pctx.TerragruntStackConfigPath = stackPath
	pctx.RootWorkingDir = filepath.Dir(stackPath)
	pctx.WorkingDir = filepath.Dir(stackPath)
	pctx.TerragruntConfigPath = stackPath
	pctx.CASCloneDepth = 1
	pctx.Experiments = experiment.NewExperiments()

	for _, c := range configure {
		c(pctx)
	}

	pool := worker.NewWorkerPool(1)
	pool.Start()

	defer pool.Stop()

	genErr := config.GenerateStackFile(t.Context(), l, v, pctx, pool, stackPath)
	waitErr := pool.Wait()

	return errors.Join(genErr, waitErr)
}

// archiveServer serves one archive at every path and counts the HEAD and GET requests it answers.
type archiveServer struct {
	*httptest.Server
	heads     atomic.Int32
	downloads atomic.Int32
}

// newArchiveServer starts an [archiveServer] for archive and closes it when the test ends.
func newArchiveServer(t *testing.T, archive []byte) *archiveServer {
	t.Helper()

	s := &archiveServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodHead:
			s.heads.Add(1)
		case http.MethodGet:
			s.downloads.Add(1)
		}

		http.ServeContent(w, r, "catalog.zip", time.Time{}, bytes.NewReader(archive))
	}))
	t.Cleanup(s.Close)

	return s
}

// catalogChecksumFile writes a GNU-style checksum line for catalog.zip and returns its file URL.
func catalogChecksumFile(t *testing.T, digest string) string {
	t.Helper()

	path := filepath.Join(helpers.TmpDirWOSymlinks(t), "catalog.sha256")
	require.NoError(t, os.WriteFile(path, []byte(digest+"  catalog.zip\n"), 0o644))

	return getter.FileURL(path).String()
}

// catalogZip packs files into a zip archive in name order.
func catalogZip(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer

	archive := zip.NewWriter(&buf)

	for _, name := range slices.Sorted(maps.Keys(files)) {
		entry, err := archive.Create(name)
		require.NoError(t, err)

		_, err = io.WriteString(entry, files[name])
		require.NoError(t, err)
	}

	require.NoError(t, archive.Close())

	return buf.Bytes()
}

// ociCatalogLayer resolves the published catalog manifest down to its single module-zip layer.
func ociCatalogLayer(t *testing.T, registry *ociregistry.Registry) ociv1.Descriptor {
	t.Helper()

	newStore := getter.NewOCIRepositoryStore(logger.CreateLogger(), ociCatalogVenv(t, registry))

	store, err := newStore(t.Context(), registry.Address(), casCatalogRepository)
	require.NoError(t, err)

	manifestDesc, err := store.Resolve(t.Context(), casCatalogTag)
	require.NoError(t, err)

	reader, err := store.Fetch(t.Context(), &manifestDesc)
	require.NoError(t, err)

	defer func() {
		require.NoError(t, reader.Close())
	}()

	var manifest ociv1.Manifest
	require.NoError(t, json.NewDecoder(reader).Decode(&manifest))
	require.Len(t, manifest.Layers, 1)

	return manifest.Layers[0]
}

// catalogGitURL serves casCatalogFiles from a git repository and returns its clone URL.
func catalogGitURL(t *testing.T) string {
	t.Helper()

	srv, err := git.NewServer()
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, srv.Close()) })

	files := make(map[string][]byte, len(casCatalogFiles))
	for name, content := range casCatalogFiles {
		files[name] = []byte(content)
	}

	require.NoError(t, srv.CommitFiles(t.Context(), files, "catalog"))

	repoURL, err := srv.Start(t.Context())
	require.NoError(t, err)

	return repoURL
}

// countingTransport counts the requests it forwards by method and path.
type countingTransport struct {
	next http.RoundTripper
	seen map[string]int
	mu   sync.Mutex
}

func (c *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.seen[req.Method+" "+req.URL.Path]++
	c.mu.Unlock()

	return c.next.RoundTrip(req)
}

// count returns the number of requests whose method and path contain fragment.
func (c *countingTransport) count(fragment string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	total := 0

	for key, n := range c.seen {
		if strings.Contains(key, fragment) {
			total += n
		}
	}

	return total
}

// countingOCIVenv returns an OCI catalog venv whose registry client counts its requests.
func countingOCIVenv(t *testing.T, registry *ociregistry.Registry) (*venv.Venv, *countingTransport) {
	t.Helper()

	trusted := registry.Client()
	require.NotNil(t, trusted.Transport)

	transport := &countingTransport{next: trusted.Transport, seen: map[string]int{}}

	v := casCatalogVenv(t)
	v.HTTP = &http.Client{Transport: transport}

	return v, transport
}
