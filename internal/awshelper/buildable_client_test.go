package awshelper_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
	"github.com/gruntwork-io/terragrunt/internal/iam"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAWSConfigHTTPClientIsBuildable verifies that the HTTP client injected
// into AWS config is an *awshttp.BuildableClient when backed by an OS
// transport, so the SDK's IMDS credential provider can apply its fail-fast
// dial and response-header timeouts.
func TestAWSConfigHTTPClientIsBuildable(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     "test-key",
		"AWS_SECRET_ACCESS_KEY": "test-secret",
		"AWS_REGION":            "us-east-1",
	}

	v := venvtest.New().
		WithFS(vfs.NewOSFS()).
		WithHTTP(vhttp.NewOSClient()).
		WithEnv(env)

	cfg, err := awshelper.NewAWSConfigBuilder().
		Build(t.Context(), l, v)
	require.NoError(t, err)

	_, ok := cfg.HTTPClient.(*awshttp.BuildableClient)
	assert.True(t, ok,
		"HTTPClient must be *awshttp.BuildableClient for IMDS fail-fast, got %T",
		cfg.HTTPClient)
}

// TestAWSConfigBuildableClientPreservesTransport verifies that the
// BuildableClient clones the venv's transport fields so pool, TLS, and
// timeout settings carry over.
func TestAWSConfigBuildableClientPreservesTransport(t *testing.T) {
	t.Parallel()

	// Build a source transport with distinctive values.
	src := http.DefaultTransport.(*http.Transport).Clone()
	src.MaxIdleConnsPerHost = 42
	src.MaxConnsPerHost = 99
	src.IdleConnTimeout = 77 * time.Second
	src.TLSHandshakeTimeout = 13 * time.Second
	src.ForceAttemptHTTP2 = false
	src.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13}

	c := &http.Client{Transport: src} //nolint:exhaustruct // only transport matters here

	got := awshelper.AWSBuildableClient(c)
	bc, ok := got.(*awshttp.BuildableClient)
	require.True(t, ok)

	tr := bc.GetTransport()
	assert.Equal(t, 42, tr.MaxIdleConnsPerHost)
	assert.Equal(t, 99, tr.MaxConnsPerHost)
	assert.Equal(t, 77*time.Second, tr.IdleConnTimeout)
	assert.Equal(t, 13*time.Second, tr.TLSHandshakeTimeout)
	assert.False(t, tr.ForceAttemptHTTP2)
	require.NotNil(t, tr.TLSClientConfig)
	assert.Equal(t, uint16(tls.VersionTLS13), tr.TLSClientConfig.MinVersion)

	// Verify the clone is independent from the source.
	src.MaxIdleConnsPerHost = 1

	tr2 := bc.GetTransport()
	assert.Equal(t, 42, tr2.MaxIdleConnsPerHost,
		"BuildableClient transport must be a clone, not a shared reference")
}

// TestAWSBuildableClientPreservesTimeout verifies that the client-level
// Timeout is carried into the BuildableClient.
func TestAWSBuildableClientPreservesTimeout(t *testing.T) {
	t.Parallel()

	c := vhttp.NewOSClientWithTimeout(7 * time.Second)
	got := awshelper.AWSBuildableClient(c)

	bc, ok := got.(*awshttp.BuildableClient)
	require.True(t, ok)
	assert.Equal(t, 7*time.Second, bc.GetTimeout())
}

// TestAWSConfigMemClientPassesThrough verifies that a vhttp.NewMemClient
// is returned as-is, preserving the in-memory handler for hermetic tests.
func TestAWSConfigMemClientPassesThrough(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64

	mc := vhttp.NewMemClient(func(_ context.Context, _ *http.Request) (*http.Response, error) {
		calls.Add(1)
		return vhttp.Respond(http.StatusOK, []byte("ok"), nil), nil
	})

	got := awshelper.AWSBuildableClient(mc)

	// Must return the original client, not a BuildableClient.
	assert.Same(t, mc, got, "mem client must pass through unchanged")
	assert.IsType(t, (*http.Client)(nil), got)

	// Prove the handler is reachable through the returned client.
	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodGet, "https://s3.stub.example/bucket", nil)
	require.NoError(t, err)

	doer, ok := got.(interface {
		Do(*http.Request) (*http.Response, error)
	})
	require.True(t, ok)

	resp, err := doer.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	assert.Equal(t, int64(1), calls.Load(),
		"mem handler must be invoked through the returned client")
}

