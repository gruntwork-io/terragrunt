package s3_test

import (
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testLogsBucketName = "test-logs-bucket"

// TestBootstrapCreatesAndConfiguresMissingBucket checks that a missing state bucket is created and then given enforced
// TLS, public access blocking, versioning, and encryption.
func TestBootstrapCreatesAndConfiguresMissingBucket(t *testing.T) {
	t.Parallel()

	fake := newFakeS3()

	err := bootstrap(t, fake, bootstrapConfig(nil), &backend.Options{NonInteractive: true})
	require.NoError(t, err)

	assert.Equal(
		t,
		[]string{
			"CreateBucket " + testBucketName,
			"PutBucketPolicy " + testBucketName,
			"PutPublicAccessBlock " + testBucketName,
			"PutBucketVersioning " + testBucketName,
			"PutBucketEncryption " + testBucketName,
		},
		fake.changes(),
	)
}

// TestBootstrapFailIfBucketCreationRequired checks that --backend-require-bootstrap stops bootstrap before it
// sends any request that creates or changes a bucket.
func TestBootstrapFailIfBucketCreationRequired(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		config            backend.Config
		name              string
		wantRefusedBucket string
		bucketExists      bool
	}{
		{
			name:              "missing state bucket",
			config:            bootstrapConfig(nil),
			wantRefusedBucket: testBucketName,
		},
		{
			name:              "missing access logging bucket",
			config:            bootstrapConfig(backend.Config{"accesslogging_bucket_name": testLogsBucketName}),
			bucketExists:      true,
			wantRefusedBucket: testLogsBucketName,
		},
		{
			name:         "configured state bucket",
			config:       bootstrapConfig(nil),
			bucketExists: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeS3()

			if tc.bucketExists {
				require.NoError(t, bootstrap(t, fake, bootstrapConfig(nil), &backend.Options{NonInteractive: true}))
			}

			changesBefore := fake.changes()

			err := bootstrap(t, fake, tc.config, &backend.Options{
				NonInteractive:               true,
				FailIfBucketCreationRequired: true,
			})

			assert.Equal(t, changesBefore, fake.changes(), "no bucket may be created or changed")

			if tc.wantRefusedBucket == "" {
				require.NoError(t, err)
				return
			}

			var notAllowed backend.BucketCreationNotAllowed

			require.ErrorAs(t, err, &notAllowed)
			assert.Equal(t, backend.BucketCreationNotAllowed(tc.wantRefusedBucket), notAllowed)
		})
	}
}

// bootstrap runs [s3backend.Backend.Bootstrap] against fake. Each call uses a new Backend, which has no record of the
// buckets an earlier call initialized.
func bootstrap(t *testing.T, fake *fakeS3, config backend.Config, opts *backend.Options) error {
	t.Helper()

	v := venv.MemVenv().
		WithHTTP(vhttp.NewMemClient(fake.handle)).
		WithEnv(map[string]string{
			"AWS_ACCESS_KEY_ID":     "fake-access-key-id",
			"AWS_SECRET_ACCESS_KEY": "fake-secret-access-key",
		})

	return s3backend.NewBackend().Bootstrap(t.Context(), logger.CreateLogger(), v, config, opts)
}

// bootstrapConfig returns a remote_state config for testBucketName that sends every request to [fakeS3], with the
// attributes in overrides added.
func bootstrapConfig(overrides backend.Config) backend.Config {
	config := backend.Config{
		"bucket":   testBucketName,
		"key":      "terraform.tfstate",
		"region":   "us-east-1",
		"endpoint": "https://s3.example.test",
		// Path style puts the bucket in req.URL.Path, where fakeS3 reads it.
		"use_path_style":              true,
		"skip_credentials_validation": true,
	}

	maps.Copy(config, overrides)

	return config
}

// bucketSetting is how S3 answers for one bucket setting, such as its policy or versioning.
type bucketSetting struct {
	// putOperation is the S3 operation that sets it, as [fakeS3.changes] records it.
	putOperation string
	// notFoundCode is the error code S3 returns for a setting that was never put.
	notFoundCode string
	// emptyRoot, when set, is the root element of the empty document S3 returns with a 200 for a setting that was
	// never put, in place of an error.
	emptyRoot string
}

