package gcphelper_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/gcphelper"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGCPConfigImpersonationScopes pins the impersonated token's scopes and the token each call carries.
func TestGCPConfigImpersonationScopes(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		scopes     []string
		wantScopes []string
	}{
		{
			name:       "unset scopes keep full control",
			wantScopes: []string{storage.ScopeFullControl},
		},
		{
			name:       "configured scopes are requested",
			scopes:     []string{storage.ScopeReadWrite},
			wantScopes: []string{storage.ScopeReadWrite},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			var (
				mu                   sync.Mutex
				scopes               []string
				iamAuthorization     string
				storageAuthorization string
			)

			client := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
				if req.URL.Host == "storage.googleapis.com" {
					mu.Lock()
					storageAuthorization = req.Header.Get("Authorization")
					mu.Unlock()

					return vhttp.Respond(http.StatusOK, []byte(`{"name":"object","bucket":"bucket"}`), nil), nil
				}

				if req.URL.Host != "iamcredentials.googleapis.com" {
					assert.Fail(t, "unexpected request to "+req.URL.Host)

					return vhttp.Respond(http.StatusInternalServerError, nil, nil), nil
				}

				body, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}

				var request struct {
					Scope []string `json:"scope"`
				}

				if err := json.Unmarshal(body, &request); err != nil {
					return nil, err
				}

				mu.Lock()
				scopes = request.Scope
				iamAuthorization = req.Header.Get("Authorization")
				mu.Unlock()

				return vhttp.Respond(
					http.StatusOK,
					[]byte(`{"accessToken":"impersonated-token","expireTime":"2099-01-01T00:00:00Z"}`),
					nil,
				), nil
			})

			gcpCfg := &gcphelper.GCPSessionConfig{
				AccessToken:               "source-token",
				ImpersonateServiceAccount: "state@project.iam.gserviceaccount.com",
				ImpersonateScopes:         testCase.scopes,
			}

			ctx := t.Context()

			gcsClient, err := gcphelper.NewGCPConfigBuilder().
				WithSessionConfig(gcpCfg).
				BuildGCSClient(ctx, venvtest.New().WithEnv(map[string]string{}).WithHTTP(client))
			require.NoError(t, err)

			t.Cleanup(func() { require.NoError(t, gcsClient.Close()) })

			_, err = gcsClient.Bucket("bucket").Object("object").Attrs(ctx)
			require.NoError(t, err)

			mu.Lock()
			defer mu.Unlock()

			assert.Equal(t, testCase.wantScopes, scopes)
			assert.Equal(t, "Bearer source-token", iamAuthorization, "the source credentials must sign the impersonation call")
			assert.Equal(t, "Bearer impersonated-token", storageAuthorization, "storage must use the impersonated token")
		})
	}
}