// TestAWSConfigNoNetworkClientPassesThrough verifies that
// vhttp.NewNoNetworkClient is returned as-is so sandboxed tests
// fail with the no-network sentinel instead of hitting real DNS.
func TestAWSConfigNoNetworkClientPassesThrough(t *testing.T) {
	t.Parallel()

	nc := vhttp.NewNoNetworkClient()
	got := awshelper.AWSBuildableClient(nc)

	assert.Same(t, nc, got, "no-network client must pass through unchanged")

	req, err := http.NewRequestWithContext(
		t.Context(), http.MethodGet, "https://example.test", nil)
	require.NoError(t, err)

	doer, ok := got.(interface {
		Do(*http.Request) (*http.Response, error)
	})
	require.True(t, ok)

	resp, err := doer.Do(req)
	if resp != nil {
		require.NoError(t, resp.Body.Close())
	}

	require.ErrorIs(t, err, vhttp.ErrNoNetwork)
}

// TestAWSConfigNilTransportProducesBuildableClient verifies the function
// treats a nil transport as the stdlib default and wraps it in a
// BuildableClient so the IMDS fail-fast timeouts are applied.
func TestAWSConfigNilTransportProducesBuildableClient(t *testing.T) {
	t.Parallel()

	//nolint:exhaustruct // nil Transport is the edge case under test.
	c := &http.Client{}
	got := awshelper.AWSBuildableClient(c)

	_, ok := got.(*awshttp.BuildableClient)
	assert.True(t, ok,
		"nil-transport client must produce BuildableClient, got %T", got)
}

// TestAssumeIamRoleFallsBackToIMDSv1WhenTokenEndpointHangs reproduces #6822:
// the IMDSv2 token request never answers (metadata hop limit of 1 with
// Terragrunt in a container), so AssumeIamRole must give up on it fast, read
// the instance role over IMDSv1, and sign the STS call with those credentials.
// The context carries no deadline on purpose: that is what a run passes in,
// and it is what makes the SDK's 5s per-operation IMDS timeout apply.
func TestAssumeIamRoleFallsBackToIMDSv1WhenTokenEndpointHangs(t *testing.T) {
	// The SDK resolves its credential chain from the process environment, so
	// this test isolates it with t.Setenv and therefore cannot run in parallel.
	imds := newHangingTokenIMDSServer(t)
	stsServer := newAssumeRoleSTSServer(t)

	missing := filepath.Join(t.TempDir(), "missing")
	for _, key := range []string{
		"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
		"AWS_PROFILE", "AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE",
		"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI",
		"AWS_EC2_METADATA_DISABLED", "AWS_EC2_METADATA_V1_DISABLED",
		"AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE", "AWS_ENDPOINT_URL",
	} {
		t.Setenv(key, "")
	}

	t.Setenv("AWS_CONFIG_FILE", missing)
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", missing)
	t.Setenv("AWS_EC2_METADATA_SERVICE_ENDPOINT", imds.URL)
	t.Setenv("AWS_ENDPOINT_URL_STS", stsServer.URL)

	v := venvtest.New().
		WithHTTP(vhttp.NewOSClient()).
		WithEnv(map[string]string{"AWS_REGION": "us-east-1"})

	creds, err := awshelper.AssumeIamRole(
		t.Context(), v, iam.RoleOptions{RoleARN: testAssumedRoleARN}, "")
	require.NoError(t, err)
	require.NotNil(t, creds)
	assert.Equal(t, testAssumedAccessKeyID, aws.ToString(creds.AccessKeyId))

	assert.Positive(t, imds.v1Requests.Load(), "instance role must be read over IMDSv1")
	assert.Contains(t, stsServer.authorization.Load(), "Credential="+testIMDSAccessKeyID+"/",
		"STS call must be signed with the IMDSv1 instance credentials")
}

