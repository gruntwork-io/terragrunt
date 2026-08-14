package getter_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/url"
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

// gcsStubBucket is the object store a stubbed GCS answers from: name to
// content.
type gcsStubBucket map[string]string

// gcsTokenScopes returns the scopes the assertion in a token request asks for.
func gcsTokenScopes(t *testing.T, req *http.Request) string {
	t.Helper()

	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)

	form, err := url.ParseQuery(string(body))
	require.NoError(t, err)

	segments := strings.Split(form.Get("assertion"), ".")
	require.Len(t, segments, 3, "assertion must be a JWT")

	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	require.NoError(t, err)

	var claims struct {
		Scope string `json:"scope"`
	}

	require.NoError(t, json.Unmarshal(payload, &claims))

	return claims.Scope
}

// TestGCSGetterRequestsStorageScopes pins the scopes the token exchange asks
// for. Building the transport is what resolves the credentials, so the storage
// scopes have to be named there; asking for none leaves the provider rejecting
// the exchange, which no listing or download fixture would reveal.
func TestGCSGetterRequestsStorageScopes(t *testing.T) {
	t.Parallel()

	var scopes string

	v := stubGCPVenv(t, func(ctx context.Context, req *http.Request) (*http.Response, error) {
		if req.URL.Host == "oauth2.googleapis.com" {
			scopes = gcsTokenScopes(t, req)
		}

		return gcsStubHandler(t, gcsStubBucket{"mod.txt": "body"})(ctx, req)
	})

	client := &getter.Client{Getters: []getter.Getter{getter.NewGCSGetter(v)}}

	_, err := client.Get(t.Context(), &getter.Request{
		Src:     "gcs::https://www.googleapis.com/storage/v1/bucket/mod.txt",
		Dst:     filepath.Join("out", "module"),
		GetMode: getter.ModeAny,
	})
	require.NoError(t, err)

	assert.Contains(t, scopes, "https://www.googleapis.com/auth/devstorage.full_control")
	assert.Contains(t, scopes, "https://www.googleapis.com/auth/cloud-platform")
}

// gcsStubHandler answers the token exchange, the objects listing, and each
// object download out of bucket, so a fetch runs end to end without a network.
// Names list in sorted order, which is the order GCS itself returns and what
// the mode scan reads.
func gcsStubHandler(t *testing.T, bucket gcsStubBucket) vhttp.Handler {
	t.Helper()

	return func(_ context.Context, req *http.Request) (*http.Response, error) {
		jsonHeader := http.Header{"Content-Type": {"application/json"}}

		if req.URL.Host == "oauth2.googleapis.com" {
			body, err := json.Marshal(map[string]any{
				"access_token": "stub-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			require.NoError(t, err)

			return vhttp.Respond(http.StatusOK, body, jsonHeader), nil
		}

		if strings.HasPrefix(req.URL.Path, "/storage/v1/b/") {
			prefix := req.URL.Query().Get("prefix")

			names := make([]string, 0, len(bucket))

			for name := range bucket {
				if strings.HasPrefix(name, prefix) {
					names = append(names, name)
				}
			}

			sort.Strings(names)

			items := make([]map[string]any, 0, len(names))
			for _, name := range names {
				items = append(items, map[string]any{"name": name})
			}

			body, err := json.Marshal(map[string]any{"kind": "storage#objects", "items": items})
			require.NoError(t, err)

			return vhttp.Respond(http.StatusOK, body, jsonHeader), nil
		}

		object := strings.TrimPrefix(req.URL.Path, "/bucket/")

		content, ok := bucket[object]
		if !ok {
			return vhttp.Respond(http.StatusNotFound, []byte(""), nil), nil
		}

		return vhttp.Respond(http.StatusOK, []byte(content), nil), nil
	}
}

// stubGCPVenv returns a venv holding a synthetic service-account key, the
// variable naming it, and h as the client. Credentials resolve against the
// venv, so the key never touches the real filesystem or the process
// environment and the test stays parallel.
func stubGCPVenv(t *testing.T, h vhttp.Handler) *venv.Venv {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	sa, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"project_id":   "stub-project",
		"client_email": "stub@stub-project.iam.gserviceaccount.com",
		"token_uri":    "https://oauth2.googleapis.com/token",
		"private_key": string(pem.EncodeToMemory(&pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})),
	})
	require.NoError(t, err)

	fsys := vfs.NewMemMapFS()
	require.NoError(t, vfs.WriteFile(fsys, "/creds/service-account.json", sa, 0o600))

	return &venv.Venv{
		FS:   fsys,
		HTTP: vhttp.NewMemClient(h),
		Env:  map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": "/creds/service-account.json"},
	}
}

// TestGCSGetterFetchesThroughVenvClient drives the gcs getter over an
// in-memory HTTP client: the token exchange, the mode scan, the listing, and
// every object body ride the venv's client, and the results land on the venv's
// filesystem. The bucket holds an object that is also a prefix, and a sibling
// whose name starts with that object's name, which is what separates the two
// modes.
func TestGCSGetterFetchesThroughVenvClient(t *testing.T) {
	t.Parallel()

	bucket := gcsStubBucket{
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
		object  string
	}{
		{
			// The sibling is what the mode scan reaches after the exact name,
			// so this fails the moment a listed name that merely starts with
			// the object's name is read as proof of a directory.
			name:   "name matching an object downloads that object",
			object: "modules/vpc",
			want:   map[string]string{"vpc": "vpc-object"},
		},
		{
			name:   "prefix without a trailing separator downloads the tree",
			object: "modules/app",
			want:   map[string]string{"main.tf": "app-main"},
		},
		{
			name:   "prefix with a trailing separator downloads the tree",
			object: "modules/vpc/",
			want: map[string]string{
				"main.tf":       "main",
				"sub/nested.tf": "nested",
			},
		},
		{
			name:    "name escaping the destination is refused",
			object:  "modules/esc/",
			wantErr: getter.ErrObjectEscapesDst,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			v := stubGCPVenv(t, gcsStubHandler(t, bucket))

			client := &getter.Client{Getters: []getter.Getter{getter.NewGCSGetter(v)}}
			dst := filepath.Join("out", "module")

			_, err := client.Get(t.Context(), &getter.Request{
				Src:     "gcs::https://www.googleapis.com/storage/v1/bucket/" + tc.object,
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
