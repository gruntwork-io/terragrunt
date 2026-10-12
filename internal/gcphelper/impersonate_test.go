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

// TestGcpConfigImpersonationScopes pins the scopes requested for the impersonated token and that the
// IAM Credentials call goes through the venv transport, signed by the source credentials.
func TestGcpConfigImpersonationScopes(t *testing.T) {
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
				mu            sync.Mutex
				scopes        []string
				authorization string
			)

			client := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
				if req.URL.Host != "iamcredentials.googleapis.com" {
					return vhttp.Respond(http.StatusOK, []byte(`{"name":"object","bucket":"bucket"}`), nil), nil
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
				authorization = req.Header.Get("Authorization")
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
			assert.Equal(t, "Bearer source-token", authorization, "the source credentials must sign the impersonation call")
		})
	}
}
