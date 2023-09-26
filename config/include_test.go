package config

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeConfigIntoIncludedConfig(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		config         *TerragruntConfig
		includedConfig *TerragruntConfig
		expected       *TerragruntConfig
	}{
		{
			&TerragruntConfig{},
			&TerragruntConfig{},
			&TerragruntConfig{},
		},
		{
			&TerragruntConfig{},
			&TerragruntConfig{Terraform: &TerraformConfig{Source: ptr("foo")}},
			&TerragruntConfig{Terraform: &TerraformConfig{Source: ptr("foo")}},
		},
		{
			&TerragruntConfig{},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: ptr("foo")}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: ptr("foo")}},
		},
		{
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}, Terraform: &TerraformConfig{Source: ptr("foo")}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: ptr("foo")}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "foo"}, Terraform: &TerraformConfig{Source: ptr("foo")}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{Source: ptr("foo")}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: ptr("foo")}},
			&TerragruntConfig{RemoteState: &remote.RemoteState{Backend: "bar"}, Terraform: &TerraformConfig{Source: ptr("foo")}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "childArgs"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "childArgs"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "childArgs"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "parentArgs"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "parentArgs"}, TerraformExtraArguments{Name: "childArgs"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "overrideArgs", Arguments: &[]string{"-child"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "overrideArgs", Arguments: &[]string{"-parent"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{ExtraArgs: []TerraformExtraArguments{TerraformExtraArguments{Name: "overrideArgs", Arguments: &[]string{"-child"}}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: nil},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: nil},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "parentHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "parentHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "parentHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "parentHooks"}, Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"parent-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{BeforeHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "parentHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "parentHooks"}, Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"parent-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooks", Commands: []string{"child-apply"}}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooksPlusMore", Commands: []string{"child-apply"}}, Hook{Name: "childHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooksPlusMore", Commands: []string{"parent-apply"}}, Hook{Name: "parentHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideHooksPlusMore", Commands: []string{"child-apply"}}, Hook{Name: "parentHooks"}, Hook{Name: "childHooks"}}}},
		},
		{
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideWithEmptyHooks"}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideWithEmptyHooks", Commands: []string{"parent-apply"}}}}},
			&TerragruntConfig{Terraform: &TerraformConfig{AfterHooks: []Hook{Hook{Name: "overrideWithEmptyHooks"}}}},
		},
		{
			&TerragruntConfig{},
			&TerragruntConfig{Skip: true},
			&TerragruntConfig{Skip: false},
		},
		{
			&TerragruntConfig{Skip: false},
			&TerragruntConfig{Skip: true},
			&TerragruntConfig{Skip: false},
		},
		{
			&TerragruntConfig{Skip: true},
			&TerragruntConfig{Skip: true},
			&TerragruntConfig{Skip: true},
		},
		{
			&TerragruntConfig{IamRole: "role2"},
			&TerragruntConfig{IamRole: "role1"},
			&TerragruntConfig{IamRole: "role2"},
		},
	}

	for _, testCase := range testCases {
		// if nil, initialize to empty dependency list
		if testCase.expected.TerragruntDependencies == nil {
			testCase.expected.TerragruntDependencies = []Dependency{}
		}

		err := testCase.includedConfig.Merge(testCase.config, mockOptionsForTest(t))
		assert.NoError(t, err)
		assert.Equal(t, testCase.expected, testCase.includedConfig)
	}
}

func TestDeepMergeConfigIntoIncludedConfig(t *testing.T) {
	t.Parallel()

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

	testCases := []struct {
		name     string
		source   *TerragruntConfig
		target   *TerragruntConfig
		expected *TerragruntConfig
	}{
		// Base case: empty config
		{
			"base case",
			&TerragruntConfig{},
			&TerragruntConfig{},
			&TerragruntConfig{},
		},
		// Simple attribute in target
		{
			"simple in target",
			&TerragruntConfig{},
			&TerragruntConfig{IamRole: "foo"},
			&TerragruntConfig{IamRole: "foo"},
		},
		// Simple attribute in source
		{
			"simple in source",
			&TerragruntConfig{IamRole: "foo"},
			&TerragruntConfig{},
			&TerragruntConfig{IamRole: "foo"},
		},
		// Simple attribute in both
		{
			"simple in both",
			&TerragruntConfig{IamRole: "foo"},
			&TerragruntConfig{IamRole: "bar"},
			&TerragruntConfig{IamRole: "foo"},
		},
		// Deep merge dependencies
		{
			"dependencies",
			&TerragruntConfig{Dependencies: &ModuleDependencies{Paths: []string{"../vpc"}},
				TerragruntDependencies: []Dependency{
					{
						Name:       "vpc",
						ConfigPath: "../vpc",
					},
				}},
			&TerragruntConfig{Dependencies: &ModuleDependencies{Paths: []string{"../mysql"}},
				TerragruntDependencies: []Dependency{
					{
						Name:       "mysql",
						ConfigPath: "../mysql",
					},
				}},
			&TerragruntConfig{Dependencies: &ModuleDependencies{Paths: []string{"../mysql", "../vpc"}},
				TerragruntDependencies: []Dependency{
					{
						Name:       "mysql",
						ConfigPath: "../mysql",
					},
					{
						Name:       "vpc",
						ConfigPath: "../vpc",
					},
				}},
		},
		// Deep merge retryable errors
		{
			"retryable errors",
			&TerragruntConfig{RetryableErrors: []string{"error", "override"}},
			&TerragruntConfig{RetryableErrors: []string{"original", "error"}},
			&TerragruntConfig{RetryableErrors: []string{"original", "error", "error", "override"}},
		},
		// Deep merge inputs
		{
			"inputs",
			&TerragruntConfig{Inputs: overrideMap},
			&TerragruntConfig{Inputs: originalMap},
			&TerragruntConfig{Inputs: mergedMap},
		},
	}

	for _, testCase := range testCases {
		// No need to capture range var because tests are run sequentially
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.target.DeepMerge(testCase.source, mockOptionsForTest(t))
			require.NoError(t, err)

			// if nil, initialize to empty dependency list
			if testCase.expected.TerragruntDependencies == nil {
				testCase.expected.TerragruntDependencies = []Dependency{}
			}
			assert.Equal(t, testCase.expected, testCase.target)
		})
	}
}
