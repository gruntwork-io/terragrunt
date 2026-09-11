package s3

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/require"
)

// mockS3Transport is an http.RoundTripper that fakes the AWS endpoints the
// access-logging fallback touches: STS GetCallerIdentity (for partition
// lookup), S3 PutBucketAcl, GetBucketPolicy and PutBucketPolicy.
type mockS3Transport struct {
	mu                sync.Mutex
	putBucketAclCalls int
	putPolicyCalls    int
	lastPolicy        string
}

func (m *mockS3Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body string

	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		req.Body.Close()

		body = string(b)
	}

	// STS GetCallerIdentity is a form-encoded POST whose body carries the action.
	if strings.Contains(body, "GetCallerIdentity") {
		return m.xmlResponse(http.StatusOK, `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::123456789012:user/test-user</Arn>
    <UserId>AIDATESTUSER</UserId>
    <Account>123456789012</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata>
    <RequestId>11111111-1111-1111-1111-111111111111</RequestId>
  </ResponseMetadata>
</GetCallerIdentityResponse>`), nil
	}

	query := req.URL.Query()

	switch {
	case req.Method == http.MethodPut && query.Has("acl"):
		m.mu.Lock()
		m.putBucketAclCalls++
		m.mu.Unlock()

		return m.xmlResponse(http.StatusBadRequest, `<Error><Code>AccessControlListNotSupported</Code><Message>The bucket does not allow ACLs</Message><RequestId>test-request-id</RequestId></Error>`), nil

	case req.Method == http.MethodGet && query.Has("policy"):
		return m.xmlResponse(http.StatusNotFound, `<Error><Code>NoSuchBucketPolicy</Code><Message>The bucket policy does not exist</Message><RequestId>test-request-id</RequestId></Error>`), nil

	case req.Method == http.MethodPut && query.Has("policy"):
		m.mu.Lock()
		m.putPolicyCalls++
		m.lastPolicy = body
		m.mu.Unlock()

		return m.xmlResponse(http.StatusOK, ``), nil
	}

	return nil, fmt.Errorf("unexpected request: %s %s", req.Method, req.URL.String())
}

func (m *mockS3Transport) xmlResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     http.Header{"Content-Type": []string{"application/xml"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestConfigureBucketAccessLoggingACLFallsBackToBucketPolicy(t *testing.T) {
	t.Parallel()

	transport := &mockS3Transport{}

	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", ""),
		HTTPClient:  &http.Client{Transport: transport},
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	client := &Client{
		s3Client:  s3Client,
		awsConfig: cfg,
	}

	l := logger.CreateLogger()

	err := client.configureBucketAccessLoggingACL(t.Context(), l, "my-logs-bucket")
	require.NoError(t, err)

	require.Equal(t, 1, transport.putBucketAclCalls, "PutBucketAcl should be attempted once")
	require.Equal(t, 1, transport.putPolicyCalls, "PutBucketPolicy should be called as the fallback")

	require.Contains(t, transport.lastPolicy, `"Service":"logging.s3.amazonaws.com"`)
	require.Contains(t, transport.lastPolicy, `"s3:PutObject"`)
	require.Contains(t, transport.lastPolicy, `"arn:aws:s3:::my-logs-bucket/*"`)
}
