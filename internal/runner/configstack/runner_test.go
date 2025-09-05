package configstack_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/runner/common"
	"github.com/gruntwork-io/terragrunt/internal/runner/configstack"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/tf"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/require"

	goerrors "github.com/go-errors/errors"
)

func TestFindStackInSubfolders(t *testing.T) {
	t.Parallel()

	filePaths := []string{
		"/stage/data-stores/redis/" + config.DefaultTerragruntConfigPath,
		"/stage/data-stores/postgres/" + config.DefaultTerragruntConfigPath,
		"/stage/ecs-cluster/" + config.DefaultTerragruntConfigPath,
		"/stage/kms-master-key/" + config.DefaultTerragruntConfigPath,
		"/stage/vpc/" + config.DefaultTerragruntConfigPath,
	}

	tempFolder := createTempFolder(t)
	writeDummyTerragruntConfigs(t, tempFolder, filePaths)

	envFolder := filepath.ToSlash(util.JoinPath(tempFolder + "/stage"))

	terragruntOptions, err := options.NewTerragruntOptionsWithConfigPath(envFolder)
	if err != nil {
		t.Fatalf("Failed when calling method under test: %s\n", err.Error())
	}

	terragruntOptions.WorkingDir = envFolder

	runner, err := configstack.Build(t.Context(), logger.CreateLogger(), terragruntOptions)
	require.NoError(t, err)

	stack := runner.GetStack()

	var unitsPaths = make([]string, 0, len(stack.Units))

	for _, unit := range stack.Units {
		relPath := strings.Replace(unit.Path, tempFolder, "", 1)
		relPath = filepath.ToSlash(util.JoinPath(relPath, config.DefaultTerragruntConfigPath))

		unitsPaths = append(unitsPaths, relPath)
	}

	for _, filePath := range filePaths {
		filePathFound := util.ListContainsElement(unitsPaths, filePath)
		require.True(t, filePathFound, "The filePath %s was not found by Terragrunt.\n", filePath)
	}
}

func TestGetUnitRunGraphApplyOrder(t *testing.T) {
	t.Parallel()

	stack := createTestRunner()
	runGraph, err := stack.GetUnitRunGraph(tf.CommandNameApply)
	require.NoError(t, err)

	require.Equal(
		t,
		[]common.Units{
			{
				stack.Units()[1],
			},
			{
				stack.Units()[3],
				stack.Units()[4],
			},
			{
				stack.Units()[5],
			},
		},
		runGraph,
	)
}

func TestGetUnitRunGraphDestroyOrder(t *testing.T) {
	t.Parallel()

	stack := createTestRunner()
	runGraph, err := stack.GetUnitRunGraph(tf.CommandNameDestroy)
	require.NoError(t, err)

	require.Equal(
		t,
		[]common.Units{
			{
				stack.Units()[5],
			},
			{
				stack.Units()[3],
				stack.Units()[4],
			},
			{
				stack.Units()[1],
			},
		},
		runGraph,
	)
}

func createTestRunner() *configstack.Runner {
	// Create the following module stack:
	// - account-baseline (excluded)
	// - vpc; depends on account-baseline
	// - lambdafunc; depends on vpc (assume already applied)
	// - mysql; depends on vpc
	// - redis; depends on vpc
	// - myapp; depends on mysql and redis
	l := logger.CreateLogger()

	basePath := "/stage/mystack"
	accountBaseline := &common.Unit{
		Path:         filepath.Join(basePath, "account-baseline"),
		FlagExcluded: true,
		Logger:       l,
	}
	vpc := &common.Unit{
		Path:         filepath.Join(basePath, "vpc"),
		Dependencies: common.Units{accountBaseline},
		Logger:       l,
	}
	lambda := &common.Unit{
		Path:                 filepath.Join(basePath, "lambda"),
		Dependencies:         common.Units{vpc},
		AssumeAlreadyApplied: true,
		Logger:               l,
	}
	mysql := &common.Unit{
		Path:         filepath.Join(basePath, "mysql"),
		Dependencies: common.Units{vpc},
		Logger:       l,
	}
	redis := &common.Unit{
		Path:         filepath.Join(basePath, "redis"),
		Dependencies: common.Units{vpc},
		Logger:       l,
	}
	myapp := &common.Unit{
		Path:         filepath.Join(basePath, "myapp"),
		Dependencies: common.Units{mysql, redis},
		Logger:       l,
	}

	runner := configstack.NewRunner(l, mockOptions)
	runner.Stack.Units = common.Units{
		accountBaseline,
		vpc,
		lambda,
		mysql,
		redis,
		myapp,
	}

	return runner
}

