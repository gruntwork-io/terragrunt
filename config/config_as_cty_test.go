package config_test

import (
	"sort"
	"testing"

	"github.com/fatih/structs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// This test makes sure that all the fields from the TerragruntConfig struct are accounted for in the conversion to
// cty.Value.
func TestTerragruntConfigAsCtyDrift(t *testing.T) {
	t.Parallel()

	testSource := "./foo"
	testTrue := true
	testFalse := false
	mockOutputs := cty.Zero
	mockOutputsAllowedTerraformCommands := []string{"init"}
	dependentModulesPath := []*string{&testSource}
	metaVal := cty.MapVal(map[string]cty.Value{
		"foo": cty.StringVal("bar"),
	})
	testConfig := config.TerragruntConfig{
		Engine: &config.EngineConfig{
			Source: "github.com/acme/terragrunt-plugin-custom-opentofu",
			Meta:   &metaVal,
		},
		Catalog: &config.CatalogConfig{
			URLs: []string{
				"repo/path",
			},
		},
		Terraform: &config.TerraformConfig{
			Source: &testSource,
			ExtraArgs: []config.TerraformExtraArguments{
				{
					Name:     "init",
					Commands: []string{"init"},
				},
			},
			BeforeHooks: []config.Hook{
				{
					Name:     "init",
					Commands: []string{"init"},
					Execute:  []string{"true"},
				},
			},
			AfterHooks: []config.Hook{
				{
					Name:     "init",
					Commands: []string{"init"},
					Execute:  []string{"true"},
				},
			},
			ErrorHooks: []config.ErrorHook{
				{
					Name:     "init",
					Commands: []string{"init"},
					Execute:  []string{"true"},
					OnErrors: []string{".*"},
				},
			},
		},
		TerraformBinary:             "terraform",
		TerraformVersionConstraint:  "= 0.12.20",
		TerragruntVersionConstraint: "= 0.23.18",
		RemoteState: remotestate.New(&remotestate.Config{
			BackendName:                   "foo",
			DisableInit:                   true,
			DisableDependencyOptimization: true,
			BackendConfig: map[string]any{
				"bar": "baz",
			},
		}),
		Dependencies: &config.ModuleDependencies{
			Paths: []string{"foo"},
		},
		DownloadDir:    ".terragrunt-cache",
		PreventDestroy: &testTrue,
		Skip:           &testTrue,
		IamRole:        "terragruntRole",
		Inputs: map[string]any{
			"aws_region": "us-east-1",
		},
		Locals: map[string]any{
			"quote": "the answer is 42",
		},
		DependentModulesPath: dependentModulesPath,
		TerragruntDependencies: config.Dependencies{
			config.Dependency{
				Name:                                "foo",
				ConfigPath:                          cty.StringVal("foo"),
				SkipOutputs:                         &testTrue,
				MockOutputs:                         &mockOutputs,
				MockOutputsAllowedTerraformCommands: &mockOutputsAllowedTerraformCommands,
				MockOutputsMergeWithState:           &testFalse,
				RenderedOutputs:                     &mockOutputs,
			},
		},
		FeatureFlags: config.FeatureFlags{
			&config.FeatureFlag{
				Name:    "test",
				Default: &cty.Zero,
			},
		},
		Errors: &config.ErrorsConfig{
			Retry: []*config.RetryBlock{
				{
					Label:            "test",
					RetryableErrors:  []string{"test"},
					MaxAttempts:      0,
					SleepIntervalSec: 0,
				},
			},
			Ignore: []*config.IgnoreBlock{
				{
					Label:           "test",
					IgnorableErrors: nil,
					Message:         "",
					Signals:         nil,
				},
			},
		},
		GenerateConfigs: map[string]codegen.GenerateConfig{
			"provider": {
				Path:          "foo",
				IfExists:      codegen.ExistsOverwriteTerragrunt,
				IfExistsStr:   "overwrite_terragrunt",
				CommentPrefix: "# ",
				Contents: `terraform {
  backend "s3" {}
}`,
			},
		},
		Exclude: &config.ExcludeConfig{},
	}
	ctyVal, err := config.TerragruntConfigAsCty(&testConfig)
	require.NoError(t, err)

	ctyMap, err := ctyhelper.ParseCtyValueToMap(ctyVal)
	require.NoError(t, err)

	// Test the root properties
	testConfigStructInfo := structs.New(testConfig)
	testConfigFields := testConfigStructInfo.Names()
	checked := map[string]bool{} // used to track which fields of the ctyMap were seen
	for _, field := range testConfigFields {
		mapKey, isConverted := terragruntConfigStructFieldToMapKey(t, field)
		if isConverted {
			_, hasKey := ctyMap[mapKey]
			assert.Truef(t, hasKey, "Struct field %s (convert of map key %s) did not convert to cty val", field, mapKey)
			checked[mapKey] = true
		}
	}
	for key := range ctyMap {
		_, hasKey := checked[key]
		assert.Truef(t, hasKey, "cty value key %s is not accounted for from struct field", key)
	}
}

// This test makes sure that all the fields in RemoteState are converted to cty
func TestRemoteStateAsCtyDrift(t *testing.T) {
	t.Parallel()

	testConfig := remotestate.Config{
		BackendName:                   "foo",
		DisableInit:                   true,
		DisableDependencyOptimization: true,
		Generate: &remotestate.ConfigGenerate{
			Path:     "foo",
			IfExists: "overwrite_terragrunt",
		},
		BackendConfig: map[string]any{
			"bar": "baz",
		},
		Encryption: map[string]any{
			"bar": "baz",
		},
	}

	ctyVal, err := config.RemoteStateAsCty(remotestate.New(&testConfig))
	require.NoError(t, err)

	ctyMap, err := ctyhelper.ParseCtyValueToMap(ctyVal)
	require.NoError(t, err)

	// Test the root properties
	testConfigStructInfo := structs.New(testConfig)
	testConfigFields := testConfigStructInfo.Names()
	checked := map[string]bool{} // used to track which fields of the ctyMap were seen
	for _, field := range testConfigFields {
		mapKey, isConverted := remoteStateStructFieldToMapKey(t, field)
		if isConverted {
			_, hasKey := ctyMap[mapKey]
			assert.Truef(t, hasKey, "Struct field %s (convert of map key %s) did not convert to cty val", field, mapKey)
			checked[mapKey] = true
		}
	}
	for key := range ctyMap {
		_, hasKey := checked[key]
		assert.Truef(t, hasKey, "cty value key %s is not accounted for from struct field", key)
	}

}

// This test makes sure that all the fields in TerraformConfig exist in ctyTerraformConfig.
func TestTerraformConfigAsCtyDrift(t *testing.T) {
	t.Parallel()

	terraformConfigStructInfo := structs.New(config.TerraformConfig{})
	terraformConfigFields := terraformConfigStructInfo.Names()
	sort.Strings(terraformConfigFields)
	ctyTerraformConfigStructInfo := structs.New(config.CtyTerraformConfig{})
	ctyTerraformConfigFields := ctyTerraformConfigStructInfo.Names()
	sort.Strings(ctyTerraformConfigFields)
	assert.Equal(t, terraformConfigFields, ctyTerraformConfigFields)
}

func TestStackUnitCtyReading(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	options := terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath)
	ctx := config.NewParsingContext(t.Context(), l, options)
	tgConfigCty, err := config.ParseTerragruntConfig(ctx, l, "../test/fixtures/stacks/basic/live/terragrunt.stack.hcl", nil)
	require.NoError(t, err)
	stackMap, err := ctyhelper.ParseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)
	assert.NotNil(t, stackMap)
	// validate parsed unit
	unit := stackMap["unit"].(map[string]any)
	assert.NotNil(t, unit)
	assert.NotNil(t, unit["mother"])
	assert.NotNil(t, unit["father"])
	assert.NotNil(t, unit["chick_1"])
	assert.NotNil(t, unit["chick_2"])
}

