package helpers_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/helpers"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// notGzip is longer than a gzip header, so decoding it fails on the header itself.
const notGzip = "this body is not gzip encoded"

func TestFetchFailures(t *testing.T) {
	t.Parallel()

	cut := errors.New("connection reset")

	testCases := []struct {
		respond   vhttp.Handler
		assertErr func(t *testing.T, err error)
		name      string
	}{
		{
			name: "transport failure",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return nil, cut
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, cut)
			},
		},
		{
			name: "status other than ok",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return vhttp.Respond(http.StatusNotFound, nil, nil), nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.Error(t, err)
			},
		},
		{
			name: "body that is not gzip despite its encoding",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				return vhttp.Respond(http.StatusOK, []byte(notGzip), http.Header{"Content-Encoding": []string{"gzip"}}), nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, gzip.ErrHeader)
			},
		},
		{
			name: "transfer cut short",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				resp := vhttp.Respond(http.StatusOK, nil, nil)
				resp.Body = io.NopCloser(errAfter("partial", cut))
				resp.ContentLength = -1

				return resp, nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, cut)
			},
		},
		{
			name: "body shorter than its content length",
			respond: func(context.Context, *http.Request) (*http.Response, error) {
				resp := vhttp.Respond(http.StatusOK, []byte("short"), nil)
				resp.ContentLength = 100

				return resp, nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.Error(t, err)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://registry.test/archive.zip", nil)
			require.NoError(t, err)

			var dst bytes.Buffer

			tc.assertErr(t, helpers.Fetch(t.Context(), vhttp.NewMemClient(tc.respond), req, &dst, testLimit))
		})
	}
}

func TestFetchToFileWritesTheBody(t *testing.T) {
	t.Parallel()

	const body = "provider archive"

	fsys := vfs.NewMemMapFS()
	c := vhttp.NewMemClient(func(context.Context, *http.Request) (*http.Response, error) {
		return vhttp.Respond(http.StatusOK, []byte(body), nil), nil
	})

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://registry.test/archive.zip", nil)
	require.NoError(t, err)

	require.NoError(t, helpers.FetchToFile(t.Context(), c, fsys, req, "/archive.zip", testLimit))

	got, err := vfs.ReadFile(fsys, "/archive.zip")
	require.NoError(t, err)
	assert.Equal(t, body, string(got), "the file should hold the response body")
}

func TestFetchToFileFailures(t *testing.T) {
	t.Parallel()

	errSync := errors.New("sync failed")
	errClose := errors.New("close failed")

	okClient := vhttp.NewMemClient(func(context.Context, *http.Request) (*http.Response, error) {
		return vhttp.Respond(http.StatusOK, []byte("provider archive"), nil), nil
	})

	testCases := []struct {
		fsys      vfs.FS
		client    vhttp.Client
		assertErr func(t *testing.T, err error)
		name      string
	}{
		{
			name:   "destination cannot be created",
			fsys:   faultyFileFS{FS: vfs.NewMemMapFS(), createErr: os.ErrPermission},
			client: vhttp.NewNoNetworkClient(),
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, os.ErrPermission)
			},
		},
		{
			name:   "fetch fails",
			fsys:   vfs.NewMemMapFS(),
			client: vhttp.NewNoNetworkClient(),
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, vhttp.ErrNoNetwork)
			},
		},
		{
			name:   "file fails to sync",
			fsys:   faultyFileFS{FS: vfs.NewMemMapFS(), syncErr: errSync},
			client: okClient,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errSync)
			},
		},
		{
			name:   "file fails to close",
			fsys:   faultyFileFS{FS: vfs.NewMemMapFS(), closeErr: errClose},
			client: okClient,
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errClose)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://registry.test/archive.zip", nil)
			require.NoError(t, err)

			tc.assertErr(t, helpers.FetchToFile(t.Context(), tc.client, tc.fsys, req, "/archive.zip", testLimit))
		})
	}
}

func TestResponseBufferFailures(t *testing.T) {
	t.Parallel()

	cut := errors.New("connection reset")

	testCases := []struct {
		body     io.Reader
		wantErr  error
		name     string
		encoding string
	}{
		{
			name:     "body that is not gzip despite its encoding",
			body:     strings.NewReader(notGzip),
			encoding: "gzip",
			wantErr:  gzip.ErrHeader,
		},
		{
			name:    "transfer cut short",
			body:    errAfter("partial", cut),
			wantErr: cut,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := vhttp.Respond(http.StatusOK, nil, http.Header{})
			defer func() { require.NoError(t, resp.Body.Close()) }()

			resp.Body = io.NopCloser(tc.body)
			if tc.encoding != "" {
				resp.Header.Set("Content-Encoding", tc.encoding)
			}

			buffer, err := helpers.ResponseBuffer(resp)

			require.ErrorIs(t, err, tc.wantErr)
			assert.Nil(t, buffer)
		})
	}
}

