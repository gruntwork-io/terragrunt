package config_test

import (
	"sync"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestMergeConfigIntoIncludedConfig(t *testing.T) {
	t.Parallel()

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
			&config.TerragruntConfig{RemoteState: remotestate.New(&remotestate.Config{BackendName: "bar"}), Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: remotestate.New(&remotestate.Config{BackendName: "bar"}), Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			&config.TerragruntConfig{RemoteState: remotestate.New(&remotestate.Config{BackendName: "foo"}), Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: remotestate.New(&remotestate.Config{BackendName: "bar"}), Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: remotestate.New(&remotestate.Config{BackendName: "foo"}), Terraform: &config.TerraformConfig{Source: ptr("foo")}},
		},
		{
			&config.TerragruntConfig{Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: remotestate.New(&remotestate.Config{BackendName: "bar"}), Terraform: &config.TerraformConfig{Source: ptr("foo")}},
			&config.TerragruntConfig{RemoteState: remotestate.New(&remotestate.Config{BackendName: "bar"}), Terraform: &config.TerraformConfig{Source: ptr("foo")}},
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
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
		},
		{
			&config.TerragruntConfig{},
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
			&config.TerragruntConfig{IamWebIdentityToken: "token"},
		},
		{
			&config.TerragruntConfig{IamAssumeRoleSessionName: "session"},
			&config.TerragruntConfig{IamAssumeRoleSessionName: "session2"},
			&config.TerragruntConfig{IamAssumeRoleSessionName: "session"},
		},
		{
			&config.TerragruntConfig{},
			&config.TerragruntConfig{IamAssumeRoleSessionName: "session"},
			&config.TerragruntConfig{IamAssumeRoleSessionName: "session"},
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

	for _, tc := range testCases {
		// if nil, initialize to empty dependency list
		if tc.expected.TerragruntDependencies == nil {
			tc.expected.TerragruntDependencies = config.Dependencies{}
		}

		err := tc.includedConfig.Merge(logger.CreateLogger(), tc.config, mockOptionsForTest(t))
		require.NoError(t, err)
		assert.EqualExportedValues(t, tc.expected, tc.includedConfig)
	}
}

func TestDeepMergeConfigIntoIncludedConfig(t *testing.T) {
	t.Parallel()

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

	testCases := []struct {
		source   *config.TerragruntConfig
		target   *config.TerragruntConfig
		expected *config.TerragruntConfig
		name     string
	}{
		// Base case: empty config
		{
			name:     "base case",
			source:   &config.TerragruntConfig{},
			target:   &config.TerragruntConfig{},
			expected: &config.TerragruntConfig{},
		},
		// Simple attribute in target
		{
			name:     "simple in target",
			source:   &config.TerragruntConfig{},
			target:   &config.TerragruntConfig{IamRole: "foo"},
			expected: &config.TerragruntConfig{IamRole: "foo"},
		},
		// Simple attribute in source
		{
			name:     "simple in source",
			source:   &config.TerragruntConfig{IamRole: "foo"},
			target:   &config.TerragruntConfig{},
			expected: &config.TerragruntConfig{IamRole: "foo"},
		},
		// Simple attribute in both
		{
			name:     "simple in both",
			source:   &config.TerragruntConfig{IamRole: "foo"},
			target:   &config.TerragruntConfig{IamRole: "bar"},
			expected: &config.TerragruntConfig{IamRole: "foo"},
		},
		// skip related tests
		// Deep merge dependencies
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
		// Deep merge retryable errors
		// Deep merge inputs
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
			name:     "terraform copy_terraform_lock_file",
			source:   &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0]}},
			target:   &config.TerragruntConfig{Terraform: &config.TerraformConfig{ExcludeFromCopy: &[]string{"abc"}}},
			expected: &config.TerragruntConfig{Terraform: &config.TerraformConfig{CopyTerraformLockFile: &[]bool{false}[0], ExcludeFromCopy: &[]string{"abc"}}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.target.DeepMerge(logger.CreateLogger(), tc.source, mockOptionsForTest(t))
			require.NoError(t, err)

			// if nil, initialize to empty dependency list
			if tc.expected.TerragruntDependencies == nil {
				tc.expected.TerragruntDependencies = config.Dependencies{}
			}

			assert.Equal(t, tc.expected, tc.target)
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

func TestDependencyFileNotFoundError(t *testing.T) {
	t.Parallel()

	// Test that DependencyFileNotFoundError is properly defined and formatted
	err := config.DependencyFileNotFoundError{Path: "/test/path/terragrunt.hcl"}

	assert.Equal(t, "/test/path/terragrunt.hcl", err.Path)
	assert.Contains(t, err.Error(), "Dependency file not found: /test/path/terragrunt.hcl")

	// Test with a different path
	err2 := config.DependencyFileNotFoundError{Path: "/another/path/config.hcl"}
	assert.Equal(t, "/another/path/config.hcl", err2.Path)
	assert.Contains(t, err2.Error(), "Dependency file not found: /another/path/config.hcl")
}

func TestIncludeConfigNotFoundError(t *testing.T) {
	t.Parallel()

	// Test that IncludeConfigNotFoundError is properly defined and formatted
	err := config.IncludeConfigNotFoundError{IncludePath: "/test/path/terragrunt.hcl", SourcePath: "/source/config.hcl"}

	assert.Equal(t, "/test/path/terragrunt.hcl", err.IncludePath)
	assert.Equal(t, "/source/config.hcl", err.SourcePath)
	assert.Contains(t, err.Error(), "Include configuration not found: /test/path/terragrunt.hcl (referenced from: /source/config.hcl)")

	// Test with a different path
	err2 := config.IncludeConfigNotFoundError{IncludePath: "/another/path/config.hcl", SourcePath: "/different/source.hcl"}
	assert.Equal(t, "/another/path/config.hcl", err2.IncludePath)
	assert.Equal(t, "/different/source.hcl", err2.SourcePath)
	assert.Contains(t, err2.Error(), "Include configuration not found: /another/path/config.hcl (referenced from: /different/source.hcl)")
}
