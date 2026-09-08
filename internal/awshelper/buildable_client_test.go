package awshelper_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/gruntwork-io/terragrunt/internal/awshelper"
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
	src.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS13} //nolint:gosec // test value

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

// TestAssumeIamRolePathUsesBuildableClient verifies the config loaded in
// the AssumeIamRole code path uses the same BuildableClient wrapping.
func TestAssumeIamRolePathUsesBuildableClient(t *testing.T) {
	t.Parallel()

	v := venvtest.New().
		WithFS(vfs.NewOSFS()).
		WithHTTP(vhttp.NewOSClient()).
		WithEnv(map[string]string{
			"AWS_ACCESS_KEY_ID":     "test-key",
			"AWS_SECRET_ACCESS_KEY": "test-secret",
			"AWS_REGION":            "us-east-1",
		})

	//nolint:forbidigo // mirrors the AssumeIamRole path to verify HTTP client type.
	cfg, err := config.LoadDefaultConfig(
		t.Context(),
		config.WithRegion("us-east-1"),
		config.WithHTTPClient(awshelper.AWSBuildableClient(v.HTTP)),
	)
	require.NoError(t, err)

	_, ok := cfg.HTTPClient.(*awshttp.BuildableClient)
	assert.True(t, ok,
		"AssumeIamRole config must use *awshttp.BuildableClient, got %T",
		cfg.HTTPClient)
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
