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
			config:         &config.TerragruntConfig{},
			includedConfig: &config.TerragruntConfig{},
			expected:       &config.TerragruntConfig{},
		},
		{
			config:         &config.TerragruntConfig{},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			config:         &config.TerragruntConfig{},
			includedConfig: &config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			expected:       &config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			config:         &config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			includedConfig: &config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			expected:       &config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			includedConfig: &config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			expected:       &config.TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "childArgs"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "childArgs"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "childArgs"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "parentArgs"}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "parentArgs"}, {Name: "childArgs"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "overrideArgs", Arguments: &[]string{"-child"}}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "overrideArgs", Arguments: &[]string{"-parent"}}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExtraArgs: []config.TerraformExtraArguments{{Name: "overrideArgs", Arguments: &[]string{"-child"}}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: nil},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: nil},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "parentHooks"}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "parentHooks"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "childHooks"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "parentHooks"}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "parentHooks"}, {Name: "childHooks"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"parent-apply"}}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{BeforeHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "childHooks"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "childHooks"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "childHooks"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "parentHooks"}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "parentHooks"}, {Name: "childHooks"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"parent-apply"}}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooksPlusMore", Commands: []string{"child-apply"}}, {Name: "childHooks"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooksPlusMore", Commands: []string{"parent-apply"}}, {Name: "parentHooks"}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideHooksPlusMore", Commands: []string{"child-apply"}}, {Name: "parentHooks"}, {Name: "childHooks"}}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideWithEmptyHooks"}}}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideWithEmptyHooks", Commands: []string{"parent-apply"}}}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{AfterHooks: []config.Hook{{Name: "overrideWithEmptyHooks"}}}},
		},
		{
			config:         &config.TerragruntConfig{},
			includedConfig: &config.TerragruntConfig{Skip: &testTrue},
			expected:       &config.TerragruntConfig{Skip: &testTrue},
		},
		{
			config:         &config.TerragruntConfig{Skip: &testFalse},
			includedConfig: &config.TerragruntConfig{Skip: &testTrue},
			expected:       &config.TerragruntConfig{Skip: &testFalse},
		},
		{
			config:         &config.TerragruntConfig{Skip: &testTrue},
			includedConfig: &config.TerragruntConfig{Skip: &testTrue},
			expected:       &config.TerragruntConfig{Skip: &testTrue},
		},
		{
			config:         &config.TerragruntConfig{IamRole: "role2"},
			includedConfig: &config.TerragruntConfig{IamRole: "role1"},
			expected:       &config.TerragruntConfig{IamRole: "role2"},
		},
		{
			config:         &config.TerragruntConfig{IamWebIdentityToken: "token"},
			includedConfig: &config.TerragruntConfig{IamWebIdentityToken: "token"},
			expected:       &config.TerragruntConfig{IamWebIdentityToken: "token"},
		},
		{
			config:         &config.TerragruntConfig{IamWebIdentityToken: "token"},
			includedConfig: &config.TerragruntConfig{IamWebIdentityToken: "token2"},
			expected:       &config.TerragruntConfig{IamWebIdentityToken: "token2"},
		},
		{
			config:         &config.TerragruntConfig{},
			includedConfig: &config.TerragruntConfig{IamWebIdentityToken: "token"},
			expected:       &config.TerragruntConfig{IamWebIdentityToken: "token"},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{IncludeInCopy: &[]string{"abc"}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], IncludeInCopy: &[]string{"abc"}}},
		},
		{
			config:         &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			includedConfig: &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExcludeFromCopy: &[]string{"abc"}}},
			expected:       &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], ExcludeFromCopy: &[]string{"abc"}}},
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
	overrideMap := map[string]any{
		"simple_string_override": "hello, mock",
		"simple_string_append":   "new val",
		"list_attr":              []string{"mock"},
		"map_attr": map[string]any{
			"simple_string_override": "hello, mock",
			"simple_string_append":   "new val",
			"list_attr":              []string{"mock"},
			"map_attr": map[string]any{
				"simple_string_override": "hello, mock",
				"simple_string_append":   "new val",
				"list_attr":              []string{"mock"},
			},
		},
	}
	originalMap := map[string]any{
		"simple_string_override": "hello, world",
		"original_string":        "original val",
		"list_attr":              []string{"hello"},
		"map_attr": map[string]any{
			"simple_string_override": "hello, world",
			"original_string":        "original val",
			"list_attr":              []string{"hello"},
			"map_attr": map[string]any{
				"simple_string_override": "hello, world",
				"original_string":        "original val",
				"list_attr":              []string{"hello"},
			},
		},
	}
	mergedMap := map[string]any{
		"simple_string_override": "hello, mock",
		"original_string":        "original val",
		"simple_string_append":   "new val",
		"list_attr":              []string{"hello", "mock"},
		"map_attr": map[string]any{
			"simple_string_override": "hello, mock",
			"original_string":        "original val",
			"simple_string_append":   "new val",
			"list_attr":              []string{"hello", "mock"},
			"map_attr": map[string]any{
				"simple_string_override": "hello, mock",
				"original_string":        "original val",
				"simple_string_append":   "new val",
				"list_attr":              []string{"hello", "mock"},
			},
		},
	}

	tc := []struct {
		source   *config.TerragruntConfig
		target   *config.TerragruntConfig
		expected *config.TerragruntConfig
		name     string
	}{
		{
			name:     "base case",
			source:   &config.TerragruntConfig{},
			target:   &config.TerragruntConfig{},
			expected: &config.TerragruntConfig{},
		},
		{
			name:     "simple in target",
			source:   &config.TerragruntConfig{},
			target:   &config.TerragruntConfig{IamRole: "foo"},
			expected: &config.TerragruntConfig{IamRole: "foo"},
		},
		{
			name:     "simple in source",
			source:   &config.TerragruntConfig{IamRole: "foo"},
			target:   &config.TerragruntConfig{},
			expected: &config.TerragruntConfig{IamRole: "foo"},
		},
		{
			name:     "simple in both",
			source:   &config.TerragruntConfig{IamRole: "foo"},
			target:   &config.TerragruntConfig{IamRole: "bar"},
			expected: &config.TerragruntConfig{IamRole: "foo"},
		},
		{
			name:     "skip - preserve target",
			source:   &config.TerragruntConfig{},
			target:   &config.TerragruntConfig{Skip: &testTrue},
			expected: &config.TerragruntConfig{Skip: &testTrue},
		},
		{
			name:     "skip - copy source",
			source:   &config.TerragruntConfig{Skip: &testFalse},
			target:   &config.TerragruntConfig{Skip: &testTrue},
			expected: &config.TerragruntConfig{Skip: &testFalse},
		},
		{
			name:     "skip - still copy source",
			source:   &config.TerragruntConfig{Skip: &testTrue},
			target:   &config.TerragruntConfig{Skip: &testTrue},
			expected: &config.TerragruntConfig{Skip: &testTrue},
		},
		{
			name: "dependencies",
			source: &config.TerragruntConfig{Dependencies: &config.ModuleDependencies{Paths: []string{"../vpc"}},
				TerragruntDependencies: config.Dependencies{
					config.Dependency{
						Name:       "vpc",
						ConfigPath: cty.StringVal("../vpc"),
					},
				}},
			target: &config.TerragruntConfig{Dependencies: &config.ModuleDependencies{Paths: []string{"../mysql"}},
				TerragruntDependencies: config.Dependencies{
					config.Dependency{
						Name:       "mysql",
						ConfigPath: cty.StringVal("../mysql"),
					},
				}},
			expected: &config.TerragruntConfig{Dependencies: &config.ModuleDependencies{Paths: []string{"../mysql", "../vpc"}},
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
		{
			name:     "retryable errors",
			source:   &config.TerragruntConfig{RetryableErrors: []string{"error", "override"}},
			target:   &config.TerragruntConfig{RetryableErrors: []string{"original", "error"}},
			expected: &config.TerragruntConfig{RetryableErrors: []string{"original", "error", "error", "override"}},
		},
		{
			name:     "inputs",
			source:   &config.TerragruntConfig{Inputs: overrideMap},
			target:   &config.TerragruntConfig{Inputs: originalMap},
			expected: &config.TerragruntConfig{Inputs: mergedMap},
		},
		{
			name:     "terraform copy_terraform_lock_file",
			source:   &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			target:   &config.TerragruntConfig{Terraform: &config.TerraformConfig{IncludeInCopy: &[]string{"abc"}}},
			expected: &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], IncludeInCopy: &[]string{"abc"}}},
		},
		{
			name:     "terraform copy_terraform_lock_file with exclude",
			source:   &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			target:   &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExcludeFromCopy: &[]string{"abc"}}},
			expected: &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], ExcludeFromCopy: &[]string{"abc"}}},
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
		FieldsMetadata: map[string]map[string]any{
			"field1": {"key1": "value1", "key2": "value2"},
			"field2": {"key3": "value3", "key4": "value4"},
		},
	}

	targetConfig := &config.TerragruntConfig{}

	var wg sync.WaitGroup
	numGoroutines := 666

	wg.Add(numGoroutines)
	for range numGoroutines {
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
