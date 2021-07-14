package config

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/remote"
	"github.com/stretchr/testify/assert"
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
		testCase.includedConfig.Merge(testCase.config, mockOptionsForTest(t))
		assert.Equal(t, testCase.expected, testCase.includedConfig)
	}
}
