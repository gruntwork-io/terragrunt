package s3_test

import (
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/internal/strict/controls"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackend_GetTFInitArgs(t *testing.T) {
	t.Parallel()

	remoteBackend := s3backend.NewBackend()

	testCases := []struct {
		config        backend.Config
		expected      map[string]any
		name          string
		shouldBeEqual bool
	}{
		{
			name:          "empty-no-values",
			config:        backend.Config{},
			expected:      map[string]any{},
			shouldBeEqual: true,
		},
		{
			name: "valid-s3-configuration-keys",
			config: backend.Config{
				"bucket":  "foo",
				"encrypt": "bar",
				"key":     "baz",
				"region":  "quux",
			},
			expected: map[string]any{
				"bucket":  "foo",
				"encrypt": "bar",
				"key":     "baz",
				"region":  "quux",
			},
			shouldBeEqual: true,
		},
		{
			name: "terragrunt-keys-filtered",
			config: backend.Config{
				"bucket":                      "foo",
				"encrypt":                     "bar",
				"key":                         "baz",
				"region":                      "quux",
				"skip_credentials_validation": true,
				"s3_bucket_tags":              map[string]string{},
			},
			expected: map[string]any{
				"bucket":                      "foo",
				"encrypt":                     "bar",
				"key":                         "baz",
				"region":                      "quux",
				"skip_credentials_validation": true,
			},
			shouldBeEqual: true,
		},
		{
			name: "empty-no-values-all-terragrunt-keys-filtered",
			config: backend.Config{
				"s3_bucket_tags":                                    map[string]string{},
				"dynamodb_table_tags":                               map[string]string{},
				"accesslogging_bucket_tags":                         map[string]string{},
				"skip_bucket_versioning":                            true,
				"skip_bucket_ssencryption":                          false,
				"skip_bucket_root_access":                           false,
				"enable_bucket_root_access":                         true,
				"skip_bucket_enforced_tls":                          false,
				"skip_bucket_public_access_blocking":                false,
				"disable_bucket_update":                             true,
				"enable_lock_table_ssencryption":                    true,
				"disable_aws_client_checksums":                      false,
				"accesslogging_bucket_name":                         "test",
				"accesslogging_target_object_partition_date_source": "EventTime",
				"accesslogging_target_prefix":                       "test",
				"skip_accesslogging_bucket_acl":                     false,
				"skip_accesslogging_bucket_policy":                  false,
				"skip_accesslogging_bucket_enforced_tls":            false,
				"skip_accesslogging_bucket_public_access_blocking":  false,
				"skip_accesslogging_bucket_ssencryption":            false,
			},
			expected:      map[string]any{},
			shouldBeEqual: true,
		},
		{
			name: "lock-table-replaced-with-dynamodb-table",
			config: backend.Config{
				"bucket":     "foo",
				"encrypt":    "bar",
				"key":        "baz",
				"region":     "quux",
				"lock_table": "xyzzy",
			},
			expected: map[string]any{
				"bucket":         "foo",
				"encrypt":        "bar",
				"key":            "baz",
				"region":         "quux",
				"dynamodb_table": "xyzzy",
			},
			shouldBeEqual: true,
		},
		{
			name: "dynamodb-table-not-replaced-with-lock-table",
			config: backend.Config{
				"bucket":         "foo",
				"encrypt":        "bar",
				"key":            "baz",
				"region":         "quux",
				"dynamodb_table": "xyzzy",
			},
			expected: map[string]any{
				"bucket":     "foo",
				"encrypt":    "bar",
				"key":        "baz",
				"region":     "quux",
				"lock_table": "xyzzy",
			},
			shouldBeEqual: false,
		},
		{
			name: "assume-role",
			config: backend.Config{
				"bucket": "foo",
				"assume_role": map[string]any{
					"role_arn":     "arn:aws:iam::123:role/role",
					"external_id":  "123",
					"session_name": "qwe",
				},
			},
			expected: map[string]any{
				"bucket":      "foo",
				"assume_role": "{external_id=\"123\",role_arn=\"arn:aws:iam::123:role/role\",session_name=\"qwe\"}",
			},
			shouldBeEqual: true,
		},
		{
			name: "use-lockfile-native-s3-locking",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			shouldBeEqual: true,
		},
		{
			name: "use-lockfile-false",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": false,
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": false,
			},
			shouldBeEqual: true,
		},
		{
			name: "dual-locking-dynamodb-and-s3",
			config: backend.Config{
				"bucket":         "foo",
				"key":            "bar",
				"region":         "us-east-1",
				"dynamodb_table": "my-lock-table",
				"use_lockfile":   true,
			},
			expected: map[string]any{
				"bucket":         "foo",
				"key":            "bar",
				"region":         "us-east-1",
				"dynamodb_table": "my-lock-table",
				"use_lockfile":   true,
			},
			shouldBeEqual: true,
		},
		{
			name: "string-bool-use-lockfile-true",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": "true",
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": true,
			},
			shouldBeEqual: true,
		},
		{
			name: "string-bool-use-lockfile-false",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": "false",
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"use_lockfile": false,
			},
			shouldBeEqual: true,
		},
		{
			name: "string-bool-encrypt-and-use-lockfile",
			config: backend.Config{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"encrypt":      "true",
				"use_lockfile": "true",
			},
			expected: map[string]any{
				"bucket":       "foo",
				"key":          "bar",
				"region":       "us-east-1",
				"encrypt":      true,
				"use_lockfile": true,
			},
			shouldBeEqual: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			actual := remoteBackend.GetTFInitArgs(tc.config)

			if !tc.shouldBeEqual {
				assert.NotEqual(t, tc.expected, actual)
				return
			}

			assert.Equal(t, tc.expected, actual)
		})
	}
}