func TestModifyJSONBodyRewritesTheBody(t *testing.T) {
	t.Parallel()

	resp := vhttp.Respond(http.StatusOK, []byte(`{"name":"aws"}`), http.Header{})
	defer func() { require.NoError(t, resp.Body.Close()) }()

	var data map[string]string

	err := helpers.ModifyJSONBody(resp, &data, func() error {
		data["name"] = "aws-renamed"

		return nil
	})
	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.JSONEq(t, `{"name":"aws-renamed"}`, string(body), "the body should carry the modified value")
	assert.Equal(t, int64(len(body)), resp.ContentLength, "content length should match the new body")
	assert.Equal(t, strconv.Itoa(len(body)), resp.Header.Get("Content-Length"))
}

func TestModifyJSONBodyLeavesNonOKResponse(t *testing.T) {
	t.Parallel()

	resp := vhttp.Respond(http.StatusNotFound, []byte("not json"), http.Header{})
	defer func() { require.NoError(t, resp.Body.Close()) }()

	var called bool

	require.NoError(t, helpers.ModifyJSONBody(resp, new(map[string]string), func() error {
		called = true

		return nil
	}))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, "not json", string(body), "a failed response should pass through untouched")
	assert.False(t, called, "the modify func should not run for a failed response")
}

func TestModifyJSONBodyDecodesWithoutModifyFunc(t *testing.T) {
	t.Parallel()

	resp := vhttp.Respond(http.StatusOK, []byte(`{"name":"aws"}`), http.Header{})
	defer func() { require.NoError(t, resp.Body.Close()) }()

	var data map[string]string

	require.NoError(t, helpers.ModifyJSONBody(resp, &data, nil))
	assert.Equal(t, map[string]string{"name": "aws"}, data)
}

func TestModifyJSONBodyFailures(t *testing.T) {
	t.Parallel()

	errModify := errors.New("modify failed")

	testCases := []struct {
		modify    func(data map[string]any) error
		assertErr func(t *testing.T, err error)
		name      string
		body      string
		encoding  string
	}{
		{
			name:     "body that cannot be read",
			body:     notGzip,
			encoding: "gzip",
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, gzip.ErrHeader)
			},
		},
		{
			name: "body that is not json",
			body: "not json",
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var syntaxErr *json.SyntaxError

				require.ErrorAs(t, err, &syntaxErr)
			},
		},
		{
			name: "modify func fails",
			body: `{"name":"aws"}`,
			modify: func(map[string]any) error {
				return errModify
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				require.ErrorIs(t, err, errModify)
			},
		},
		{
			name: "modified value cannot be encoded",
			body: `{"name":"aws"}`,
			modify: func(data map[string]any) error {
				data["name"] = make(chan int)

				return nil
			},
			assertErr: func(t *testing.T, err error) {
				t.Helper()

				var unsupported *json.UnsupportedTypeError

				require.ErrorAs(t, err, &unsupported)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			if tc.encoding != "" {
				header.Set("Content-Encoding", tc.encoding)
			}

			resp := vhttp.Respond(http.StatusOK, []byte(tc.body), header)
			defer func() { require.NoError(t, resp.Body.Close()) }()

			var data map[string]any

			modify := func() error {
				if tc.modify == nil {
					return nil
				}

				return tc.modify(data)
			}

			tc.assertErr(t, helpers.ModifyJSONBody(resp, &data, modify))
		})
	}
}

// faultyFileFS fails Create, or creates files whose Sync and Close fail, with the configured errors.
type faultyFileFS struct {
	vfs.FS

	createErr error
	syncErr   error
	closeErr  error
}

func (fsys faultyFileFS) Create(name string) (vfs.File, error) {
	if fsys.createErr != nil {
		return nil, fsys.createErr
	}

	f, err := fsys.FS.Create(name)
	if err != nil {
		return nil, err
	}

	return faultyFile{File: f, syncErr: fsys.syncErr, closeErr: fsys.closeErr}, nil
}

// faultyFile is a file whose Sync and Close return the configured errors.
type faultyFile struct {
	vfs.File

	syncErr  error
	closeErr error
}

func (f faultyFile) Sync() error {
	return f.syncErr
}

func (f faultyFile) Close() error {
	return errors.Join(f.File.Close(), f.closeErr)
}
