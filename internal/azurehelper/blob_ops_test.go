//go:build azure

package azurehelper_test

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

func TestBlobClient_CopyBlob_StreamsThroughClient(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodGet, pathSub: "/srcc/src.tfstate", status: http.StatusOK, body: "state-bytes"},
		{method: http.MethodPut, pathSub: "/dstc/dst.tfstate", status: http.StatusCreated},
	}}
	c := newRoutedBlobClient(t, rt)

	require.NoError(t, c.CopyBlob(t.Context(), "srcc", "src.tfstate", "dstc", "dst.tfstate"))

	assert.True(t, rt.sawMethodOnPath(http.MethodGet, "/srcc/src.tfstate"), "source must be downloaded through the client")
	assert.True(t, rt.sawMethodOnPath(http.MethodPut, "/dstc/dst.tfstate"), "destination must be uploaded through the client")
	assert.True(t, rt.sawBodyOnPath(http.MethodPut, "/dstc/dst.tfstate", "state-bytes"), "upload must carry the downloaded payload")

	// No x-ms-copy-source means no server-side copy, so private containers work.
	for _, r := range rt.requests() {
		assert.Emptyf(t, r.copySource, "request %s %s must not use server-side copy", r.method, r.path)
	}
}

func TestBlobClient_CopyBlob_SourceMissing(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodGet, pathSub: "/srcc/src.tfstate", status: http.StatusNotFound, code: "BlobNotFound"},
	}}
	c := newRoutedBlobClient(t, rt)

	err := c.CopyBlob(t.Context(), "srcc", "src.tfstate", "dstc", "dst.tfstate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "downloading blob")
}

func TestBlobClient_MoveBlobIfNecessary_NoopWhenSourceAbsent(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodHead, pathSub: "/srcc/src.tfstate", status: http.StatusNotFound, code: "BlobNotFound"},
	}}
	c := newRoutedBlobClient(t, rt)

	require.NoError(t, c.MoveBlobIfNecessary(t.Context(), "srcc", "src.tfstate", "dstc", "dst.tfstate"))

	assert.False(t, rt.sawMethodOnPath(http.MethodGet, ""), "absent source must not be downloaded")
	assert.False(t, rt.sawMethodOnPath(http.MethodPut, ""), "absent source must not trigger an upload")
	assert.False(t, rt.sawMethodOnPath(http.MethodDelete, ""), "absent source must not trigger a delete")
}

func TestBlobClient_MoveBlobIfNecessary_RefusesExistingDestination(t *testing.T) {
	t.Parallel()

	// The destination write is conditional, so an existing blob answers 409.
	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodHead, pathSub: "/srcc/src.tfstate", status: http.StatusOK},
		{method: http.MethodGet, pathSub: "/srcc/src.tfstate", status: http.StatusOK, body: "state-bytes"},
		{method: http.MethodPut, pathSub: "/dstc/dst.tfstate", status: http.StatusConflict, code: "BlobAlreadyExists"},
	}}
	c := newRoutedBlobClient(t, rt)

	err := c.MoveBlobIfNecessary(t.Context(), "srcc", "src.tfstate", "dstc", "dst.tfstate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")

	assert.False(t, rt.sawMethodOnPath(http.MethodDelete, ""), "source must survive a refused move")
}

func TestBlobClient_MoveBlobIfNecessary_MovesThenDeletesSource(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodHead, pathSub: "/srcc/src.tfstate", status: http.StatusOK},
		{method: http.MethodGet, pathSub: "/srcc/src.tfstate", status: http.StatusOK, body: "state-bytes"},
		{method: http.MethodPut, pathSub: "/dstc/dst.tfstate", status: http.StatusCreated},
		{method: http.MethodDelete, pathSub: "/srcc/src.tfstate", status: http.StatusAccepted},
	}}
	c := newRoutedBlobClient(t, rt)

	require.NoError(t, c.MoveBlobIfNecessary(t.Context(), "srcc", "src.tfstate", "dstc", "dst.tfstate"))

	getIdx := rt.firstIndexOf(http.MethodGet, "/srcc/src.tfstate")
	putIdx := rt.firstIndexOf(http.MethodPut, "/dstc/dst.tfstate")
	delIdx := rt.firstIndexOf(http.MethodDelete, "/srcc/src.tfstate")

	require.GreaterOrEqual(t, getIdx, 0, "source must be downloaded")
	require.GreaterOrEqual(t, putIdx, 0, "destination must be written")
	require.GreaterOrEqual(t, delIdx, 0, "source must be deleted")

	// Delete-before-copy would lose the state file, so the order is load-bearing.
	assert.Less(t, getIdx, putIdx, "download must precede the upload")
	assert.Less(t, putIdx, delIdx, "source delete must come after the copy is written")

	// The conditional header is what prevents overwriting a concurrent writer.
	assert.Equal(t, "*", rt.requests()[putIdx].ifNoneMatch, "destination write must carry If-None-Match")
}