// TestAWSBuildableClientOSTransportIsBuildable explicitly tests the
// function's return type with a production OS client.
func TestAWSBuildableClientOSTransportIsBuildable(t *testing.T) {
	t.Parallel()

	oc := vhttp.NewOSClient()
	got := awshelper.AWSBuildableClient(oc)

	_, ok := got.(*awshttp.BuildableClient)
	assert.True(t, ok,
		"OS transport must produce *awshttp.BuildableClient, got %T", got)
}

const (
	testIMDSRoleName       = "test-instance-role"
	testIMDSAccessKeyID    = "AKIAIMDSV1TESTKEY"
	testAssumedRoleARN     = "arn:aws:iam::123456789012:role/test-assumed-role"
	testAssumedAccessKeyID = "ASIAASSUMEDTESTKEY"
)

// hangingTokenIMDSServer is a local IMDS whose IMDSv2 token endpoint never
// answers, while the IMDSv1 credential endpoints answer normally.
type hangingTokenIMDSServer struct {
	*httptest.Server
	v1Requests atomic.Int64
}

// newHangingTokenIMDSServer starts the IMDS stand-in and closes it with the test.
func newHangingTokenIMDSServer(t *testing.T) *hangingTokenIMDSServer {
	t.Helper()

	srv := &hangingTokenIMDSServer{} //nolint:exhaustruct // Server is set below.
	expiration := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/latest/api/token" {
			<-r.Context().Done()
			return
		}

		if r.Header.Get("X-aws-ec2-metadata-token") != "" {
			http.Error(w, "unexpected IMDSv2 token", http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/latest/meta-data/iam/security-credentials/":
			srv.v1Requests.Add(1)

			_, _ = io.WriteString(w, testIMDSRoleName)
		case "/latest/meta-data/iam/security-credentials/" + testIMDSRoleName:
			srv.v1Requests.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"Code":"Success","Type":"AWS-HMAC","AccessKeyId":%q,`+
				`"SecretAccessKey":"imds-secret","Token":"imds-token","Expiration":%q}`,
				testIMDSAccessKeyID, expiration)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

// assumeRoleSTSServer is a local STS that answers AssumeRole and records the
// Authorization header of the last request it served.
type assumeRoleSTSServer struct {
	*httptest.Server
	authorization atomic.Value
}

// newAssumeRoleSTSServer starts the STS stand-in and closes it with the test.
func newAssumeRoleSTSServer(t *testing.T) *assumeRoleSTSServer {
	t.Helper()

	srv := &assumeRoleSTSServer{} //nolint:exhaustruct // Server is set below.
	srv.authorization.Store("")

	expiration := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.authorization.Store(r.Header.Get("Authorization"))

		body, err := io.ReadAll(r.Body)
		if err != nil || !strings.Contains(string(body), "Action=AssumeRole") {
			http.Error(w, "expected AssumeRole", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/xml")
		_, _ = fmt.Fprintf(w, `<AssumeRoleResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">`+
			`<AssumeRoleResult><Credentials><AccessKeyId>%s</AccessKeyId>`+
			`<SecretAccessKey>assumed-secret</SecretAccessKey><SessionToken>assumed-token</SessionToken>`+
			`<Expiration>%s</Expiration></Credentials>`+
			`<AssumedRoleUser><AssumedRoleId>AROATEST:session</AssumedRoleId><Arn>%s</Arn></AssumedRoleUser>`+
			`</AssumeRoleResult><ResponseMetadata><RequestId>test</RequestId></ResponseMetadata>`+
			`</AssumeRoleResponse>`, testAssumedAccessKeyID, expiration, testAssumedRoleARN)
	}))
	t.Cleanup(srv.Close)

	return srv
}
