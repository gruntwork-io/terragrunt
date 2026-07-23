package getter_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/gruntwork-io/terragrunt/internal/getter"
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

			client := getter.NewClient()
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

// Verifies that the SDK allows EKS and ECS container credential endpoints
// and rejects arbitrary hosts. Uses a recording RoundTripper so no real
// network I/O occurs and results are deterministic on any runner.
// This is a dependency-contract test: it exercises the same session constructor
// go-getter/s3/v2 uses to verify the aws-sdk-go v1 version pin.
func TestS3SessionCredentialEndpointHosts(t *testing.T) {
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

			var called atomic.Bool

			sess, err := session.NewSession(&aws.Config{
				Region: aws.String("us-east-1"),
				HTTPClient: &http.Client{
					Transport: recordingTransport(&called),
				},
				CredentialsChainVerboseErrors: aws.Bool(true),
				MaxRetries:                    aws.Int(0),
			})
			require.NoError(t, err)

			_, credErr := sess.Config.Credentials.Get()

			if tt.wantReject {
				assert.False(t, called.Load(), "transport must NOT be invoked for rejected hosts")
				require.Error(t, credErr)

				return
			}

			assert.True(t, called.Load(), "transport must be invoked for accepted hosts")
		})
	}
}

// Verifies the full EKS Pod Identity credential flow: endpoint acceptance,
// authorization-token-file loading, header propagation, and response decoding.
// Uses a recording RoundTripper against the EKS endpoint IP so no real
// request reaches 169.254.170.23. The token file is a real temp file because
// the SDK reads the OS filesystem directly.
func TestS3SessionEKSPodIdentityAuthTokenFile(t *testing.T) {
	const (
		fakeAccessKey = "AKIAIOSFODNN7EXAMPLE"
		fakeSecretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLE"
		fakeToken     = "FwoGZXIvYXdzEBYaDHqa0AP"
		authToken     = "k8s-pod-identity-token-xyz"
	)

	tokenFile := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(tokenFile, []byte(authToken), 0o600))

	suppressAWSEnv(t)
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "http://169.254.170.23/v1/credentials")
	t.Setenv("AWS_CONTAINER_AUTHORIZATION_TOKEN_FILE", tokenFile)

	credsJSON, err := json.Marshal(map[string]any{
		"AccessKeyId":     fakeAccessKey,
		"SecretAccessKey": fakeSecretKey,
		"Token":           fakeToken,
		"Expiration":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)

	var gotAuthHeader string

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				gotAuthHeader = req.Header.Get("Authorization")

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(bytes.NewReader(credsJSON)),
				}, nil
			}),
		},
		MaxRetries: aws.Int(0),
	})
	require.NoError(t, err)

	creds, err := sess.Config.Credentials.Get()
	require.NoError(t, err)

	assert.Equal(t, fakeAccessKey, creds.AccessKeyID)
	assert.Equal(t, fakeSecretKey, creds.SecretAccessKey)
	assert.Equal(t, fakeToken, creds.SessionToken)
	assert.Equal(t, authToken, gotAuthHeader)
}

// Verifies the full credential flow via a local httptest server serving STS-shaped credentials.
func TestS3SessionRetrievesCredsFromLocalEndpoint(t *testing.T) {
	const (
		fakeAccessKey = "AKIAIOSFODNN7EXAMPLE"
		fakeSecretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLE"
		fakeToken     = "FwoGZXIvYXdzEBYaDHqa0AP"
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"AccessKeyId":     fakeAccessKey,
			"SecretAccessKey": fakeSecretKey,
			"Token":           fakeToken,
			"Expiration":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	t.Cleanup(srv.Close)

	suppressAWSEnv(t)
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", srv.URL+"/v1/credentials")

	sess, err := session.NewSession(&aws.Config{Region: aws.String("us-east-1")})
	require.NoError(t, err)

	creds, err := sess.Config.Credentials.Get()
	require.NoError(t, err)
	assert.Equal(t, fakeAccessKey, creds.AccessKeyID)
	assert.Equal(t, fakeSecretKey, creds.SecretAccessKey)
	assert.Equal(t, fakeToken, creds.SessionToken)
}

// suppressAWSEnv neutralizes all AWS credential env vars the SDK v1 chain inspects.
func suppressAWSEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_ACCESS_KEY", "AWS_SECRET_KEY",
		"AWS_PROFILE", "AWS_DEFAULT_PROFILE",
		"AWS_SDK_LOAD_CONFIG",
		"AWS_METADATA_URL",
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

	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// recordingTransport returns a RoundTripper that records whether it was called and returns a sentinel error.
func recordingTransport(called *atomic.Bool) http.RoundTripper {
	return roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		called.Store(true)

		return nil, errTransportSentinel
	})
}

var errTransportSentinel = errors.New("sentinel: transport invoked")

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }
