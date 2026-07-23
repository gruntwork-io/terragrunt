package getter_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
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

// suppressAWSEnv clears all environment variables that the aws-sdk-go
// v1 default credential chain inspects, ensuring the session falls
// through to the container credential endpoint provider (set via
// AWS_CONTAINER_CREDENTIALS_FULL_URI) or fails with no credentials.
func suppressAWSEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"AWS_ACCESS_KEY_ID",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SESSION_TOKEN",
		"AWS_SHARED_CREDENTIALS_FILE",
		"AWS_CONFIG_FILE",
		"AWS_METADATA_URL",
		"AWS_PROFILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI",
		"AWS_EC2_METADATA_DISABLED",
	} {
		t.Setenv(key, "")
	}

	// Point shared config at /dev/null so no stale file is read.
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
	t.Setenv("AWS_CONFIG_FILE", "/dev/null")
	// Disable IMDS so the chain doesn't hang trying the EC2
	// metadata service in a non-EC2 environment.
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// newShortTimeoutSession creates an aws-sdk-go v1 session with a very
// short HTTP timeout so tests that verify credential endpoint
// acceptance don't hang waiting for unreachable endpoints
// (169.254.170.x). CredentialsChainVerboseErrors is enabled so the
// per-provider error messages (including endpoint host rejections)
// surface in the final error string. The timeout only affects the HTTP
// round-trip, not session construction or provider chain building.
func newShortTimeoutSession(t *testing.T) *session.Session {
	t.Helper()

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
		HTTPClient: &http.Client{
			Timeout: 200 * time.Millisecond,
		},
		CredentialsChainVerboseErrors: aws.Bool(true),
	})
	require.NoError(t, err)

	return sess
}

// TestS3SessionCredentialEndpointHosts verifies that the aws-sdk-go v1
// session — the same code path go-getter/s3/v2.Getter.newS3Client uses —
// accepts EKS Pod Identity and ECS container credential endpoints while
// rejecting arbitrary hosts.
//
// This is a regression test for github.com/gruntwork-io/terragrunt/issues/6450:
// aws-sdk-go v1.44.122 rejected the EKS Pod Identity Agent endpoint
// (169.254.170.23) because its isLoopbackHost check only allowed
// 127.0.0.1. The fix bumps aws-sdk-go v1 to >= v1.47.11 which adds
// the EKS/ECS container IPs to the allowlist.
//
// No AWS credentials or network connectivity are required. The test
// verifies only the host validation in the credential provider chain,
// not the actual credential retrieval.
//
// Cannot use t.Parallel because t.Setenv modifies process-global
// environment variables. The subtests run sequentially for the same
// reason.
func TestS3SessionCredentialEndpointHosts(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		wantReject bool
	}{
		{
			name:       "EKS Pod Identity IPv4 endpoint accepted",
			endpoint:   "http://169.254.170.23/v1/credentials",
			wantReject: false,
		},
		{
			name:       "ECS container IPv4 endpoint accepted",
			endpoint:   "http://169.254.170.2/v1/credentials",
			wantReject: false,
		},
		{
			name:       "loopback endpoint accepted",
			endpoint:   "http://127.0.0.1/v1/credentials",
			wantReject: false,
		},
		{
			name:       "arbitrary private IP rejected",
			endpoint:   "http://192.168.1.1/v1/credentials",
			wantReject: true,
		},
		{
			name:       "arbitrary private IP 10.x rejected",
			endpoint:   "http://10.0.0.1/v1/credentials",
			wantReject: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suppressAWSEnv(t)
			t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", tt.endpoint)

			sess := newShortTimeoutSession(t)

			// Attempt to retrieve credentials. The session builds a
			// credential chain; Get walks it. We care about whether
			// the host was rejected statically (ErrorProvider), not
			// whether the endpoint responded.
			_, err := sess.Config.Credentials.Get()
			require.Error(t, err, "credential retrieval should fail (no real endpoint)")

			// The rejection message in v1.44.122 is "only loopback hosts
			// are allowed"; in v1.55.6 it's "only loopback/ecs/eks hosts
			// are allowed". Both contain "only loopback".
			const rejectMsg = "only loopback"
			if tt.wantReject {
				assert.Contains(t, err.Error(), rejectMsg,
					"arbitrary hosts must be rejected by the SDK credential chain")
			} else {
				assert.NotContains(t, err.Error(), rejectMsg,
					"EKS/ECS/loopback hosts must NOT be rejected by the SDK credential chain")
			}
		})
	}
}