// TestBackend_NeedsBootstrapSkipBucketRootAccessStrictControl verifies that the deprecated
// `skip_bucket_root_access` attribute is reported through its strict control, and that enabling
// the control turns the deprecation into an error before any AWS call is made.
func TestBackend_NeedsBootstrapSkipBucketRootAccessStrictControl(t *testing.T) {
	t.Parallel()

	strictControls := controls.New()

	ctrl, ok := strictControls.Find(controls.SkipBucketRootAccess).(*controls.Control)
	require.True(t, ok)

	strictControls.FilterByNames(controls.SkipBucketRootAccess).Enable()

	_, err := s3backend.NewBackend().NeedsBootstrap(
		t.Context(),
		logger.CreateLogger(),
		nil,
		backend.Config{
			"bucket":                  "my-bucket",
			"key":                     "my-key",
			"region":                  "us-east-1",
			"skip_bucket_root_access": false,
		},
		&backend.Options{StrictControls: strictControls},
	)

	require.ErrorIs(t, err, ctrl.Error)
}

// fakeS3Bucket serves the bucket-level S3 calls NeedsBootstrap makes for one existing bucket.
func fakeS3Bucket(t *testing.T, bucket, policy string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSuffix(r.URL.Path, "/") != "/"+bucket {
			http.NotFound(w, r)
			return
		}

		q := r.URL.Query()

		switch {
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusOK)
		case q.Has("versioning"):
			fmt.Fprint(w, `<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>`)
		case q.Has("encryption"):
			fmt.Fprint(w, `<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault>`+
				`<SSEAlgorithm>aws:kms</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`)
		case q.Has("publicAccessBlock"):
			fmt.Fprint(w, `<PublicAccessBlockConfiguration><BlockPublicAcls>true</BlockPublicAcls>`+
				`<IgnorePublicAcls>true</IgnorePublicAcls><BlockPublicPolicy>true</BlockPublicPolicy>`+
				`<RestrictPublicBuckets>true</RestrictPublicBuckets></PublicAccessBlockConfiguration>`)
		case q.Has("policy"):
			if policy == "denied" {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>Access Denied</Message></Error>`)

				return
			}

			if policy == "" {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `<Error><Code>NoSuchBucketPolicy</Code><Message>The bucket policy does not exist</Message></Error>`)

				return
			}

			fmt.Fprint(w, policy)
		default:
			http.Error(w, "unexpected request "+r.URL.String(), http.StatusNotImplemented)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestBackend_NeedsBootstrapExistingBucketOutOfDate(t *testing.T) {
	t.Parallel()

	const bucket = "tg-state-store"

	enforcedTLSPolicy := `{"Version":"2012-10-17","Statement":[{"Sid":"` + s3backend.SidEnforcedTLSPolicy +
		`","Effect":"Deny","Principal":"*","Action":"s3:*","Resource":"*"}]}`

	testCases := []struct {
		extraConfig backend.Config
		name        string
		policy      string
		expected    bool
	}{
		{name: "missing-enforced-tls-policy", expected: true},
		{name: "up-to-date", policy: enforcedTLSPolicy, expected: false},
		{
			name:        "missing-root-access-policy",
			extraConfig: backend.Config{"enable_bucket_root_access": true},
			policy:      enforcedTLSPolicy,
			expected:    true,
		},
		{name: "policy-not-readable", policy: "denied", expected: false},
		{name: "skip-enforced-tls", extraConfig: backend.Config{"skip_bucket_enforced_tls": true}, expected: false},
		{name: "disable-bucket-update", extraConfig: backend.Config{"disable_bucket_update": true}, expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := fakeS3Bucket(t, bucket, tc.policy)

			config := backend.Config{
				"bucket":                      bucket,
				"key":                         "terraform.tfstate",
				"region":                      "us-east-1",
				"endpoints":                   map[string]any{"s3": srv.URL},
				"force_path_style":            true,
				"skip_credentials_validation": true,
			}
			maps.Copy(config, tc.extraConfig)

			v := venvtest.New().
				WithHTTP(vhttp.NewOSClient()).
				WithEnv(map[string]string{
					"AWS_ACCESS_KEY_ID":     "test-key",
					"AWS_SECRET_ACCESS_KEY": "test-secret",
				})

			needs, err := s3backend.NewBackend().NeedsBootstrap(t.Context(), logger.CreateLogger(), v, config, &backend.Options{})
			require.NoError(t, err)
			assert.Equal(t, tc.expected, needs)
		})
	}
}
