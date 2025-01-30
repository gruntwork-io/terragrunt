//go:build gcp

package remote_test

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGcpConfigValuesEqual(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("remote_state_test")
	require.NoError(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	testCases := []struct {
		name          string
		config        map[string]interface{}
		backend       *remote.TerraformBackend
		shouldBeEqual bool
	}{
		{
			"equal-both-empty",
			map[string]interface{}{},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{}},
			true,
		},
		{
			"equal-empty-and-nil",
			map[string]interface{}{},
			nil,
			true,
		},
		{
			"equal-empty-and-nil-backend-config",
			map[string]interface{}{},
			&remote.TerraformBackend{Type: "gcs"},
			true,
		},
		{
			"equal-one-key",
			map[string]interface{}{"foo": "bar"},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"foo": "bar"}},
			true,
		},
		{
			"equal-multiple-keys",
			map[string]interface{}{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true}},
			true,
		},
		{
			"equal-encrypt-bool-handling",
			map[string]interface{}{"encrypt": true},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"encrypt": "true"}},
			true,
		},
		{
			"equal-general-bool-handling",
			map[string]interface{}{"something": true, "encrypt": true},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"something": "true", "encrypt": "true"}},
			true,
		},
		{
			"equal-ignore-gcs-labels",
			map[string]interface{}{"foo": "bar", "gcs_bucket_labels": []map[string]string{{"foo": "bar"}}},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"foo": "bar"}},
			true,
		},
		{
			"unequal-wrong-backend",
			map[string]interface{}{"foo": "bar"},
			&remote.TerraformBackend{Type: "wrong", Config: map[string]interface{}{"foo": "bar"}},
			false,
		},
		{
			"unequal-values",
			map[string]interface{}{"foo": "bar"},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"foo": "different"}},
			false,
		},
		{
			"unequal-non-empty-config-nil",
			map[string]interface{}{"foo": "bar"},
			nil,
			false,
		},
		{
			"unequal-general-bool-handling",
			map[string]interface{}{"something": true},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"something": "false"}},
			false,
		},
		{
			"equal-null-ignored",
			map[string]interface{}{"something": "foo"},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"something": "foo", "ignored-because-null": nil}},
			true,
		},
		{
			"terragrunt-only-configs-remain-intact",
			map[string]interface{}{"something": "foo", "skip_bucket_creation": true},
			&remote.TerraformBackend{Type: "gcs", Config: map[string]interface{}{"something": "foo"}},
			true,
		},
	}

	for _, testCase := range testCases {
		// Save the testCase in local scope so all the t.Run calls don't end up with the last item in the list
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			// Create a copy of the new config
			config := make(map[string]interface{})
			for key, value := range testCase.config {
				config[key] = value
			}

			actual := remote.GCSConfigValuesEqual(config, testCase.backend, terragruntOptions)
			assert.Equal(t, testCase.shouldBeEqual, actual)

			// Ensure the config remains unchanged by the comparison
			assert.Equal(t, testCase.config, config)
		})
	}
}

func TestValidateGCSConfig(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		config           map[string]interface{}
		expectedError    bool
		mockBucketExists bool // Simulate whether the bucket exists
	}{
		"Valid_config_with_project_and_location": {
			config: map[string]interface{}{
				"bucket":   "test-bucket",
				"project":  "test-project",
				"location": "us-central1",
			},
			expectedError:    false,
			mockBucketExists: false,
		},
		"Valid_config_with_skip_bucket_creation": {
			config: map[string]interface{}{
				"bucket":               "test-bucket",
				"skip_bucket_creation": true,
			},
			expectedError:    false,
			mockBucketExists: false,
		},
		"Missing_bucket": {
			config: map[string]interface{}{
				"project":  "test-project",
				"location": "us-central1",
			},
			expectedError:    true,
			mockBucketExists: false,
		},
		"Missing_project_when_bucket_does_not_exist": {
			config: map[string]interface{}{
				"bucket":   "test-bucket",
				"location": "us-central1",
			},
			expectedError:    true,
			mockBucketExists: false,
		},
		"Missing_location_when_bucket_does_not_exist": {
			config: map[string]interface{}{
				"bucket":  "test-bucket",
				"project": "test-project",
			},
			expectedError:    true,
			mockBucketExists: false,
		},
		"Existing_bucket_without_project_and_location_when_skip_bucket_creation_is_true": {
			config: map[string]interface{}{
				"bucket":               "test-bucket",
				"skip_bucket_creation": true,
			},
			expectedError:    false,
			mockBucketExists: true,
		},
		"Existing_bucket_without_project_and_location_when_bucket_exists": {
			config: map[string]interface{}{
				"bucket": "test-bucket",
			},
			expectedError:    false,
			mockBucketExists: true,
		},
		"Missing_project_when_bucket_exists": {
			config: map[string]interface{}{
				"bucket":   "test-bucket",
				"location": "us-central1",
			},
			expectedError:    false,
			mockBucketExists: true,
		},
		"Missing_location_when_bucket_exists": {
			config: map[string]interface{}{
				"bucket":  "test-bucket",
				"project": "test-project",
			},
			expectedError:    false,
			mockBucketExists: true,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Set up the mock bucket handle for existence check
			mockBucketHandle := &mockBucketHandle{doesExist: tc.mockBucketExists}

			extendedConfig, err := remote.ParseExtendedGCSConfig(tc.config)
			require.NoError(t, err)

			err = remote.ValidateGCSConfigWithHandle(context.TODO(), mockBucketHandle, extendedConfig)

			if tc.expectedError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type mockBucketHandle struct {
	doesExist bool
}

func (m *mockBucketHandle) Attrs(ctx context.Context) (*storage.BucketAttrs, error) {
	if m.doesExist {
		return &storage.BucketAttrs{}, nil
	}

	return nil, errors.New("bucket does not exist")
}

func (m *mockBucketHandle) Objects(ctx context.Context, q *storage.Query) *storage.ObjectIterator {
	if m.doesExist {
		return &storage.ObjectIterator{}
	}

	return &storage.ObjectIterator{}
}