// bucketSettings holds the settings [fakeS3] stores, keyed by the query parameter that addresses each one.
var bucketSettings = map[string]bucketSetting{
	"policy":            {putOperation: "PutBucketPolicy", notFoundCode: "NoSuchBucketPolicy"},
	"publicAccessBlock": {putOperation: "PutPublicAccessBlock", notFoundCode: "NoSuchPublicAccessBlockConfiguration"},
	"versioning":        {putOperation: "PutBucketVersioning", emptyRoot: "VersioningConfiguration"},
	"encryption": {
		putOperation: "PutBucketEncryption",
		notFoundCode: "ServerSideEncryptionConfigurationNotFoundError",
	},
	"logging": {putOperation: "PutBucketLogging", emptyRoot: "BucketLoggingStatus"},
	"tagging": {putOperation: "PutBucketTagging", notFoundCode: "NoSuchTagSet"},
}

// fakeS3 is an in-memory S3 that keeps the buckets created through it and the setting documents put on them, and
// records each request that creates or changes a bucket.
type fakeS3 struct {
	buckets map[string]map[string][]byte
	log     []string
	mu      sync.Mutex
}

func newFakeS3() *fakeS3 {
	return &fakeS3{buckets: map[string]map[string][]byte{}}
}

// changes returns the requests that created or changed a bucket, in the order they arrived, as "Operation bucket".
func (f *fakeS3) changes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return slices.Clone(f.log)
}

func (f *fakeS3) handle(_ context.Context, req *http.Request) (*http.Response, error) {
	// Enforced TLS and encryption read the partition and account ID from STS.
	if strings.HasPrefix(req.URL.Host, "sts.") {
		return vhttp.Respond(http.StatusOK, []byte(testCallerIdentityXML), nil), nil
	}

	bucket := strings.Trim(req.URL.Path, "/")
	setting := bucketSettingIn(req)

	f.mu.Lock()
	defer f.mu.Unlock()

	settings, exists := f.buckets[bucket]

	switch {
	case req.Method == http.MethodPut && setting == "" && !exists:
		f.buckets[bucket] = map[string][]byte{}
		f.log = append(f.log, "CreateBucket "+bucket)

		return vhttp.Respond(http.StatusOK, nil, nil), nil
	case !exists && req.Method == http.MethodHead:
		// S3 answers HeadBucket with no body, so the SDK reports the 404 as NotFound.
		return vhttp.Respond(http.StatusNotFound, nil, nil), nil
	case !exists:
		return s3Error(http.StatusNotFound, "NoSuchBucket"), nil
	case req.Method == http.MethodHead && setting == "":
		return vhttp.Respond(http.StatusOK, nil, nil), nil
	case req.Method == http.MethodPut && setting != "":
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		settings[setting] = body
		f.log = append(f.log, bucketSettings[setting].putOperation+" "+bucket)

		return vhttp.Respond(http.StatusOK, nil, nil), nil
	case req.Method == http.MethodGet && setting != "":
		if body, ok := settings[setting]; ok {
			return vhttp.Respond(http.StatusOK, body, nil), nil
		}

		if root := bucketSettings[setting].emptyRoot; root != "" {
			return vhttp.Respond(http.StatusOK, []byte("<"+root+"/>"), nil), nil
		}

		return s3Error(http.StatusNotFound, bucketSettings[setting].notFoundCode), nil
	}

	return s3Error(http.StatusNotImplemented, "NotImplemented"), nil
}

// bucketSettingIn returns the bucket setting req addresses, or "" when it addresses the bucket itself.
func bucketSettingIn(req *http.Request) string {
	query := req.URL.Query()

	for setting := range bucketSettings {
		if query.Has(setting) {
			return setting
		}
	}

	return ""
}

func s3Error(status int, code string) *http.Response {
	body := fmt.Sprintf("<Error><Code>%s</Code><Message>%s</Message></Error>", code, code)

	return vhttp.Respond(status, []byte(body), nil)
}
