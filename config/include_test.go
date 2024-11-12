package config_test

import (
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestMergeConfigIntoIncludedConfig(t *testing.T) {
	t.Parallel()

	testTrue := true
	testFalse := false

	testCases := []struct {
		config         *config.TerragruntConfig
		includedConfig *config.TerragruntConfig
		expected       *config.TerragruntConfig
	}{
		{
			&config.TerragruntConfig{},
			&config.TerragruntConfig{},
			&config.TerragruntConfig{},
		},
		{
			&config.TerragruntConfig{},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			&config.TerragruntConfig{},
			&config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			&config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "childArgs"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "childArgs"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "childArgs"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "parentArgs"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "parentArgs"}, {Name: "childArgs"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "overrideArgs", Arguments: &[]string{"-child"}}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "overrideArgs", Arguments: &[]string{"-parent"}}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "overrideArgs", Arguments: &[]string{"-child"}}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
			&config.TerragruntConfig{Terraform: nil},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: nil},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "parentHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "parentHooks"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "parentHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "parentHooks"}, {Name: "childHooks"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"parent-apply"}}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "childHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "childHooks"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "childHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "parentHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "parentHooks"}, {Name: "childHooks"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"parent-apply"}}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooksPlusMore", Commands: []string{"child-apply"}}, {Name: "childHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooksPlusMore", Commands: []string{"parent-apply"}}, {Name: "parentHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooksPlusMore", Commands: []string{"child-apply"}}, {Name: "parentHooks"}, {Name: "childHooks"}}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideWithEmptyHooks"}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideWithEmptyHooks", Commands: []string{"parent-apply"}}}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideWithEmptyHooks"}}}},
		},
		{
			&config.TerragruntConfig{},
			&config.TerragruntConfig{Skip: &testTrue},
			&config.TerragruntConfig{Skip: &testTrue},
		},
		{
			&config.TerragruntConfig{Skip: &testFalse},
			&config.TerragruntConfig{Skip: &testTrue},
			&config.TerragruntConfig{Skip: &testFalse},
		},
		{
			&config.TerragruntConfig{Skip: &testTrue},
			&config.TerragruntConfig{Skip: &testTrue},
			&config.TerragruntConfig{Skip: &testTrue},
		},
		{
			&config.TerragruntConfig{IamRole: "role2"},
			&config.TerragruntConfig{IamRole: "role1"},
			&config.TerragruntConfig{IamRole: "role2"},
		},
		{
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
		},
		{
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
			&config.TerragruntConfig{IamWebIdentityToken: "token2"},
			&config.TerragruntConfig{IamWebIdentityToken: "token2"},
		},
		{
			&config.TerragruntConfig{},
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{IncludeInCopy: &[]string{"abc"}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], IncludeInCopy: &[]string{"abc"}}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExcludeFromCopy: &[]string{"abc"}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], ExcludeFromCopy: &[]string{"abc"}}},
		},
	}

	for _, testCase := range testCases {
		// if nil, initialize to empty dependency list
		if testCase.expected.TerragruntDependencies == nil {
			testCase.expected.TerragruntDependencies = config.Dependencies{}
		}

		err := testCase.includedConfig.Merge(testCase.config, mockOptionsForTest(t))
		require.NoError(t, err)
		assert.Equal(t, testCase.expected, testCase.includedConfig)
	}
}

