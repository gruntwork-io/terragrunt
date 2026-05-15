package getter_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cas"
	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithCASRegistersCASGetter pins the wiring: WithCAS adds a CASGetter
// ahead of the standard git getter, plus a CASProtocolGetter so `cas::`
// references produced by update_source_with_cas stack rewrites resolve.
func TestWithCASRegistersCASGetter(t *testing.T) {
	t.Parallel()

	c, err := cas.New(cas.WithStorePath(filepath.Join(helpers.TmpDirWOSymlinks(t), "store")))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	client := getter.NewClient(getter.WithCAS(c, v, &cas.CloneOptions{}))

	assert.True(t, hasGetter[*getter.CASGetter](client.Getters), "WithCAS must register CASGetter")
	assert.True(t, hasGetter[*getter.CASProtocolGetter](client.Getters), "WithCAS must register CASProtocolGetter")
}

// TestWithCASRoutesCASProtocolURLs pins that a `cas::` source URL routes to
// CASProtocolGetter rather than falling through to a getter that can't handle
// it. The CASProtocolGetter is registered earlier in the chain than CASGetter
// so its forced-prefix Detect wins on the first match.
func TestWithCASRoutesCASProtocolURLs(t *testing.T) {
	t.Parallel()

	c, err := cas.New(cas.WithStorePath(filepath.Join(helpers.TmpDirWOSymlinks(t), "store")))
	require.NoError(t, err)

	v, err := cas.OSVenv()
	require.NoError(t, err)

	client := getter.NewClient(getter.WithCAS(c, v, &cas.CloneOptions{}))

	req := &getter.Request{Src: "cas::sha1:0000000000000000000000000000000000000000"}

	for _, g := range client.Getters {
		ok, err := g.Detect(req)
		require.NoError(t, err)

		if !ok {
			continue
		}

		_, isProtocolGetter := g.(*getter.CASProtocolGetter)
		assert.True(t, isProtocolGetter, "first getter to match cas:: must be CASProtocolGetter, got %T", g)

		return
	}

	t.Fatal("no getter matched cas:: source")
}

// TestWithHTTPSAuthHeaderReachesServer verifies WithHTTPSAuth wires its
// extra headers onto the https getter the client uses for downloads.
func TestWithHTTPSAuthHeaderReachesServer(t *testing.T) {
	t.Parallel()

	const want = "Bearer https-token"

	var got string

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("Authorization")
		_, err := w.Write([]byte("ok"))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)

	client := getter.NewClient(
		getter.WithHTTPSAuth(http.Header{"Authorization": {want}}),
		getter.WithCustomGettersPrepended(&gogetter.HttpGetter{
			Client: server.Client(),
			Header: http.Header{"Authorization": {want}},
			Netrc:  true,
		}),
	)

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "out")
	_, err := client.Get(t.Context(), &getter.Request{
		Src:     server.URL + "/blob",
		Dst:     dst,
		GetMode: getter.ModeFile,
	})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestHTTPSchemeRoutingChoosesAuthSlot pins the per-scheme auth routing.
// Without HTTPSchemeGetter wrapping each HttpGetter, the upstream
// HttpGetter.Detect matches both http and https, so the http slot (no auth)
// shadowed the https slot (with auth) for every https URL.
func TestHTTPSchemeRoutingChoosesAuthSlot(t *testing.T) {
	t.Parallel()

	httpsHeader := http.Header{"X-Auth": {"https-token"}}
	httpHeader := http.Header{"X-Auth": {"http-token"}}

	client := getter.NewClient(
		getter.WithHTTPSAuth(httpsHeader),
		getter.WithHTTPAuth(httpHeader),
	)

	tests := []struct {
		name string
		src  string
		want string
	}{
		{name: "https routes to https slot", src: "https://example.com/blob", want: "https-token"},
		{name: "http routes to http slot", src: "http://example.com/blob", want: "http-token"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := firstHTTPMatcher(t, client.Getters, tc.src)
			assert.Equal(t, tc.want, got.Inner.Header.Get("X-Auth"))
		})
	}
}

// firstHTTPMatcher walks the client's getter chain and returns the first
// HTTPSchemeGetter whose Detect matches src.
func firstHTTPMatcher(t *testing.T, getters []getter.Getter, src string) *getter.HTTPSchemeGetter {
	t.Helper()

	req := &getter.Request{Src: src}

	for _, g := range getters {
		ok, err := g.Detect(req)
		require.NoError(t, err)

		if !ok {
			continue
		}

		h, isHTTP := g.(*getter.HTTPSchemeGetter)
		require.True(t, isHTTP, "first matcher for %s was %T, expected *HTTPSchemeGetter", src, g)

		return h
	}

	require.Fail(t, "no getter matched "+src)

	return nil
}