func createTempFolder(t *testing.T) string {
	t.Helper()

	tmpFolder := t.TempDir()

	return filepath.ToSlash(tmpFolder)
}

// Create a dummy Terragrunt config file at each of the given paths
func writeDummyTerragruntConfigs(t *testing.T, tmpFolder string, paths []string) {
	t.Helper()

	contents := []byte("terraform {\nsource = \"test\"\n}\n")

	for _, path := range paths {
		absPath := util.JoinPath(tmpFolder, path)

		containingDir := filepath.Dir(absPath)
		createDirIfNotExist(t, containingDir)

		err := os.WriteFile(absPath, contents, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to write file at path %s: %s\n", path, err.Error())
		}
	}
}

func createDirIfNotExist(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		err = os.MkdirAll(path, os.ModePerm)
		if err != nil {
			t.Fatalf("Failed to create directory: %s\n", err.Error())
		}
	}
}

func TestResolveTerraformModulesNoPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{}
	expected := common.Units{}
	stack := configstack.NewRunner(logger.CreateLogger(), mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), logger.CreateLogger(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitA}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneJsonModuleNoDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/json-module-a/"+config.DefaultTerragruntJSONConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/json-module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/json-module-a/" + config.DefaultTerragruntJSONConfigPath}
	expected := common.Units{unitA}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)
	unitB := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-b/module-b-child"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../../../test/fixtures/modules/module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitB}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesReadConfigFromParentConfig(t *testing.T) {
	t.Parallel()

	childDir := "../../../test/fixtures/modules/module-m/module-m-child"
	childConfigPath := filepath.Join(childDir, config.DefaultTerragruntConfigPath)

	parentDir := "../../../test/fixtures/modules/module-m"
	parentCofnigPath := filepath.Join(parentDir, config.DefaultTerragruntConfigPath)

	localsConfigPaths := map[string]string{
		"env_vars":  "../../../test/fixtures/modules/module-m/env.hcl",
		"tier_vars": "../../../test/fixtures/modules/module-m/module-m-child/tier.hcl",
	}

	localsConfigs := make(map[string]any)

	for name, configPath := range localsConfigPaths {
		opts, err := options.NewTerragruntOptionsWithConfigPath(configPath)
		require.NoError(t, err)

		l := logger.CreateLogger()

		ctx := config.NewParsingContext(t.Context(), l, opts)
		cfg, err := config.PartialParseConfigFile(ctx, l, configPath, nil)
		require.NoError(t, err)

		localsConfigs[name] = map[string]any{
			"dependencies":                  any(nil),
			"download_dir":                  "",
			"generate":                      map[string]any{},
			"iam_assume_role_duration":      any(nil),
			"iam_assume_role_session_name":  "",
			"iam_role":                      "",
			"iam_web_identity_token":        "",
			"inputs":                        any(nil),
			"locals":                        cfg.Locals,
			"retry_max_attempts":            any(nil),
			"retry_sleep_interval_sec":      any(nil),
			"retryable_errors":              any(nil),
			"terraform_binary":              "",
			"terraform_version_constraint":  "",
			"terragrunt_version_constraint": "",
		}
	}

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, childConfigPath)
	unitM := &common.Unit{
		Path:         canonical(t, childDir),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../../../test/fixtures/modules/module-m/root.hcl")},
			},
			Locals:          localsConfigs,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
			FieldsMetadata: map[string]map[string]any{
				"locals-env_vars": {
					"found_in_file": canonical(t, "../../../test/fixtures/modules/module-m/root.hcl"),
				},
				"locals-tier_vars": {
					"found_in_file": canonical(t, "../../../test/fixtures/modules/module-m/root.hcl"),
				},
			},
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{childConfigPath}
	childTerragruntConfig := &config.TerragruntConfig{
		ProcessedIncludes: map[string]config.IncludeConfig{
			"": {
				Path: parentCofnigPath,
			},
		},
	}
	expected := common.Units{unitM}

	mockOptions, _ := options.NewTerragruntOptionsForTest("running_module_test")
	mockOptions.OriginalTerragruntConfigPath = childConfigPath

	stack := configstack.NewRunner(l, mockOptions, common.WithChildTerragruntConfig(childTerragruntConfig))
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneJsonModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/json-module-b/module-b-child/"+config.DefaultTerragruntJSONConfigPath)
	unitB := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/json-module-b/module-b-child"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../../../test/fixtures/modules/json-module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/json-module-b/module-b-child/" + config.DefaultTerragruntJSONConfigPath}
	expected := common.Units{unitB}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneHclModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/hcl-module-b/module-b-child/"+config.DefaultTerragruntConfigPath)
	unitB := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/hcl-module-b/module-b-child"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../../../test/fixtures/modules/hcl-module-b/root.hcl.json")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/hcl-module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitB}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)
	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitA, unitC}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesJsonModulesWithHclDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/json-module-c/"+config.DefaultTerragruntJSONConfigPath)
	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/json-module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/json-module-c/" + config.DefaultTerragruntJSONConfigPath}
	expected := common.Units{unitA, unitC}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesHclModulesWithJsonDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/json-module-a/"+config.DefaultTerragruntJSONConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/json-module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/hcl-module-c/"+config.DefaultTerragruntConfigPath)
	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/hcl-module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../json-module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/json-module-a/" + config.DefaultTerragruntJSONConfigPath, "../../../test/fixtures/modules/hcl-module-c/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitA, unitC}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../../../test/fixtures/modules/module-a")}

	l := logger.CreateLogger()

	lA, optsA := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
	unitA := &common.Unit{
		Path:              canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies:      common.Units{},
		TerragruntOptions: optsA,
		Logger:            lA,
	}

	lC, optsC := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)
	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: optsC,
		Logger:            lC,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewRunner(l, opts)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)

	// construct the expected list
	unitA.FlagExcluded = true
	expected := common.Units{unitA, unitC}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependencyAndConflictingNaming(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../../../test/fixtures/modules/module-a")}

	l := logger.CreateLogger()

	lA, optsA := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)

	unitA := &common.Unit{
		Path:              canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies:      common.Units{},
		TerragruntOptions: optsA,
		Logger:            lA,
	}

	lC, optsC := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)

	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: optsC,
		Logger:            lC,
	}

	lAbba, optsAbba := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-abba/"+config.DefaultTerragruntConfigPath)

	unitAbba := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-abba"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: optsAbba,
		Logger:            lAbba,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-abba/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewRunner(l, opts)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)

	// construct the expected list
	unitA.FlagExcluded = true
	expected := common.Units{unitA, unitC, unitAbba}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependencyAndConflictingNamingAndGlob(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name             string
		strictDoubleStar bool
	}{
		{name: "strict control double-star disabled", strictDoubleStar: false},
		{name: "strict control double-star enabled", strictDoubleStar: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			opts, err := options.NewTerragruntOptionsForTest("running_module_test")
			require.NoError(t, err)

			if opts.StrictControls.FilterByNames("double-star").Evaluate(t.Context()) != nil && !tt.strictDoubleStar {
				t.Skip("Skipping test because double-star is already enabled by default")
			}

			opts.ExcludeDirs = []string{"../../../test/fixtures/modules/module-a*"}
			if tt.strictDoubleStar {
				opts.StrictControls.FilterByNames("double-star").Enable()
			} else {
				var err error

				opts.ExcludeDirs, err = util.GlobCanonicalPath(log.Default(), "", opts.ExcludeDirs...)
				require.NoError(t, err)
			}

			l := logger.CreateLogger()

			lA, optsA := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
			unitA := &common.Unit{
				Path:              canonical(t, "../../../test/fixtures/modules/module-a"),
				Dependencies:      common.Units{},
				TerragruntOptions: optsA,
				Logger:            lA,
			}

			lC, optsC := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)
			unitC := &common.Unit{
				Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
				Dependencies: common.Units{unitA},
				Config: config.TerragruntConfig{
					Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
					Terraform:       &config.TerraformConfig{Source: ptr("temp")},
					IsPartial:       true,
					GenerateConfigs: make(map[string]codegen.GenerateConfig),
				},
				TerragruntOptions: optsC,
				Logger:            lC,
			}

			lAbba, optsAbba := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-abba/"+config.DefaultTerragruntConfigPath)
			unitAbba := &common.Unit{
				Path:              canonical(t, "../../../test/fixtures/modules/module-abba"),
				Dependencies:      common.Units{},
				TerragruntOptions: optsAbba,
				Logger:            lAbba,
			}

			configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-abba/" + config.DefaultTerragruntConfigPath}

			stack := configstack.NewRunner(l, opts)
			actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
			// construct the expected list
			unitA.FlagExcluded = true
			unitAbba.FlagExcluded = true
			expected := common.Units{unitA, unitC, unitAbba}

			require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
			assertUnitListsEqual(t, expected, actualModules)
		})
	}
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../../../test/fixtures/modules/module-c")}

	l := logger.CreateLogger()

	lA, optsA := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: optsA,
		Logger:            lA,
	}

	lC, optsC := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)
	unitC := &common.Unit{
		Path:              canonical(t, "../../../test/fixtures/modules/module-c"),
		TerragruntOptions: optsC,
		Logger:            lC,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewRunner(l, opts)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)

	// construct the expected list
	unitC.FlagExcluded = true
	expected := common.Units{unitA, unitC}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../../../test/fixtures/modules/module-c")}

	l := logger.CreateLogger()
	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)

	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)

	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewRunner(l, opts)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)

	// construct the expected list
	unitA.FlagExcluded = false
	expected := common.Units{unitA, unitC}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../../../test/fixtures/modules/module-a")}
	opts.ExcludeByDefault = true

	l := logger.CreateLogger()

	lA, optsA := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)

	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: optsA,
		Logger:            lA,
	}

	lC, optsC := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)

	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: optsC,
		Logger:            lC,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewRunner(l, opts)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)

	// construct the expected list
	unitC.FlagExcluded = true
	expected := common.Units{unitA, unitC}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithDependencyExcludeModuleWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../../../test/fixtures/modules/module-c"), canonical(t, "../../../test/fixtures/modules/module-f")}
	opts.ExcludeDirs = []string{canonical(t, "../../../test/fixtures/modules/module-f")}

	l := logger.CreateLogger()

	lA, optsA := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: optsA,
		Logger:            lA,
	}

	lC, optsC := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)
	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: optsC,
		Logger:            lC,
	}

	lF, optsF := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-f/"+config.DefaultTerragruntConfigPath)
	unitF := &common.Unit{
		Path:                 canonical(t, "../../../test/fixtures/modules/module-f"),
		Dependencies:         common.Units{},
		TerragruntOptions:    optsF,
		Logger:               lF,
		AssumeAlreadyApplied: false,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-f/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewRunner(l, opts)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)

	// construct the expected list
	unitF.FlagExcluded = true
	expected := common.Units{unitA, unitC, unitF}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()
	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)

	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)
	unitB := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-b/module-b-child"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../../../test/fixtures/modules/module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)
	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-d/"+config.DefaultTerragruntConfigPath)
	unitD := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-d"),
		Dependencies: common.Units{unitA, unitB, unitC},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a", "../module-b/module-b-child", "../module-c"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-d/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitA, unitB, unitC, unitD}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithMixedDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/json-module-b/module-b-child/"+config.DefaultTerragruntJSONConfigPath)
	unitB := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/json-module-b/module-b-child"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../../../test/fixtures/modules/json-module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath)
	unitC := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-c"),
		Dependencies: common.Units{unitA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/json-module-d/"+config.DefaultTerragruntJSONConfigPath)
	unitD := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/json-module-d"),
		Dependencies: common.Units{unitA, unitB, unitC},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a", "../json-module-b/module-b-child", "../module-c"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/json-module-b/module-b-child/" + config.DefaultTerragruntJSONConfigPath, "../../../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/json-module-d/" + config.DefaultTerragruntJSONConfigPath}
	expected := common.Units{unitA, unitB, unitC, unitD}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependenciesWithIncludes(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath)
	unitA := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-a"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)
	unitB := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-b/module-b-child"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../../../test/fixtures/modules/module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-e/module-e-child/"+config.DefaultTerragruntConfigPath)
	unitE := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-e/module-e-child"),
		Dependencies: common.Units{unitA, unitB},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../../module-a", "../../module-b/module-b-child"}},
			Terraform:    &config.TerraformConfig{Source: ptr("test")},
			IsPartial:    true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../../../test/fixtures/modules/module-e/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-e/module-e-child/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitA, unitB, unitE}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithExternalDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-f/"+config.DefaultTerragruntConfigPath)
	unitF := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-f"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    opts,
		Logger:               l,
		AssumeAlreadyApplied: true,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-g/"+config.DefaultTerragruntConfigPath)
	unitG := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-g"),
		Dependencies: common.Units{unitF},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-f"}},
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-g/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitF, unitG}

	stack := configstack.NewRunner(l, mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), l, configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithNestedExternalDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	l, opts := cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-h/"+config.DefaultTerragruntConfigPath)
	unitH := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-h"),
		Dependencies: common.Units{},
		Config: config.TerragruntConfig{
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    opts,
		Logger:               l,
		AssumeAlreadyApplied: true,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-i/"+config.DefaultTerragruntConfigPath)
	unitI := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-i"),
		Dependencies: common.Units{unitH},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-h"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    opts,
		Logger:               l,
		AssumeAlreadyApplied: true,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-j/"+config.DefaultTerragruntConfigPath)
	unitJ := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-j"),
		Dependencies: common.Units{unitI},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-i"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	l, opts = cloneOptions(t, l, mockOptions, "../../../test/fixtures/modules/module-k/"+config.DefaultTerragruntConfigPath)
	unitK := &common.Unit{
		Path:         canonical(t, "../../../test/fixtures/modules/module-k"),
		Dependencies: common.Units{unitH},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-h"}},
			Terraform:       &config.TerraformConfig{Source: ptr("fire")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts,
		Logger:            l,
	}

	configPaths := []string{"../../../test/fixtures/modules/module-j/" + config.DefaultTerragruntConfigPath, "../../../test/fixtures/modules/module-k/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{unitH, unitI, unitJ, unitK}

	stack := configstack.NewRunner(logger.CreateLogger(), mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), logger.CreateLogger(), configPaths)
	require.NoError(t, actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesInvalidPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../../../test/fixtures/modules/module-missing-dependency/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewRunner(logger.CreateLogger(), mockOptions)
	_, actualErr := stack.ResolveTerraformModules(t.Context(), logger.CreateLogger(), configPaths)
	require.Error(t, actualErr)

	var processingUnitError common.ProcessingUnitError

	ok := errors.As(actualErr, &processingUnitError)
	require.True(t, ok)

	goError := new(goerrors.Error)

	unwrapped := processingUnitError.UnderlyingError
	if errors.As(unwrapped, &goError) {
		unwrapped = goError.Err
	}

	require.True(t, os.IsNotExist(unwrapped), "Expected a file not exists error but got %v", processingUnitError.UnderlyingError)
}

func TestResolveTerraformModuleNoTerraformConfig(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../../../test/fixtures/modules/module-l/" + config.DefaultTerragruntConfigPath}
	expected := common.Units{}

	stack := configstack.NewRunner(logger.CreateLogger(), mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(t.Context(), logger.CreateLogger(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertUnitListsEqual(t, expected, actualModules)
}

func TestBasicDependency(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	unitC := &common.Unit{Path: "C", Dependencies: common.Units{}, Logger: l}
	unitB := &common.Unit{Path: "B", Dependencies: common.Units{unitC}, Logger: l}
	unitA := &common.Unit{Path: "A", Dependencies: common.Units{unitB}, Logger: l}

	stack := configstack.NewRunner(l, mockOptions)
	stack.Stack.Units = common.Units{unitA, unitB, unitC}

	expected := map[string][]string{
		"B": {"A"},
		"C": {"B", "A"},
	}

	result := stack.ListStackDependentUnits()

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestNestedDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	unitD := &common.Unit{Path: "D", Dependencies: common.Units{}, Logger: l}
	unitC := &common.Unit{Path: "C", Dependencies: common.Units{unitD}, Logger: l}
	unitB := &common.Unit{Path: "B", Dependencies: common.Units{unitC}, Logger: l}
	unitA := &common.Unit{Path: "A", Dependencies: common.Units{unitB}, Logger: l}

	// Create a mock stack
	stack := configstack.NewRunner(l, mockOptions)
	stack.Stack.Units = common.Units{unitA, unitB, unitC, unitD}

	// Expected result
	expected := map[string][]string{
		"B": {"A"},
		"C": {"B", "A"},
		"D": {"C", "B", "A"},
	}

	// Run the function
	result := stack.ListStackDependentUnits()

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestCircularDependencies(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	// Mock modules with circular dependencies
	unitA := &common.Unit{Path: "A", Logger: l}
	unitB := &common.Unit{Path: "B", Logger: l}
	unitC := &common.Unit{Path: "C", Logger: l}

	unitA.Dependencies = common.Units{unitB}
	unitB.Dependencies = common.Units{unitC}
	unitC.Dependencies = common.Units{unitA} // Circular dependency

	stack := configstack.NewRunner(l, mockOptions)
	stack.Stack.Units = common.Units{unitA, unitB, unitC}

	expected := map[string][]string{
		"A": {"C", "B"},
		"B": {"A", "C"},
		"C": {"B", "A"},
	}

	// Run the function
	result := stack.ListStackDependentUnits()

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}
