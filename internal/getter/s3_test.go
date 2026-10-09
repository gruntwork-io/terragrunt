package getter_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	gogetter "github.com/hashicorp/go-getter/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultClientCanonicalizesS3SourceURLs pins that `s3::https://`
// sources work in every AWS S3 endpoint form, including the
// virtual-hosted style (`<bucket>.s3.<region>.amazonaws.com`) AWS
// documents as canonical. The upstream go-getter/s3/v2 getter only
// parses path-style hostnames (`s3.amazonaws.com`,
// `s3-<region>.amazonaws.com`), so whichever getter claims the request
// must canonicalize req.Src at Detect time: Client.Get re-parses
// req.Src after detection into the URL the fetch uses.
//
// The test mirrors the outer client's getter-selection loop instead of
// calling Client.Get so no network I/O happens.
func TestDefaultClientCanonicalizesS3SourceURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		src      string
		wantHost string
		wantPath string
	}{
		{
			name:     "modern virtual-host style",
			src:      "s3::https://my-bucket.s3.us-west-2.amazonaws.com/terraform/modules/myapp.zip",
			wantHost: "s3-us-west-2.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
		{
			name:     "legacy regional virtual-host style",
			src:      "s3::https://my-bucket.s3-us-west-2.amazonaws.com/terraform/modules/myapp.zip",
			wantHost: "s3-us-west-2.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
		{
			name:     "global virtual-host style",
			src:      "s3::https://my-bucket.s3.amazonaws.com/terraform/modules/myapp.zip",
			wantHost: "s3.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
		{
			name:     "modern path-style",
			src:      "s3::https://s3.us-west-2.amazonaws.com/my-bucket/terraform/modules/myapp.zip",
			wantHost: "s3-us-west-2.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
		{
			name:     "legacy regional path-style stays unchanged",
			src:      "s3::https://s3-us-west-2.amazonaws.com/my-bucket/terraform/modules/myapp.zip",
			wantHost: "s3-us-west-2.amazonaws.com",
			wantPath: "/my-bucket/terraform/modules/myapp.zip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := getter.NewClient(logger.CreateLogger(), venvtest.NewWithOSFS())
			req := &gogetter.Request{Src: tt.src, GetMode: getter.ModeAny}

			claimed := false

			for _, g := range client.Getters {
				ok, err := gogetter.Detect(req, g)
				require.NoError(t, err)

				if ok {
					claimed = true
					break
				}
			}

			require.True(t, claimed, "a getter must claim %s", tt.src)
			assert.Equal(t, getter.SchemeS3, req.Forced)

			u, err := url.Parse(req.Src)
			require.NoError(t, err)
			assert.Equal(
				t,
				tt.wantHost,
				u.Host,
				"Detect must canonicalize to a path-style host the bare go-getter/s3 v2 getter accepts",
			)
			assert.Equal(t, tt.wantPath, u.Path)
		})
	}
}

// TestS3ClientForTargetContainerCredentialHosts pins which container credential
// endpoints the S3 download client accepts: loopback and the ECS and EKS Pod
// Identity agent addresses. The SDK rejects any other host while building the
// client, so no case sends a request.
func TestS3ClientForTargetContainerCredentialHosts(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		wantReject bool
	}{
		{
			name:       "EKS Pod Identity IPv4 accepted",
			endpoint:   "http://169.254.170.23/v1/credentials",
			wantReject: false,
		},
		{
			name:       "ECS container IPv4 accepted",
			endpoint:   "http://169.254.170.2/v1/credentials",
			wantReject: false,
		},
		{
			name:       "loopback accepted",
			endpoint:   "http://127.0.0.1/v1/credentials",
			wantReject: false,
		},
		{
			name:       "arbitrary private IP rejected",
			endpoint:   "http://192.168.1.1/v1/credentials",
			wantReject: true,
		},
		{
			name:       "arbitrary 10.x IP rejected",
			endpoint:   "http://10.0.0.1/v1/credentials",
			wantReject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suppressAWSEnv(t)
			t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", tt.endpoint)

			_, err := getter.S3ClientForTarget(
				t.Context(), logger.CreateLogger(), venvtest.New(), &getter.S3FetchTarget{})

			if tt.wantReject {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

// TestS3ClientForTargetEKSPodIdentityCredentials pins that the S3 download
// client completes the EKS Pod Identity flow: it reads the token from
// AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE, sends it as the Authorization header
// to AWS_CONTAINER_CREDENTIALS_FULL_URI, and decodes the credentials the agent
// returns. A loopback server stands in for the agent because the SDK accepts
// loopback endpoints. The token file lives on the OS filesystem because the SDK
// reads it with os.ReadFile.
func TestS3ClientForTargetEKSPodIdentityCredentials(t *testing.T) {
	const (
		fakeAccessKey = "test-access-key-id"
		fakeSecretKey = "test-secret-access-key"
		fakeToken     = "test-session-token"
		authToken     = "k8s-pod-identity-token-xyz"
	)

	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte(authToken), 0o600))

	credsJSON, err := json.Marshal(map[string]any{
		"AccessKeyId":     fakeAccessKey,
		"SecretAccessKey": fakeSecretKey,
		"Token":           fakeToken,
		"Expiration":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)

	var gotAuthHeader atomic.Pointer[string]

	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader.Store(new(r.Header.Get("Authorization")))
		w.Header().Set("Content-Type", "application/json")

		_, writeErr := w.Write(credsJSON)
		assert.NoError(t, writeErr)
	}))
	t.Cleanup(agent.Close)

	suppressAWSEnv(t)
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", agent.URL+"/v1/credentials")
	t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", tokenFile)

	client, err := getter.S3ClientForTarget(
		t.Context(), logger.CreateLogger(), venvtest.New(), &getter.S3FetchTarget{})
	require.NoError(t, err)

	creds, err := client.Options().Credentials.Retrieve(t.Context())
	require.NoError(t, err)

	assert.Equal(t, fakeAccessKey, creds.AccessKeyID)
	assert.Equal(t, fakeSecretKey, creds.SecretAccessKey)
	assert.Equal(t, fakeToken, creds.SessionToken)

	header := gotAuthHeader.Load()
	require.NotNil(t, header, "the agent must receive the credentials request")
	assert.Equal(t, authToken, *header)
}

// suppressAWSEnv clears the AWS environment the SDK credential chain reads, so
// keys, profiles, or container settings on the machine running the suite cannot
// change which provider resolves.
func suppressAWSEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_ACCESS_KEY", "AWS_SECRET_KEY",
		"AWS_PROFILE", "AWS_DEFAULT_PROFILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		"AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN",
		"AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE",
		"AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_ROLE_ARN",
		"AWS_ROLE_SESSION_NAME",
	} {
		t.Setenv(key, "")
	}

	missing := filepath.Join(t.TempDir(), "missing")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}