// TestWithHTTPSAuthSetsBuilderField pins the option-to-builder wiring
// without depending on TLS. The internal HTTPGetter constructed for https
// downloads carries the extra headers.
func TestWithHTTPSAuthSetsBuilderField(t *testing.T) {
	t.Parallel()

	header := http.Header{"X-Test": {"yes"}}
	client := getter.NewClient(getter.WithHTTPSAuth(header))

	httpGetters := allHTTPGetters(client.Getters)
	require.Len(t, httpGetters, 2, "client must register both http and https getters")

	// The https getter (second in registration order) carries the auth header.
	assert.Equal(t, "yes", httpGetters[1].Header.Get("X-Test"))
	assert.Empty(t, httpGetters[0].Header.Get("X-Test"), "plain http getter must not carry the https-only auth")
}

// TestFileCopyGetIncludeExcludeFiltersHonor pins the include/exclude glob
// configuration end-to-end against the FileCopyGetter Get path.
func TestFileCopyGetIncludeExcludeFiltersHonor(t *testing.T) {
	t.Parallel()

	src := helpers.TmpDirWOSymlinks(t)
	require.NoError(t, writeFile(filepath.Join(src, "main.tf"), "# main\n"))
	require.NoError(t, writeFile(filepath.Join(src, "secret.txt"), "shh\n"))

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "out")
	fcg := getter.NewFileCopyGetter().
		WithLogger(logger.CreateLogger()).
		WithExcludeFromCopy("*.txt")

	client := getter.NewClient(getter.WithFileCopy(fcg))

	_, err := client.Get(t.Context(), &getter.Request{
		Src:     "file://" + src,
		Dst:     dst,
		GetMode: getter.ModeDir,
	})
	require.NoError(t, err)

	assert.FileExists(t, filepath.Join(dst, "main.tf"))
	assert.NoFileExists(t, filepath.Join(dst, "secret.txt"))
}

// TestFileCopyGetMissingPath pins the user-facing error from the Get path
// when the requested source directory does not exist.
func TestFileCopyGetMissingPath(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(helpers.TmpDirWOSymlinks(t), "does-not-exist")

	client := getter.NewClient(getter.WithFileCopy(getter.NewFileCopyGetter()))
	_, err := client.Get(t.Context(), &getter.Request{
		Src:     "file://" + missing,
		Dst:     filepath.Join(helpers.TmpDirWOSymlinks(t), "out"),
		GetMode: getter.ModeDir,
	})
	require.Error(t, err)
}

// TestFileCopyGetSourceIsFile pins the contract that FileCopyGetter only
// handles directories. A file source is rejected explicitly so callers
// don't end up with a half-initialized destination directory.
func TestFileCopyGetSourceIsFile(t *testing.T) {
	t.Parallel()

	srcFile := filepath.Join(helpers.TmpDirWOSymlinks(t), "main.tf")
	require.NoError(t, writeFile(srcFile, "# main\n"))

	client := getter.NewClient(getter.WithFileCopy(getter.NewFileCopyGetter()))
	_, err := client.Get(t.Context(), &getter.Request{
		Src:     "file://" + srcFile,
		Dst:     filepath.Join(helpers.TmpDirWOSymlinks(t), "out"),
		GetMode: getter.ModeDir,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, getter.ErrSourceNotADirectory)
}

// TestFileCopyGetFileDelegates pins the GetFile passthrough so a future
// change can't silently drop the file-copy semantics.
func TestFileCopyGetFileDelegates(t *testing.T) {
	t.Parallel()

	srcDir := helpers.TmpDirWOSymlinks(t)
	srcFile := filepath.Join(srcDir, "main.tf")
	require.NoError(t, writeFile(srcFile, "# main\n"))

	dst := filepath.Join(helpers.TmpDirWOSymlinks(t), "out.tf")

	client := getter.NewClient(getter.WithFileCopy(getter.NewFileCopyGetter()))
	_, err := client.Get(t.Context(), &getter.Request{
		Src:     "file://" + srcFile,
		Dst:     dst,
		GetMode: getter.ModeFile,
	})
	require.NoError(t, err)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "# main\n", string(got))
}

// allHTTPGetters returns the underlying HttpGetter values registered on the
// client in registration order, transparently unwrapping the
// HTTPSchemeGetter that buildGetters uses to bind each instance to one
// scheme.
func allHTTPGetters(getters []getter.Getter) []*gogetter.HttpGetter {
	out := make([]*gogetter.HttpGetter, 0, 2)

	for _, g := range getters {
		switch h := g.(type) {
		case *gogetter.HttpGetter:
			out = append(out, h)
		case *getter.HTTPSchemeGetter:
			if h.Inner != nil {
				out = append(out, h.Inner)
			}
		}
	}

	return out
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0644)
}