func TestStackLocalsCtyReading(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	options := terragruntOptionsForTest(t, config.DefaultTerragruntConfigPath)
	ctx := config.NewParsingContext(t.Context(), l, options)
	tgConfigCty, err := config.ParseTerragruntConfig(ctx, l, "../test/fixtures/stacks/locals/live/terragrunt.stack.hcl", nil)
	require.NoError(t, err)
	stackMap, err := ctyhelper.ParseCtyValueToMap(tgConfigCty)
	require.NoError(t, err)
	assert.NotNil(t, stackMap)
	locals := stackMap["local"].(map[string]any)
	assert.NotNil(t, locals)
}

func terragruntConfigStructFieldToMapKey(t *testing.T, fieldName string) (string, bool) {
	t.Helper()

	switch fieldName {
	case "Catalog":
		return "catalog", true
	case "Terraform":
		return "terraform", true
	case "TerraformBinary":
		return "terraform_binary", true
	case "TerraformVersionConstraint":
		return "terraform_version_constraint", true
	case "TerragruntVersionConstraint":
		return "terragrunt_version_constraint", true
	case "RemoteState":
		return "remote_state", true
	case "Dependencies":
		return "dependencies", true
	case "DownloadDir":
		return "download_dir", true
	case "PreventDestroy":
		return "prevent_destroy", true
	case "Skip":
		return "skip", true
	case "IamRole":
		return "iam_role", true
	case "IamAssumeRoleDuration":
		return "iam_assume_role_duration", true
	case "IamAssumeRoleSessionName":
		return "iam_assume_role_session_name", true
	case "IamWebIdentityToken":
		return "iam_web_identity_token", true
	case "Inputs":
		return "inputs", true
	case "Locals":
		return "locals", true
	case "TerragruntDependencies":
		return "dependency", true
	case "GenerateConfigs":
		return "generate", true
	case "IsPartial":
		return "", false
	case "ProcessedIncludes":
		return "", false
	case "FieldsMetadata":
		return "", false
	case "RetryableErrors":
		return "retryable_errors", true
	case "RetryMaxAttempts":
		return "retry_max_attempts", true
	case "RetrySleepIntervalSec":
		return "retry_sleep_interval_sec", true
	case "DependentModulesPath":
		return "dependent_modules", true
	case "Engine":
		return "engine", true
	case "FeatureFlags":
		return "feature", true
	case "Exclude":
		return "exclude", true
	case "Errors":
		return "errors", true
	default:
		t.Fatalf("Unknown struct property: %s", fieldName)
		// This should not execute
		return "", false
	}
}

func remoteStateStructFieldToMapKey(t *testing.T, fieldName string) (string, bool) {
	t.Helper()

	switch fieldName {
	case "BackendName":
		return "backend", true
	case "DisableInit":
		return "disable_init", true
	case "DisableDependencyOptimization":
		return "disable_dependency_optimization", true
	case "Generate":
		return "generate", true
	case "BackendConfig":
		return "config", true
	case "Encryption":
		return "encryption", true
	default:
		t.Fatalf("Unknown struct property: %s", fieldName)
		// This should not execute
		return "", false
	}
}
