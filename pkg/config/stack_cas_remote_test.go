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
	"slices"
	"strings"
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
	server := catalogArchiveServer(t, archive)
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

	server := catalogArchiveServer(t, catalogZip(t, casCatalogFiles))

	testCases := []struct {
		target  any
		wantErr error
		name    string
		source  string
	}{
		{
			target: new(*getter.ChecksumError),
			name:   "mismatched checksum",
			source: server.URL + "/catalog.zip//stacks/svc?checksum=sha256:" + strings.Repeat("0", 64),
		},
		{
			target: new(*getter.ChecksumError),
			name:   "mismatched checksum file",
			source: server.URL + "/catalog.zip//stacks/svc?checksum=file:" + catalogChecksumFile(t, strings.Repeat("0", 64)),
		},
		{
			wantErr: getter.ErrSubdirNotResolved,
			name:    "wildcard without a match",
			source:  server.URL + "/catalog.zip//stacks/x*",
		},
		{
			wantErr: getter.ErrSubdirNotResolved,
			name:    "wildcard selecting several directories",
			source:  server.URL + "/catalog.zip//stacks/s*",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := casCatalogVenv(t)
			stackPath := catalogLiveStack(t, v, "stack", tc.source)

			err := generateCatalogStack(t, v, stackPath)
			require.Error(t, err)

			if tc.target != nil {
				require.ErrorAs(t, err, tc.target)
			}

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}

			assert.NoFileExists(t, filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile))
		})
	}
}

// TestGenerateStackHTTPArchiveChecksumHonorsOffline: a pinned catalog generates offline from a warm store only.
func TestGenerateStackHTTPArchiveChecksumHonorsOffline(t *testing.T) {
	t.Parallel()

	archive := catalogZip(t, casCatalogFiles)

	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.ServeContent(w, r, "catalog.zip", time.Time{}, bytes.NewReader(archive))
	}))
	t.Cleanup(server.Close)

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
	assert.Equal(t, int32(0), requests.Load(), "the offline miss must not reach the server")

	stackPath = catalogLiveStack(t, v, "stack", source)
	err = generateCatalogStack(t, v, stackPath, recording)
	require.NoError(t, err)

	fetched := requests.Load()
	require.Positive(t, fetched, "the online run must download the archive")

	stackPath = catalogLiveStack(t, v, "stack", source)
	err = generateCatalogStack(t, v, stackPath, offline)
	require.NoError(t, err, "a warm store must serve the pinned archive offline")

	content, err := vfs.ReadFile(v.FS, filepath.Join(
		filepath.Dir(stackPath), config.StackDir, "svc", config.DefaultStackFile,
	))
	require.NoError(t, err)

	assert.Regexp(t, casStackRefPattern, string(content))
	assert.Equal(t, fetched, requests.Load(), "the offline generation must not download the archive again")
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

			// The README is copied as fetched; the stack file is rewritten.
			info, err := v.FS.Stat(filepath.Join(filepath.Dir(stackPath), config.StackDir, "svc", "README.md"))
			require.NoError(t, err)

			assert.Equal(t, tc.wantWritable, info.Mode().Perm()&0o200 != 0, "mode %v", info.Mode())
		})
	}
}

// TestGenerateStackBareGitURLClonesWithGit: an http URL that names no archive is cloned by git, with or without .git.
func TestGenerateStackBareGitURLClonesWithGit(t *testing.T) {
	t.Parallel()

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

			// A git catalog yields references keyed by commit sha1.
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

// TestGenerateStackOCIRegistryFailuresSurfaceGetterError: a registry failure falls back to the getter's typed error.
func TestGenerateStackOCIRegistryFailuresSurfaceGetterError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		target  any
		prepare func(t *testing.T, registry *ociregistry.Registry)
		source  func(registry *ociregistry.Registry) string
		name    string
	}{
		{
			target:  new(getter.OCIReferenceResolutionError),
			prepare: func(_ *testing.T, _ *ociregistry.Registry) {},
			source: func(registry *ociregistry.Registry) string {
				return "oci://" + registry.Address() + "/" + casCatalogRepository + "//stacks/svc?tag=missing"
			},
			name: "unknown tag",
		},
		{
			target: new(getter.OCIDigestVerificationError),
			prepare: func(t *testing.T, registry *ociregistry.Registry) {
				t.Helper()

				layer := ociCatalogLayer(t, registry)
				// A same-length blob reaches the digest check.
				registry.OverwriteBlob(t, casCatalogRepository, layer.Digest, make([]byte, layer.Size))
			},
			source: func(registry *ociregistry.Registry) string {
				return ociCatalogSource(registry, "stacks/svc")
			},
			name: "corrupted layer",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			registry := ociregistry.Start(t)
			registry.PushModule(t, casCatalogRepository, casCatalogTag, casCatalogFiles)
			tc.prepare(t, registry)

			v := ociCatalogVenv(t, registry)
			stackPath := catalogLiveStack(t, v, "stack", tc.source(registry))

			err := generateCatalogStack(t, v, stackPath)
			require.ErrorAs(t, err, tc.target)

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

	// The CAS stack path needs the OS filesystem; the private cache dir keeps its store off the machine's.
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

	// Fetch errors from pool workers surface from Wait.
	genErr := config.GenerateStackFile(t.Context(), l, v, pctx, pool, stackPath)
	waitErr := pool.Wait()

	return errors.Join(genErr, waitErr)
}

// catalogArchiveServer serves archive at every path.
func catalogArchiveServer(t *testing.T, archive []byte) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "catalog.zip", time.Time{}, bytes.NewReader(archive))
	}))
	t.Cleanup(server.Close)

	return server
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