func TestBlobClient_MoveBlobIfNecessary_KeepsSourceWhenCopyFails(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodHead, pathSub: "/srcc/src.tfstate", status: http.StatusOK},
		{method: http.MethodGet, pathSub: "/srcc/src.tfstate", status: http.StatusOK, body: "state-bytes"},
		{method: http.MethodPut, pathSub: "/dstc/dst.tfstate", status: http.StatusForbidden, code: "AuthorizationFailure"},
	}}
	c := newRoutedBlobClient(t, rt)

	require.Error(t, c.MoveBlobIfNecessary(t.Context(), "srcc", "src.tfstate", "dstc", "dst.tfstate"))
	assert.False(t, rt.sawMethodOnPath(http.MethodDelete, ""), "source must survive a failed copy")
}

func TestBlobClient_ContainerExists_Stubbed(t *testing.T) {
	t.Parallel()

	t.Run("exists", func(t *testing.T) {
		t.Parallel()

		rt := &routeTransport{routes: []stubRoute{
			{method: http.MethodGet, pathSub: "/somec", status: http.StatusOK},
		}}
		c := newRoutedBlobClient(t, rt)

		exists, err := c.ContainerExists(t.Context(), "somec")
		require.NoError(t, err)
		assert.True(t, exists)

		// The existence check must stay a read; a write here would break read-only credentials.
		assert.True(t, rt.sawMethodOnPath(http.MethodGet, "/somec"), "existence check must read container properties")
		assert.False(t, rt.sawMethodOnPath(http.MethodPut, ""), "existence check must not write")
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()

		rt := &routeTransport{routes: []stubRoute{
			{method: http.MethodGet, pathSub: "/somec", status: http.StatusNotFound, code: "ContainerNotFound"},
		}}
		c := newRoutedBlobClient(t, rt)

		exists, err := c.ContainerExists(t.Context(), "somec")
		require.NoError(t, err)
		assert.False(t, exists)
		assert.False(t, rt.sawMethodOnPath(http.MethodPut, ""), "existence check must not write")
	})
}

func TestBlobClient_EnsureBlobDeleted_IdempotentWhenMissing(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodDelete, pathSub: "/somec/gone.tfstate", status: http.StatusNotFound, code: "BlobNotFound"},
	}}
	c := newRoutedBlobClient(t, rt)

	require.NoError(t, c.EnsureBlobDeleted(t.Context(), "somec", "gone.tfstate"))
}

func TestBlobClient_CreateContainer_ToleratesExisting(t *testing.T) {
	t.Parallel()

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodPut, pathSub: "/dupc", status: http.StatusConflict, code: "ContainerAlreadyExists"},
	}}
	c := newRoutedBlobClient(t, rt)

	require.NoError(t, c.CreateContainer(t.Context(), "dupc"))
}

func TestBlobClient_ListBlobs_WithPrefix(t *testing.T) {
	t.Parallel()

	const listing = `<?xml version="1.0" encoding="utf-8"?>` +
		`<EnumerationResults><Blobs><Blob><Name>state/a.tfstate</Name></Blob></Blobs><NextMarker /></EnumerationResults>`

	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodGet, pathSub: "/prefc", status: http.StatusOK, body: listing},
	}}
	c := newRoutedBlobClient(t, rt)

	names, err := c.ListBlobs(t.Context(), log.New(), "prefc", azurehelper.WithPrefix("state/"))
	require.NoError(t, err)
	assert.Equal(t, []string{"state/a.tfstate"}, names)

	reqs := rt.requests()
	require.NotEmpty(t, reqs)
	assert.Contains(t, reqs[0].query, "prefix=state", "prefix option must reach the list request")
}