// TestS3SessionRetrievesCredsFromLocalEndpoint verifies the full
// credential flow: an httptest server on loopback returns STS-shaped
// credentials via AWS_CONTAINER_CREDENTIALS_FULL_URI, and a session
// created the same way go-getter/s3/v2 does it picks them up. This
// exercises the happy path without touching any real AWS endpoint.
//
// Cannot use t.Parallel because t.Setenv modifies process-global
// environment variables.
func TestS3SessionRetrievesCredsFromLocalEndpoint(t *testing.T) {
	const (
		fakeAccessKey = "AKIAIOSFODNN7EXAMPLE"
		fakeSecretKey = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLE"
		fakeToken     = "FwoGZXIvYXdzEBYaDHqa0AP"
	)

	// STS-shaped credential response the SDK expects from a container
	// credential endpoint.
	credsResponse := map[string]any{
		"AccessKeyId":     fakeAccessKey,
		"SecretAccessKey": fakeSecretKey,
		"Token":           fakeToken,
		"Expiration":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(credsResponse)
	}))
	t.Cleanup(srv.Close)

	suppressAWSEnv(t)
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", srv.URL+"/v1/credentials")

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
	})
	require.NoError(t, err)

	creds, err := sess.Config.Credentials.Get()
	require.NoError(t, err)

	assert.Equal(t, fakeAccessKey, creds.AccessKeyID)
	assert.Equal(t, fakeSecretKey, creds.SecretAccessKey)
	assert.Equal(t, fakeToken, creds.SessionToken)
}

// TestS3SessionEnvCredsPreemptEndpoint verifies the credential chain
// precedence: when AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are set
// (the workaround described in issue #6450), the environment provider
// wins over the container endpoint provider. This confirms both the
// workaround and the credential chain ordering.
//
// Cannot use t.Parallel because t.Setenv modifies process-global
// environment variables.
func TestS3SessionEnvCredsPreemptEndpoint(t *testing.T) {
	const (
		envAccessKey = "AKIAENVOVERRIDE"
		envSecretKey = "envSecretOverride"
	)

	// The endpoint returns different credentials than the env vars.
	// We never expect the endpoint to be called, but serve it just in
	// case so we can distinguish which source won.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"AccessKeyId":     "AKIANOTTHISONE",
			"SecretAccessKey": "notThisSecret",
			"Token":           "notThisToken",
			"Expiration":      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
		})
	}))
	t.Cleanup(srv.Close)

	suppressAWSEnv(t)
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", srv.URL+"/v1/credentials")
	t.Setenv("AWS_ACCESS_KEY_ID", envAccessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", envSecretKey)

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"),
	})
	require.NoError(t, err)

	creds, err := sess.Config.Credentials.Get()
	require.NoError(t, err)

	assert.Equal(t, envAccessKey, creds.AccessKeyID,
		"env provider must win over container endpoint provider")
	assert.Equal(t, envSecretKey, creds.SecretAccessKey)
}

// TestS3SessionNoCredsDoesNotPanic verifies that creating a session
// with no credential source at all (no env vars, no shared config, no
// IMDS) does not panic and returns a clear error on credential
// retrieval. This covers the fallthrough in go-getter's credential
// chain when running outside AWS entirely.
//
// Cannot use t.Parallel because t.Setenv modifies process-global
// environment variables.
func TestS3SessionNoCredsDoesNotPanic(t *testing.T) {
	suppressAWSEnv(t)
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")

	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("us-east-1"),
		Credentials: credentials.NewChainCredentials([]credentials.Provider{&credentials.EnvProvider{}}),
	})
	require.NoError(t, err)

	_, err = sess.Config.Credentials.Get()
	require.Error(t, err, "should fail with no credentials available")
}
