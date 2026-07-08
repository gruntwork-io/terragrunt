package getter_test

import (
	"context"
	"net/url"
	"path/filepath"
	"testing"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Detect
// ---------------------------------------------------------------------------

func TestOCIGetter_DetectClaimsOCIScheme(t *testing.T) {
	t.Parallel()

	g := getter.NewOCIGetter(logger.CreateLogger(), vfs.NewOSFS())
	req := &gogetter.Request{Src: "oci://ghcr.io/org/module:v1.0.0"}

	ok, err := g.Detect(req)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOCIGetter_DetectClaimsForcedOCIPrefix(t *testing.T) {
	t.Parallel()

	g := getter.NewOCIGetter(logger.CreateLogger(), vfs.NewOSFS())
	req := &gogetter.Request{Src: "ghcr.io/org/module:v1.0.0", Forced: getter.SchemeOCI}

	ok, err := g.Detect(req)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestOCIGetter_DetectIgnoresOtherSchemes(t *testing.T) {
	t.Parallel()

	g := getter.NewOCIGetter(logger.CreateLogger(), vfs.NewOSFS())

	cases := []string{
		"https://ghcr.io/org/module:v1.0.0",
		"git::https://github.com/org/repo.git",
		"tfr://registry.terraform.io/org/module/aws?version=1.0.0",
		"s3://mybucket/module.tar.gz",
	}

	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			t.Parallel()

			req := &gogetter.Request{Src: src}
			ok, err := g.Detect(req)
			require.NoError(t, err)
			assert.False(t, ok)
		})
	}
}

// ---------------------------------------------------------------------------
// Mode
// ---------------------------------------------------------------------------

func TestOCIGetter_ModeReturnsDir(t *testing.T) {
	t.Parallel()

	g := getter.NewOCIGetter(logger.CreateLogger(), vfs.NewOSFS())
	mode, err := g.Mode(context.Background(), &url.URL{})
	require.NoError(t, err)
	assert.Equal(t, gogetter.ModeDir, mode)
}

// ---------------------------------------------------------------------------
// GetFile
// ---------------------------------------------------------------------------

func TestOCIGetter_GetFileReturnsUnsupported(t *testing.T) {
	t.Parallel()

	g := getter.NewOCIGetter(logger.CreateLogger(), vfs.NewOSFS())
	err := g.GetFile(context.Background(), &gogetter.Request{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

// ---------------------------------------------------------------------------
// Get — root extraction
// ---------------------------------------------------------------------------

func TestOCIGetter_GetExtractsModuleFiles(t *testing.T) {
	t.Parallel()

	srv, _ := newOCITestServer(t, map[string]string{
		"main.tf":      `resource "null_resource" "a" {}`,
		"variables.tf": "variable \"name\" {}",
	})

	// OCIGetter detects 127.0.0.1 and sets PlainHTTP = true.
	ref := srv.Listener.Addr().String() + "/org/module:v1.0.0"
	dst := helpers.TmpDirWOSymlinks(t)

	g := getter.NewOCIGetter(logger.CreateLogger(), vfs.NewOSFS())

	err := g.Get(t.Context(), &gogetter.Request{
		Src:     "oci://" + ref,
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "main.tf"))
	require.FileExists(t, filepath.Join(dst, "variables.tf"))
}

// ---------------------------------------------------------------------------
// Get — subdir selector
// ---------------------------------------------------------------------------

func TestOCIGetter_GetExtractsSubdir(t *testing.T) {
	t.Parallel()

	srv, _ := newOCITestServer(t, map[string]string{
		"modules/vpc/main.tf":   "# vpc module",
		"modules/other/main.tf": "# other module",
	})

	ref := srv.Listener.Addr().String() + "/org/modules:v1.0.0"
	dst := helpers.TmpDirWOSymlinks(t)

	g := getter.NewOCIGetter(logger.CreateLogger(), vfs.NewOSFS())

	// The //subdir suffix is the go-getter subdirectory convention.
	err := g.Get(t.Context(), &gogetter.Request{
		Src:     "oci://" + ref + "//modules/vpc",
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "main.tf"),
		"subdir files must be rooted at the destination")
	require.NoFileExists(t, filepath.Join(dst, "modules", "other", "main.tf"),
		"sibling directories must not be present when a subdir is selected")
}

// ---------------------------------------------------------------------------
// Get — via go-getter Client (Detect → Get pipeline)
// ---------------------------------------------------------------------------

func TestOCIGetter_ClientGetPipeline(t *testing.T) {
	t.Parallel()

	srv, _ := newOCITestServer(t, map[string]string{
		"main.tf": "# module root",
	})

	ref := srv.Listener.Addr().String() + "/org/module:v1.0.0"
	dst := helpers.TmpDirWOSymlinks(t)

	client := getter.NewClient(
		getter.WithOCIRegistry(getter.NewOCIGetter(logger.CreateLogger(), vfs.NewOSFS())),
	)

	_, err := client.Get(t.Context(), &gogetter.Request{
		Src:     "oci://" + ref,
		Dst:     dst,
		GetMode: gogetter.ModeDir,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "main.tf"))
}

// ---------------------------------------------------------------------------
// CAS integration — second fetch must not re-download the blob
// ---------------------------------------------------------------------------

func TestOCIGetter_CASCachesSecondRun(t *testing.T) {
	t.Parallel()

	srv, state := newOCITestServer(t, map[string]string{
		"main.tf":   `resource "null_resource" "a" {}`,
		"README.md": "hello",
	})

	ref := srv.Listener.Addr().String() + "/org/module:v1.0.0"
	src := "oci://" + ref

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "cas-store")
	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	l := logger.CreateLogger()
	fs := vfs.NewOSFS()

	casGetter := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{},
		getter.WithDefaultGenericDispatch(
			getter.WithOCIConfig(l, fs),
		),
	)

	runGet := func(t *testing.T) {
		t.Helper()

		dst := filepath.Join(t.TempDir(), "out")
		client := &gogetter.Client{Getters: []gogetter.Getter{casGetter}}
		_, err := client.Get(t.Context(), &gogetter.Request{
			Src:     src,
			Dst:     dst,
			GetMode: gogetter.ModeAny,
		})
		require.NoError(t, err)
		require.FileExists(t, filepath.Join(dst, "main.tf"))
	}

	runGet(t) // first run: populates CAS

	firstBlobs := state.blobGets.Load()
	assert.GreaterOrEqual(t, firstBlobs, int32(1), "first run must download the blob")

	runGet(t) // second run: CAS hit via digest probe

	assert.Equal(t, firstBlobs, state.blobGets.Load(),
		"second run must hit the CAS via the digest probe and skip the blob GET")
}