func TestBlobClient_EnsureContainer_CreatesWhenMissing(t *testing.T) {
	t.Parallel()

	// The PUT route must come first so the existence probe falls through to 404.
	rt := &routeTransport{routes: []stubRoute{
		{method: http.MethodPut, pathSub: "/newc", status: http.StatusCreated},
		{pathSub: "/newc", status: http.StatusNotFound, code: "ContainerNotFound"},
	}}
	c := newRoutedBlobClient(t, rt)

	require.NoError(t, c.EnsureContainer(t.Context(), "newc"))

	probeIdx := rt.firstIndexOf(http.MethodGet, "/newc")
	createIdx := rt.firstIndexOf(http.MethodPut, "/newc")

	require.GreaterOrEqual(t, probeIdx, 0, "existence must be probed before creating")
	require.GreaterOrEqual(t, createIdx, 0, "missing container must be created")
	assert.Less(t, probeIdx, createIdx, "probe must precede the create")
}

// stubRoute describes one stubbed response, matched by method and URL path
// substring; empty fields match anything.
type stubRoute struct {
	method  string
	pathSub string
	code    string
	body    string
	status  int
}

// recordedRequest captures the request facts the tests assert on.
type recordedRequest struct {
	method      string
	path        string
	query       string
	copySource  string
	ifNoneMatch string
	body        string
}

// routeTransport is a policy.Transporter that answers each request with the
// first matching route and records every request for later assertions.
type routeTransport struct {
	routes []stubRoute
	reqs   []recordedRequest
	mu     sync.Mutex
}

func (rt *routeTransport) Do(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.reqs = append(rt.reqs, recordedRequest{
		method:      req.Method,
		path:        req.URL.Path,
		query:       req.URL.RawQuery,
		copySource:  req.Header.Get("x-ms-copy-source"),
		ifNoneMatch: req.Header.Get("If-None-Match"),
		body:        readRequestBody(req),
	})

	for _, r := range rt.routes {
		if r.method != "" && r.method != req.Method {
			continue
		}

		if r.pathSub != "" && !strings.Contains(req.URL.Path, r.pathSub) {
			continue
		}

		return stubResponse(req, r), nil
	}

	// 400 is terminal for the SDK retry policy, so unmatched requests fail fast.
	return stubResponse(req, stubRoute{status: http.StatusBadRequest, code: "UnmatchedTestRequest"}), nil
}

// requests returns a snapshot of the recorded requests.
func (rt *routeTransport) requests() []recordedRequest {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	return slices.Clone(rt.reqs)
}

// sawMethodOnPath reports whether any recorded request used method on a path
// containing pathSub; an empty pathSub matches any path.
func (rt *routeTransport) sawMethodOnPath(method, pathSub string) bool {
	return rt.firstIndexOf(method, pathSub) >= 0
}

// firstIndexOf returns the arrival index of the first request matching method
// and pathSub, or -1 when none matched.
func (rt *routeTransport) firstIndexOf(method, pathSub string) int {
	return slices.IndexFunc(rt.requests(), func(r recordedRequest) bool {
		return r.method == method && strings.Contains(r.path, pathSub)
	})
}

// sawBodyOnPath reports whether any recorded request used method on a path
// containing pathSub with exactly the given body.
func (rt *routeTransport) sawBodyOnPath(method, pathSub, body string) bool {
	return slices.ContainsFunc(rt.requests(), func(r recordedRequest) bool {
		return r.method == method && strings.Contains(r.path, pathSub) && r.body == body
	})
}

// readRequestBody drains and returns the request body, empty when absent.
func readRequestBody(req *http.Request) string {
	if req.Body == nil {
		return ""
	}

	b, err := io.ReadAll(req.Body)
	if err != nil {
		return ""
	}

	return string(b)
}

func stubResponse(req *http.Request, r stubRoute) *http.Response {
	header := http.Header{"Content-Type": []string{"application/json"}}
	if r.code != "" {
		header.Set("x-ms-error-code", r.code)
	}

	return &http.Response{
		Request:    req,
		StatusCode: r.status,
		Status:     http.StatusText(r.status),
		Body:       io.NopCloser(strings.NewReader(r.body)),
		Header:     header,
	}
}

func newRoutedBlobClient(t *testing.T, rt *routeTransport) *azurehelper.BlobClient {
	t.Helper()

	c, err := azurehelper.NewBlobClient(cfgWithTransport(rt))
	require.NoError(t, err, "NewBlobClient")

	return c
}
