package remote

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureRMConfigValuesEqual(t *testing.T) {
	t.Parallel()

	terragruntOptions, err := options.NewTerragruntOptionsForTest("remote_state_test")
	require.Nil(t, err, "Unexpected error creating NewTerragruntOptionsForTest: %v", err)

	testCases := []struct {
		name          string
		config        map[string]interface{}
		backend       *TerraformBackend
		shouldBeEqual bool
	}{
		{
			"equal-both-empty",
			map[string]interface{}{},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{}},
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
			&TerraformBackend{Type: "azurerm"},
			true,
		},
		{
			"equal-one-key",
			map[string]interface{}{"foo": "bar"},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"foo": "bar"}},
			true,
		},
		{
			"equal-multiple-keys",
			map[string]interface{}{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"foo": "bar", "baz": []string{"a", "b", "c"}, "blah": 123, "bool": true}},
			true,
		},
		{
			"equal-encrypt-bool-handling",
			map[string]interface{}{"encrypt": true},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"encrypt": "true"}},
			true,
		},
		{
			"equal-general-bool-handling",
			map[string]interface{}{"something": true, "encrypt": true},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"something": "true", "encrypt": "true"}},
			true,
		},
		{
			"equal-ignore-gcs-labels",
			map[string]interface{}{"foo": "bar", "tags": []map[string]string{{"foo": "bar"}}},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"foo": "bar"}},
			true,
		},
		{
			"unequal-wrong-backend",
			map[string]interface{}{"foo": "bar"},
			&TerraformBackend{Type: "wrong", Config: map[string]interface{}{"foo": "bar"}},
			false,
		},
		{
			"unequal-values",
			map[string]interface{}{"foo": "bar"},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"foo": "different"}},
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
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"something": "false"}},
			false,
		},
		{
			"equal-null-ignored",
			map[string]interface{}{"something": "foo"},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"something": "foo", "ignored-because-null": nil}},
			true,
		},
		{
			"terragrunt-only-configs-remain-intact",
			map[string]interface{}{"something": "foo", "skip_create": true},
			&TerraformBackend{Type: "azurerm", Config: map[string]interface{}{"something": "foo"}},
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

			actual := armConfigValuesEqual(config, testCase.backend, terragruntOptions)
			assert.Equal(t, testCase.shouldBeEqual, actual)

			// Ensure the config remains unchanged by the comparison
			assert.Equal(t, testCase.config, config)
		})
	}
}