func TestDeepMergeConfigIntoIncludedConfig(t *testing.T) {
	t.Parallel()

	testTrue := true
	testFalse := false

	// The following maps are convenience vars for setting up deep merge map tests
	overrideMap := map[string]interface{}{
		"simple_string_override": "hello, mock",
		"simple_string_append":   "new val",
		"list_attr":              []string{"mock"},
		"map_attr": map[string]interface{}{
			"simple_string_override": "hello, mock",
			"simple_string_append":   "new val",
			"list_attr":              []string{"mock"},
			"map_attr": map[string]interface{}{
				"simple_string_override": "hello, mock",
				"simple_string_append":   "new val",
				"list_attr":              []string{"mock"},
			},
		},
	}
	originalMap := map[string]interface{}{
		"simple_string_override": "hello, world",
		"original_string":        "original val",
		"list_attr":              []string{"hello"},
		"map_attr": map[string]interface{}{
			"simple_string_override": "hello, world",
			"original_string":        "original val",
			"list_attr":              []string{"hello"},
			"map_attr": map[string]interface{}{
				"simple_string_override": "hello, world",
				"original_string":        "original val",
				"list_attr":              []string{"hello"},
			},
		},
	}
	mergedMap := map[string]interface{}{
		"simple_string_override": "hello, mock",
		"original_string":        "original val",
		"simple_string_append":   "new val",
		"list_attr":              []string{"hello", "mock"},
		"map_attr": map[string]interface{}{
			"simple_string_override": "hello, mock",
			"original_string":        "original val",
			"simple_string_append":   "new val",
			"list_attr":              []string{"hello", "mock"},
			"map_attr": map[string]interface{}{
				"simple_string_override": "hello, mock",
				"original_string":        "original val",
				"simple_string_append":   "new val",
				"list_attr":              []string{"hello", "mock"},
			},
		},
	}

	tc := []struct {
		name     string
		source   *config.TerragruntConfig
		target   *config.TerragruntConfig
		expected *config.TerragruntConfig
	}{
		// Base case: empty config
		{
			"base case",
			&config.TerragruntConfig{},
			&config.TerragruntConfig{},
			&config.TerragruntConfig{},
		},
		// Simple attribute in target
		{
			"simple in target",
			&config.TerragruntConfig{},
			&config.TerragruntConfig{IamRole: "foo"},
			&config.TerragruntConfig{IamRole: "foo"},
		},
		// Simple attribute in source
		{
			"simple in source",
			&config.TerragruntConfig{IamRole: "foo"},
			&config.TerragruntConfig{},
			&config.TerragruntConfig{IamRole: "foo"},
		},
		// Simple attribute in both
		{
			"simple in both",
			&config.TerragruntConfig{IamRole: "foo"},
			&config.TerragruntConfig{IamRole: "bar"},
			&config.TerragruntConfig{IamRole: "foo"},
		},
		// skip related tests
		{
			"skip - preserve target",
			&config.TerragruntConfig{},
			&config.TerragruntConfig{Skip: &testTrue},
			&config.TerragruntConfig{Skip: &testTrue},
		},
		{
			"skip - copy source",
			&config.TerragruntConfig{Skip: &testFalse},
			&config.TerragruntConfig{Skip: &testTrue},
			&config.TerragruntConfig{Skip: &testFalse},
		},
		{
			"skip - still copy source",
			&config.TerragruntConfig{Skip: &testTrue},
			&config.TerragruntConfig{Skip: &testTrue},
			&config.TerragruntConfig{Skip: &testTrue},
		},
		// Deep merge dependencies
		{
			"dependencies",
			&config.TerragruntConfig{Dependencies: &config.ModuleDependencies{Paths: []string{"../vpc"}},
				TerragruntDependencies: config.Dependencies{
					config.Dependency{
						Name:       "vpc",
						ConfigPath: cty.StringVal("../vpc"),
					},
				}},
			&config.TerragruntConfig{Dependencies: &config.ModuleDependencies{Paths: []string{"../mysql"}},
				TerragruntDependencies: config.Dependencies{
					config.Dependency{
						Name:       "mysql",
						ConfigPath: cty.StringVal("../mysql"),
					},
				}},
			&config.TerragruntConfig{Dependencies: &config.ModuleDependencies{Paths: []string{"../mysql", "../vpc"}},
				TerragruntDependencies: config.Dependencies{
					config.Dependency{
						Name:       "mysql",
						ConfigPath: cty.StringVal("../mysql"),
					},
					config.Dependency{
						Name:       "vpc",
						ConfigPath: cty.StringVal("../vpc"),
					},
				}},
		},
		// Deep merge retryable errors
		{
			"retryable errors",
			&config.TerragruntConfig{RetryableErrors: []string{"error", "override"}},
			&config.TerragruntConfig{RetryableErrors: []string{"original", "error"}},
			&config.TerragruntConfig{RetryableErrors: []string{"original", "error", "error", "override"}},
		},
		// Deep merge inputs
		{
			"inputs",
			&config.TerragruntConfig{Inputs: overrideMap},
			&config.TerragruntConfig{Inputs: originalMap},
			&config.TerragruntConfig{Inputs: mergedMap},
		},
		{
			"terraform copy_terraform_lock_file",
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{IncludeInCopy: &[]string{"abc"}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], IncludeInCopy: &[]string{"abc"}}},
		},
		{
			"terraform copy_terraform_lock_file",
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{ExcludeFromCopy: &[]string{"abc"}}},
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], ExcludeFromCopy: &[]string{"abc"}}},
		},
	}

	for _, tt := range tc {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.target.DeepMerge(tt.source, mockOptionsForTest(t))
			require.NoError(t, err)

			// if nil, initialize to empty dependency list
			if tt.expected.TerragruntDependencies == nil {
				tt.expected.TerragruntDependencies = config.Dependencies{}
			}
			assert.Equal(t, tt.expected, tt.target)
		})
	}
}

func TestConcurrentCopyFieldsMetadata(t *testing.T) {
	t.Parallel()

	sourceConfig := &config.TerragruntConfig{
		FieldsMetadata: map[string]map[string]interface{}{
			"field1": {"key1": "value1", "key2": "value2"},
			"field2": {"key3": "value3", "key4": "value4"},
		},
	}

	targetConfig := &config.TerragruntConfig{}

	var wg sync.WaitGroup
	numGoroutines := 666

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			config.CopyFieldsMetadata(sourceConfig, targetConfig)
		}()
	}

	wg.Wait()

	// Optionally, here you can add assertions to check the integrity of the targetConfig
	// For example, checking if all keys and values have been copied correctly
	expectedFields := len(sourceConfig.FieldsMetadata)
	if len(targetConfig.FieldsMetadata) != expectedFields {
		t.Errorf("Expected %d fields, got %d", expectedFields, len(targetConfig.FieldsMetadata))
	}
}
