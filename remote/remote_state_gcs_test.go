//go:build gcp

package remote_test

import (
	"testing"

	"cloud.google.com/go/storage"
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

	testCases := []struct {
		name               string
		config             map[string]interface{}
		expectedError      string
		skipBucketCreation bool
		bucketExists       bool
	}{
		{
			name: "Valid config with project and location",
			config: map[string]interface{}{
				"bucket":   "test-bucket",
				"project":  "test-project",
				"location": "US",
			},
			expectedError:      "",
			skipBucketCreation: false,
			bucketExists:       true,
		},
		{
			name: "Missing project when bucket does not exist and creation not skipped",
			config: map[string]interface{}{
				"bucket":   "test-bucket",
				"location": "US",
			},
			expectedError:      "Missing required GCS remote state configuration project",
			skipBucketCreation: false,
			bucketExists:       false,
		},
		{
			name: "Missing location when bucket does not exist and creation not skipped",
			config: map[string]interface{}{
				"bucket":  "test-bucket",
				"project": "test-project",
			},
			expectedError:      "Missing required GCS remote state configuration location",
			skipBucketCreation: false,
			bucketExists:       false,
		},
		{
			name: "Skip bucket creation allows missing project and location",
			config: map[string]interface{}{
				"bucket": "test-bucket",
			},
			expectedError:      "",
			skipBucketCreation: true,
			bucketExists:       false,
		},
		{
			name: "Existing bucket without project and location",
			config: map[string]interface{}{
				"bucket": "test-bucket",
			},
			expectedError:      "",
			skipBucketCreation: false,
			bucketExists:       true,
		},
		{
			name: "Non-existing bucket without project and location",
			config: map[string]interface{}{
				"bucket": "test-bucket",
			},
			expectedError:      "Missing required GCS remote state configuration project",
			skipBucketCreation: false,
			bucketExists:       false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Add skip_bucket_creation to the config if specified
			if tc.skipBucketCreation {
				tc.config["skip_bucket_creation"] = true
			}

			// Mock the CreateGCSClient and DoesGCSBucketExist functions
			originalCreateGCSClient := remote.CreateGCSClient
			originalDoesGCSBucketExist := remote.DoesGCSBucketExist
			defer func() {
				remote.CreateGCSClient = originalCreateGCSClient
				remote.DoesGCSBucketExist = originalDoesGCSBucketExist
			}()

			remote.CreateGCSClient = func(config remote.RemoteStateConfigGCS) (*storage.Client, error) {
				// Return a mock client
				return &storage.Client{}, nil
			}

			remote.DoesGCSBucketExist = func(client *storage.Client, config *remote.RemoteStateConfigGCS) bool {
				return tc.bucketExists
			}

			// Parse the config
			extendedConfig, err := remote.ParseExtendedGCSConfig(tc.config)
			require.NoError(t, err)

			// Validate the config
			err = remote.ValidateGCSConfig(extendedConfig)

			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			}
		})
	}
}
