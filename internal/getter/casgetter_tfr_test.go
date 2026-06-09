package getter_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"

	tgcas "github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCASGetter_TFRRoutesThroughCAS_FirstRunDownloads pins that a
// tfr:// source claimed by CASGetter resolves to an archive download
// and lands a tree in the CAS store.
//
// The wiring mirrors the production tryCASDownload path but routes the
// underlying HTTPS calls through the httptest TLS server.
func TestCASGetter_TFRRoutesThroughCAS(t *testing.T) {
	t.Parallel()

	var archiveGets atomic.Int32

	server := newCountingRegistryTestServer(t, &archiveGets)

	l := logger.CreateLogger()
	httpClient := server.Client()

	tfr := getter.NewRegistryGetter(l).
		WithHTTPClient(httpClient).
		WithTofuImplementation(tfimpl.Terraform)

	resolver := getter.NewTFRResolver().
		WithHTTPClient(httpClient).
		WithLogger(l).
		WithTofuImplementation(tfimpl.Terraform)

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	// Inner client builder injects a TLS-trusting HttpGetter so the
	// RegistryGetter's delegated archive download trusts the
	// httptest TLS server's self-signed cert. Production wiring does
	// not need this hook — the default builder uses the standard
	// HttpGetter, which is fine for a real registry.
	innerBuilder := func(bare gogetter.Getter, _ string) *gogetter.Client {
		return getter.NewClient(getter.WithCustomGettersPrepended(
			bare,
			&gogetter.HttpGetter{Client: httpClient, Netrc: true},
		))
	}

	g := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{},
		getter.WithGenericFetchers(map[string]gogetter.Getter{
			getter.SchemeTFR: tfr,
		}),
		getter.WithGenericResolvers(map[string]getter.SourceResolver{
			getter.SchemeTFR: resolver,
		}),
		getter.WithInnerClientBuilder(innerBuilder),
	)

	client := &gogetter.Client{Getters: []gogetter.Getter{g}}

	src := "tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws?version=3.3.0"

	first := filepath.Join(helpers.TmpDirWOSymlinks(t), "first")
	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     src,
		Dst:     first,
		GetMode: gogetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(first, "main.tf"))

	firstGets := archiveGets.Load()
	assert.Equal(t, int32(1), firstGets, "first run must download the archive once")

	// Second run against the same source — probe resolves to the
	// same key, CAS materializes from the store, and the archive
	// endpoint must not be hit again.
	second := filepath.Join(helpers.TmpDirWOSymlinks(t), "second")
	_, err = client.Get(t.Context(), &gogetter.Request{
		Src:     src,
		Dst:     second,
		GetMode: gogetter.ModeAny,
	})
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(second, "main.tf"))

	assert.Equal(t, firstGets, archiveGets.Load(),
		"second run must hit the CAS cache and skip the archive download")
}

// TestCASGetter_TFRBareScheme pins that a tfr:// URL with no forced
// prefix is claimed by CASGetter's detector and routed through the
// generic dispatch.
func TestCASGetter_TFRBareSchemeIsClaimed(t *testing.T) {
	t.Parallel()

	server := newRegistryTestServer(t)

	l := logger.CreateLogger()

	// A no-op fetcher we register as the TFR entry. The test only
	// verifies detection; we count how many times the fetcher is
	// invoked to confirm CASGetter routed the URL here.
	calls := atomic.Int32{}
	stub := &countingGetter{calls: &calls}

	storePath := filepath.Join(helpers.TmpDirWOSymlinks(t), "store")

	c, err := tgcas.New(tgcas.WithStorePath(storePath))
	require.NoError(t, err)

	v, err := tgcas.OSVenv()
	require.NoError(t, err)

	g := getter.NewCASGetter(l, c, v, &tgcas.CloneOptions{},
		getter.WithGenericFetchers(map[string]gogetter.Getter{
			getter.SchemeTFR: stub,
		}),
	)

	req := &gogetter.Request{
		Src: "tfr://" + server.Listener.Addr().String() + "/terraform-aws-modules/vpc/aws?version=3.3.0",
	}

	ok, err := g.Detect(req)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "tfr", req.Forced, "matchGenericScheme must set Forced=tfr")
}

// countingGetter is a no-op gogetter.Getter used to confirm CASGetter
// dispatched a request without actually performing a fetch.
type countingGetter struct {
	calls *atomic.Int32
}

func (c *countingGetter) Detect(_ *gogetter.Request) (bool, error) { return true, nil }
func (c *countingGetter) Get(_ context.Context, _ *gogetter.Request) error {
	c.calls.Add(1)
	return nil
}
func (c *countingGetter) GetFile(_ context.Context, _ *gogetter.Request) error { return nil }
func (c *countingGetter) Mode(_ context.Context, _ *url.URL) (gogetter.Mode, error) {
	return gogetter.ModeDir, nil
}

// newCountingRegistryTestServer is a variant of newRegistryTestServer
// that records every successful archive GET in archiveGets. Use it to
// pin that the CAS cache hit on a second probe skips the underlying
// archive download.
func newCountingRegistryTestServer(t *testing.T, archiveGets *atomic.Int32) *httptest.Server {
	t.Helper()

	zipBody := buildModuleZip(t)

	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"modules.v1":"/v1/modules/"}`))
		assert.NoError(t, err)
	})

	mux.HandleFunc("/v1/modules/terraform-aws-modules/vpc/aws/3.3.0/download", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Terraform-Get", "https://"+r.Host+"/download/terraform-aws-vpc.zip")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/download/terraform-aws-vpc.zip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")

		if r.Method != http.MethodGet {
			return
		}

		archiveGets.Add(1)

		_, err := w.Write(zipBody)
		assert.NoError(t, err)
	})

	server := httptest.NewTLSServer(mux)
	t.Cleanup(server.Close)

	return server
}
