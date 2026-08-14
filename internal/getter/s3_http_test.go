package getter_test

import (
	"context"
	"encoding/xml"
	"io/fs"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
)

// s3StubBucket is the object store a stubbed S3 answers from: key to content.
type s3StubBucket map[string]string

// s3ListResult mirrors the ListObjectsV2 response the SDK parses.
type s3ListResult struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Name     string   `xml:"Name"`
	Prefix   string   `xml:"Prefix"`
	Contents []struct {
		Key  string `xml:"Key"`
		Size int    `xml:"Size"`
	} `xml:"Contents"`
	KeyCount    int  `xml:"KeyCount"`
	MaxKeys     int  `xml:"MaxKeys"`
	IsTruncated bool `xml:"IsTruncated"`
}

// s3StubHandler answers ListObjectsV2 and GetObject out of bucket, so a fetch
// runs end to end without a network or credentials. Keys list in sorted order,
// which is the order S3 itself returns and what the mode scan reads.
func s3StubHandler(t *testing.T, bucket s3StubBucket) vhttp.Handler {
	t.Helper()

	return func(_ context.Context, req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("list-type") == "2" {
			prefix := req.URL.Query().Get("prefix")

			keys := make([]string, 0, len(bucket))

			for key := range bucket {
				if strings.HasPrefix(key, prefix) {
					keys = append(keys, key)
				}
			}

			sort.Strings(keys)

			var result s3ListResult
			for _, key := range keys {
				result.Contents = append(result.Contents, struct {
					Key  string `xml:"Key"`
					Size int    `xml:"Size"`
				}{Key: key, Size: len(bucket[key])})
			}

			result.Name = "bucket"
			result.Prefix = prefix
			result.KeyCount = len(keys)
			result.MaxKeys = len(keys)

			body, err := xml.Marshal(result)
			require.NoError(t, err)

			return vhttp.Respond(http.StatusOK, body, nil), nil
		}

		key := strings.TrimPrefix(req.URL.Path, "/bucket/")

		content, ok := bucket[key]
		if !ok {
			return vhttp.Respond(http.StatusNotFound, []byte(""), nil), nil
		}

		return vhttp.Respond(http.StatusOK, []byte(content), nil), nil
	}
}

// s3StubSrc addresses key on the stubbed service. Credentials in the query
// pin the endpoint to that host, so the SDK signs with them instead of
// resolving the ambient credential chain.
func s3StubSrc(key string) string {
	return "s3::https://s3.stub.example/bucket/" + key +
		"?aws_access_key_id=stub&aws_access_key_secret=stub&region=us-east-1"
}

// TestS3GetterFetchesThroughVenvClient drives the s3 getter over an in-memory
// HTTP client: the mode scan, the listing, and every object body ride the
// venv's client, and the results land on the venv's filesystem. The bucket
// holds an object that is also a prefix, and a sibling whose name starts with
// that object's name, which is what separates the two modes.
func TestS3GetterFetchesThroughVenvClient(t *testing.T) {
	t.Parallel()

	bucket := s3StubBucket{
		"modules/vpc":                 "vpc-object",
		"modules/vpc/":                "",
		"modules/vpc/main.tf":         "main",
		"modules/vpc/sub/nested.tf":   "nested",
		"modules/vpc-old/main.tf":     "old",
		"modules/app/main.tf":         "app-main",
		"modules/app-old/main.tf":     "app-old",
		"modules/esc/../../escape.tf": "escape",
	}

	tests := []struct {
		want    map[string]string
		wantErr error
		name    string
		key     string
	}{
		{
			// The sibling is what the mode scan reaches after the exact key,
			// so this fails the moment a listed name that merely starts with
			// the key is read as proof of a directory.
			name: "key naming an object downloads that object",
			key:  "modules/vpc",
			want: map[string]string{"vpc": "vpc-object"},
		},
		{
			name: "prefix without a trailing separator downloads the tree",
			key:  "modules/app",
			want: map[string]string{"main.tf": "app-main"},
		},
		{
			name: "prefix with a trailing separator downloads the tree",
			key:  "modules/vpc/",
			want: map[string]string{
				"main.tf":       "main",
				"sub/nested.tf": "nested",
			},
		},
		{
			name:    "key escaping the destination is refused",
			key:     "modules/esc/",
			wantErr: getter.ErrObjectEscapesDst,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := &venv.Venv{
				FS:   vfs.NewMemMapFS(),
				HTTP: vhttp.NewMemClient(s3StubHandler(t, bucket)),
				Env:  map[string]string{},
			}

			client := &getter.Client{Getters: []getter.Getter{getter.NewS3Getter(v)}}
			dst := filepath.Join("out", "module")

			_, err := client.Get(t.Context(), &getter.Request{
				Src:     s3StubSrc(tc.key),
				Dst:     dst,
				GetMode: getter.ModeAny,
			})

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assertDownloadedTree(t, v.FS, dst, tc.want)
		})
	}
}

// assertDownloadedTree compares everything under dst against want, which is
// keyed by slash-separated path relative to dst. Comparing the whole tree
// rather than each wanted file catches a sibling key or a directory
// placeholder that landed alongside the requested objects.
func assertDownloadedTree(t *testing.T, fsys vfs.FS, dst string, want map[string]string) {
	t.Helper()

	got := map[string]string{}

	require.NoError(t, vfs.WalkDir(fsys, dst, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		content, err := vfs.ReadFile(fsys, path)
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dst, path)
		if err != nil {
			return err
		}

		got[filepath.ToSlash(rel)] = string(content)

		return nil
	}))

	assert.Equal(t, want, got)
}
