package remote_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCSConfigValuesEqual(t *testing.T) {
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
