package s3_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testBucketName = "test-bucket"
	testTableName  = "test-lock-table"

	testCallerIdentityXML = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">` +
		`<GetCallerIdentityResult>` +
		`<Arn>arn:aws:iam::123456789012:user/test</Arn>` +
		`<UserId>AIDATESTUSER</UserId>` +
		`<Account>123456789012</Account>` +
		`</GetCallerIdentityResult></GetCallerIdentityResponse>`
)

// TestUpdateS3BucketIfNecessaryEncryptionResponseWithoutConfiguration checks that an empty GetBucketEncryption body enables encryption.
func TestUpdateS3BucketIfNecessaryEncryptionResponseWithoutConfiguration(t *testing.T) {
	t.Parallel()

	var putEncryptionCalls atomic.Int32

	client := newMemS3Client(t, func(_ context.Context, req *http.Request) (*http.Response, error) {
		switch {
		// EnableSSEForS3BucketWide reads the account ID and partition from STS.
		case strings.HasPrefix(req.URL.Host, "sts."):
			return vhttp.Respond(http.StatusOK, []byte(testCallerIdentityXML), nil), nil
		case req.Method == http.MethodHead && req.URL.Path == "/"+testBucketName:
			return vhttp.Respond(http.StatusOK, nil, nil), nil
		case req.Method == http.MethodGet && req.URL.Query().Has("encryption"):
			return vhttp.Respond(http.StatusOK, nil, nil), nil
		case req.Method == http.MethodPut && req.URL.Query().Has("encryption"):
			putEncryptionCalls.Add(1)

			return vhttp.Respond(http.StatusOK, nil, nil), nil
		}

		return vhttp.Respond(http.StatusNotImplemented, nil, nil), nil
	})

	opts := &backend.Options{NonInteractive: true}

	require.NotPanics(t, func() {
		err := client.UpdateS3BucketIfNecessary(t.Context(), logger.CreateLogger(), venvtest.New(), testBucketName, opts)
		require.NoError(t, err)
	})

	assert.Equal(t, int32(1), putEncryptionCalls.Load(), "a bucket with no encryption document must get encryption enabled")
}

// TestLockTableDescribeResponseWithoutTable checks that both lock table checks return `TableDescriptionMissingError`.
func TestLockTableDescribeResponseWithoutTable(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		check func(client *s3backend.Client, ctx context.Context, tableName string) (bool, error)
		name  string
	}{
		{
			name:  "exists and active",
			check: (*s3backend.Client).DoesLockTableExistAndIsActive,
		},
		{
			name:  "encryption enabled",
			check: (*s3backend.Client).LockTableCheckSSEncryptionIsOn,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := newMemS3Client(t, func(_ context.Context, req *http.Request) (*http.Response, error) {
				if req.Header.Get("X-Amz-Target") != "DynamoDB_20120810.DescribeTable" {
					return vhttp.Respond(http.StatusNotImplemented, nil, nil), nil
				}

				headers := http.Header{"Content-Type": []string{"application/x-amz-json-1.0"}}

				// Valid JSON with no Table field.
				return vhttp.Respond(http.StatusOK, []byte(`{}`), headers), nil
			})

			var (
				ok  bool
				err error
			)

			require.NotPanics(t, func() {
				ok, err = tc.check(client, t.Context(), testTableName)
			})

			var missingErr s3backend.TableDescriptionMissingError

			require.ErrorAs(t, err, &missingErr)
			assert.Equal(t, testTableName, missingErr.TableName)
			assert.False(t, ok)
		})
	}
}

// newMemS3Client builds a Client whose S3 and DynamoDB calls are answered by h.
func newMemS3Client(t *testing.T, h vhttp.Handler) *s3backend.Client {
	t.Helper()

	v := venvtest.New().
		WithHTTP(vhttp.NewMemClient(h)).
		WithEnv(map[string]string{
			"AWS_ACCESS_KEY_ID":     "AKIATESTKEY",
			"AWS_SECRET_ACCESS_KEY": "test-secret",
		})

	cfg := &s3backend.ExtendedRemoteStateConfigS3{
		RemoteStateConfigS3: s3backend.RemoteStateConfigS3{
			Region:           "us-east-1",
			Bucket:           testBucketName,
			Endpoint:         "https://s3.example.test",
			DynamoDBEndpoint: "https://dynamodb.example.test",
			// Path style puts the bucket in req.URL.Path, where the handlers match it.
			UsePathStyle: true,
		},
		SkipCredentialsValidation: true,
		// Skip the other bucket checks so only the encryption check sends requests.
		SkipBucketVersioning:           true,
		SkipBucketEnforcedTLS:          true,
		SkipBucketPublicAccessBlocking: true,
	}

	client, err := s3backend.NewClient(t.Context(), logger.CreateLogger(), v, cfg, &backend.Options{})
	require.NoError(t, err)

	return client
}
