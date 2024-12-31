package configstack_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/configstack"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/terraform"
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

	stack, err := configstack.FindStackInSubfolders(context.Background(), terragruntOptions)
	require.NoError(t, err)

	var modulePaths = make([]string, 0, len(stack.Modules))

	for _, module := range stack.Modules {
		relPath := strings.Replace(module.Path, tempFolder, "", 1)
		relPath = filepath.ToSlash(util.JoinPath(relPath, config.DefaultTerragruntConfigPath))

		modulePaths = append(modulePaths, relPath)
	}

	for _, filePath := range filePaths {
		filePathFound := util.ListContainsElement(modulePaths, filePath)
		require.True(t, filePathFound, "The filePath %s was not found by Terragrunt.\n", filePath)
	}
}

func TestGetModuleRunGraphApplyOrder(t *testing.T) {
	t.Parallel()

	stack := createTestStack()
	runGraph, err := stack.GetModuleRunGraph(terraform.CommandNameApply)
	require.NoError(t, err)

	require.Equal(
		t,
		[]configstack.TerraformModules{
			{
				stack.Modules[1],
			},
			{
				stack.Modules[3],
				stack.Modules[4],
			},
			{
				stack.Modules[5],
			},
		},
		runGraph,
	)
}

func TestGetModuleRunGraphDestroyOrder(t *testing.T) {
	t.Parallel()

	stack := createTestStack()
	runGraph, err := stack.GetModuleRunGraph(terraform.CommandNameDestroy)
	require.NoError(t, err)

	require.Equal(
		t,
		[]configstack.TerraformModules{
			{
				stack.Modules[5],
			},
			{
				stack.Modules[3],
				stack.Modules[4],
			},
			{
				stack.Modules[1],
			},
		},
		runGraph,
	)

}

func createTestStack() *configstack.Stack {
	// Create the following module stack:
	// - account-baseline (excluded)
	// - vpc; depends on account-baseline
	// - lambdafunc; depends on vpc (assume already applied)
	// - mysql; depends on vpc
	// - redis; depends on vpc
	// - myapp; depends on mysql and redis
	basePath := "/stage/mystack"
	accountBaseline := &configstack.TerraformModule{
		Path:         filepath.Join(basePath, "account-baseline"),
		FlagExcluded: true,
	}
	vpc := &configstack.TerraformModule{
		Path:         filepath.Join(basePath, "vpc"),
		Dependencies: configstack.TerraformModules{accountBaseline},
	}
	lambda := &configstack.TerraformModule{
		Path:                 filepath.Join(basePath, "lambda"),
		Dependencies:         configstack.TerraformModules{vpc},
		AssumeAlreadyApplied: true,
	}
	mysql := &configstack.TerraformModule{
		Path:         filepath.Join(basePath, "mysql"),
		Dependencies: configstack.TerraformModules{vpc},
	}
	redis := &configstack.TerraformModule{
		Path:         filepath.Join(basePath, "redis"),
		Dependencies: configstack.TerraformModules{vpc},
	}
	myapp := &configstack.TerraformModule{
		Path:         filepath.Join(basePath, "myapp"),
		Dependencies: configstack.TerraformModules{mysql, redis},
	}

	stack := configstack.NewStack(&options.TerragruntOptions{WorkingDir: "/stage/mystack"})
	stack.Modules = configstack.TerraformModules{
		accountBaseline,
		vpc,
		lambda,
		mysql,
		redis,
		myapp,
	}

	return stack
}

func createTempFolder(t *testing.T) string {
	t.Helper()

	tmpFolder, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s\n", err.Error())
	}

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
	expected := configstack.TerraformModules{}
	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleA}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneJsonModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/json-module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/json-module-a/"+config.DefaultTerragruntJSONConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/json-module-a/" + config.DefaultTerragruntJSONConfigPath}
	expected := configstack.TerraformModules{moduleA}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-b/module-b-child"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixtures/modules/module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleB}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesReadConfigFromParentConfig(t *testing.T) {
	t.Parallel()

	childDir := "../test/fixtures/modules/module-m/module-m-child"
	childConfigPath := filepath.Join(childDir, config.DefaultTerragruntConfigPath)

	parentDir := "../test/fixtures/modules/module-m"
	parentCofnigPath := filepath.Join(parentDir, config.DefaultTerragruntConfigPath)

	localsConfigPaths := map[string]string{
		"env_vars":  "../test/fixtures/modules/module-m/env.hcl",
		"tier_vars": "../test/fixtures/modules/module-m/module-m-child/tier.hcl",
	}

	localsConfigs := make(map[string]interface{})

	for name, configPath := range localsConfigPaths {
		opts, err := options.NewTerragruntOptionsWithConfigPath(configPath)
		require.NoError(t, err)

		ctx := config.NewParsingContext(context.Background(), opts)
		cfg, err := config.PartialParseConfigFile(ctx, configPath, nil)
		require.NoError(t, err)

		localsConfigs[name] = map[string]interface{}{
			"dependencies":                  interface{}(nil),
			"download_dir":                  "",
			"generate":                      map[string]interface{}{},
			"iam_assume_role_duration":      interface{}(nil),
			"iam_assume_role_session_name":  "",
			"iam_role":                      "",
			"iam_web_identity_token":        "",
			"inputs":                        interface{}(nil),
			"locals":                        cfg.Locals,
			"retry_max_attempts":            interface{}(nil),
			"retry_sleep_interval_sec":      interface{}(nil),
			"retryable_errors":              interface{}(nil),
			"terraform_binary":              "",
			"terraform_version_constraint":  "",
			"terragrunt_version_constraint": "",
		}
	}

	moduleM := &configstack.TerraformModule{
		Path:         canonical(t, childDir),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixtures/modules/module-m/root.hcl")},
			},
			Locals:          localsConfigs,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
			FieldsMetadata: map[string]map[string]interface{}{
				"locals-env_vars": {
					"found_in_file": canonical(t, "../test/fixtures/modules/module-m/root.hcl"),
				},
				"locals-tier_vars": {
					"found_in_file": canonical(t, "../test/fixtures/modules/module-m/root.hcl"),
				},
			},
		},
		TerragruntOptions: cloneOptions(t, mockOptions, childConfigPath),
	}

	configPaths := []string{childConfigPath}
	childTerragruntConfig := &config.TerragruntConfig{
		ProcessedIncludes: map[string]config.IncludeConfig{
			"": {
				Path: parentCofnigPath,
			},
		},
	}
	expected := configstack.TerraformModules{moduleM}

	mockOptions, _ := options.NewTerragruntOptionsForTest("running_module_test")
	mockOptions.OriginalTerragruntConfigPath = childConfigPath

	stack := configstack.NewStack(mockOptions, configstack.WithChildTerragruntConfig(childTerragruntConfig))
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneJsonModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/json-module-b/module-b-child"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixtures/modules/json-module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/json-module-b/module-b-child/"+config.DefaultTerragruntJSONConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/json-module-b/module-b-child/" + config.DefaultTerragruntJSONConfigPath}
	expected := configstack.TerraformModules{moduleB}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneHclModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/hcl-module-b/module-b-child"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixtures/modules/hcl-module-b/root.hcl.json")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/hcl-module-b/module-b-child/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/hcl-module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleB}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleA, moduleC}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesJsonModulesWithHclDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/json-module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/json-module-c/"+config.DefaultTerragruntJSONConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/json-module-c/" + config.DefaultTerragruntJSONConfigPath}
	expected := configstack.TerraformModules{moduleA, moduleC}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesHclModulesWithJsonDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/json-module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/json-module-a/"+config.DefaultTerragruntJSONConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/hcl-module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../json-module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/hcl-module-c/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/json-module-a/" + config.DefaultTerragruntJSONConfigPath, "../test/fixtures/modules/hcl-module-c/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleA, moduleC}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixtures/modules/module-a")}

	moduleA := &configstack.TerraformModule{
		Path:              canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies:      configstack.TerraformModules{},
		TerragruntOptions: cloneOptions(t, opts, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleA.FlagExcluded = true
	expected := configstack.TerraformModules{moduleA, moduleC}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependencyAndConflictingNaming(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixtures/modules/module-a")}

	moduleA := &configstack.TerraformModule{
		Path:              canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies:      configstack.TerraformModules{},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	moduleAbba := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-abba"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-abba/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-abba/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleA.FlagExcluded = true
	expected := configstack.TerraformModules{moduleA, moduleC, moduleAbba}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependencyAndConflictingNamingAndGlob(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = globCanonical(t, "../test/fixtures/modules/module-a*")

	moduleA := &configstack.TerraformModule{
		Path:              canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies:      configstack.TerraformModules{},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	moduleAbba := &configstack.TerraformModule{
		Path:              canonical(t, "../test/fixtures/modules/module-abba"),
		Dependencies:      configstack.TerraformModules{},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-abba/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-abba/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	// construct the expected list
	moduleA.FlagExcluded = true
	moduleAbba.FlagExcluded = true
	expected := configstack.TerraformModules{moduleA, moduleC, moduleAbba}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixtures/modules/module-c")}

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:              canonical(t, "../test/fixtures/modules/module-c"),
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleC.FlagExcluded = true
	expected := configstack.TerraformModules{moduleA, moduleC}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixtures/modules/module-c")}

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleA.FlagExcluded = false
	expected := configstack.TerraformModules{moduleA, moduleC}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixtures/modules/module-a")}
	opts.ExcludeByDefault = true

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleC.FlagExcluded = true
	expected := configstack.TerraformModules{moduleA, moduleC}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithDependencyExcludeModuleWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixtures/modules/module-c"), canonical(t, "../test/fixtures/modules/module-f")}
	opts.ExcludeDirs = []string{canonical(t, "../test/fixtures/modules/module-f")}

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	moduleF := &configstack.TerraformModule{
		Path:                 canonical(t, "../test/fixtures/modules/module-f"),
		Dependencies:         configstack.TerraformModules{},
		TerragruntOptions:    cloneOptions(t, mockOptions, "../test/fixtures/modules/module-f/"+config.DefaultTerragruntConfigPath),
		AssumeAlreadyApplied: false,
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-f/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleF.FlagExcluded = true
	expected := configstack.TerraformModules{moduleA, moduleC, moduleF}

	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleB := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-b/module-b-child"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixtures/modules/module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	moduleD := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-d"),
		Dependencies: configstack.TerraformModules{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a", "../module-b/module-b-child", "../module-c"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-d/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-d/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithMixedDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleB := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/json-module-b/module-b-child"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixtures/modules/json-module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/json-module-b/module-b-child/"+config.DefaultTerragruntJSONConfigPath),
	}

	moduleC := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-c"),
		Dependencies: configstack.TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-c/"+config.DefaultTerragruntConfigPath),
	}

	moduleD := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/json-module-d"),
		Dependencies: configstack.TerraformModules{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a", "../json-module-b/module-b-child", "../module-c"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/json-module-d/"+config.DefaultTerragruntJSONConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/json-module-b/module-b-child/" + config.DefaultTerragruntJSONConfigPath, "../test/fixtures/modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/json-module-d/" + config.DefaultTerragruntJSONConfigPath}
	expected := configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependenciesWithIncludes(t *testing.T) {
	t.Parallel()

	moduleA := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-a"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-a/"+config.DefaultTerragruntConfigPath),
	}

	moduleB := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-b/module-b-child"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixtures/modules/module-b/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath),
	}

	moduleE := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-e/module-e-child"),
		Dependencies: configstack.TerraformModules{moduleA, moduleB},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../../module-a", "../../module-b/module-b-child"}},
			Terraform:    &config.TerraformConfig{Source: ptr("test")},
			IsPartial:    true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixtures/modules/module-e/root.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-e/module-e-child/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-e/module-e-child/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleA, moduleB, moduleE}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithExternalDependencies(t *testing.T) {
	t.Parallel()

	moduleF := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-f"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    cloneOptions(t, mockOptions, "../test/fixtures/modules/module-f/"+config.DefaultTerragruntConfigPath),
		AssumeAlreadyApplied: true,
	}

	moduleG := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-g"),
		Dependencies: configstack.TerraformModules{moduleF},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-f"}},
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-g/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-g/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleF, moduleG}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithNestedExternalDependencies(t *testing.T) {
	t.Parallel()

	moduleH := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-h"),
		Dependencies: configstack.TerraformModules{},
		Config: config.TerragruntConfig{
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    cloneOptions(t, mockOptions, "../test/fixtures/modules/module-h/"+config.DefaultTerragruntConfigPath),
		AssumeAlreadyApplied: true,
	}

	moduleI := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-i"),
		Dependencies: configstack.TerraformModules{moduleH},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-h"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    cloneOptions(t, mockOptions, "../test/fixtures/modules/module-i/"+config.DefaultTerragruntConfigPath),
		AssumeAlreadyApplied: true,
	}

	moduleJ := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-j"),
		Dependencies: configstack.TerraformModules{moduleI},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-i"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-j/"+config.DefaultTerragruntConfigPath),
	}

	moduleK := &configstack.TerraformModule{
		Path:         canonical(t, "../test/fixtures/modules/module-k"),
		Dependencies: configstack.TerraformModules{moduleH},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-h"}},
			Terraform:       &config.TerraformConfig{Source: ptr("fire")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: cloneOptions(t, mockOptions, "../test/fixtures/modules/module-k/"+config.DefaultTerragruntConfigPath),
	}

	configPaths := []string{"../test/fixtures/modules/module-j/" + config.DefaultTerragruntConfigPath, "../test/fixtures/modules/module-k/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{moduleH, moduleI, moduleJ, moduleK}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesInvalidPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixtures/modules/module-missing-dependency/" + config.DefaultTerragruntConfigPath}

	stack := configstack.NewStack(mockOptions)
	_, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.Error(t, actualErr)

	var processingModuleError configstack.ProcessingModuleError
	ok := errors.As(actualErr, &processingModuleError)
	require.True(t, ok)

	goError := new(goerrors.Error)

	unwrapped := processingModuleError.UnderlyingError
	if errors.As(unwrapped, &goError) {
		unwrapped = goError.Err
	}

	require.True(t, os.IsNotExist(unwrapped), "Expected a file not exists error but got %v", processingModuleError.UnderlyingError)
}

func TestResolveTerraformModuleNoTerraformConfig(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixtures/modules/module-l/" + config.DefaultTerragruntConfigPath}
	expected := configstack.TerraformModules{}

	stack := configstack.NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestBasicDependency(t *testing.T) {
	t.Parallel()

	moduleC := &configstack.TerraformModule{Path: "C", Dependencies: configstack.TerraformModules{}}
	moduleB := &configstack.TerraformModule{Path: "B", Dependencies: configstack.TerraformModules{moduleC}}
	moduleA := &configstack.TerraformModule{Path: "A", Dependencies: configstack.TerraformModules{moduleB}}

	stack := configstack.NewStack(&options.TerragruntOptions{WorkingDir: "test-stack"})
	stack.Modules = configstack.TerraformModules{moduleA, moduleB, moduleC}

	expected := map[string][]string{
		"B": {"A"},
		"C": {"B", "A"},
	}

	result := stack.ListStackDependentModules()

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}
func TestNestedDependencies(t *testing.T) {
	t.Parallel()

	moduleD := &configstack.TerraformModule{Path: "D", Dependencies: configstack.TerraformModules{}}
	moduleC := &configstack.TerraformModule{Path: "C", Dependencies: configstack.TerraformModules{moduleD}}
	moduleB := &configstack.TerraformModule{Path: "B", Dependencies: configstack.TerraformModules{moduleC}}
	moduleA := &configstack.TerraformModule{Path: "A", Dependencies: configstack.TerraformModules{moduleB}}

	// Create a mock stack
	stack := configstack.NewStack(&options.TerragruntOptions{WorkingDir: "nested-stack"})
	stack.Modules = configstack.TerraformModules{moduleA, moduleB, moduleC, moduleD}

	// Expected result
	expected := map[string][]string{
		"B": {"A"},
		"C": {"B", "A"},
		"D": {"C", "B", "A"},
	}

	// Run the function
	result := stack.ListStackDependentModules()

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}

func TestCircularDependencies(t *testing.T) {
	t.Parallel()

	// Mock modules with circular dependencies
	moduleA := &configstack.TerraformModule{Path: "A"}
	moduleB := &configstack.TerraformModule{Path: "B"}
	moduleC := &configstack.TerraformModule{Path: "C"}

	moduleA.Dependencies = configstack.TerraformModules{moduleB}
	moduleB.Dependencies = configstack.TerraformModules{moduleC}
	moduleC.Dependencies = configstack.TerraformModules{moduleA} // Circular dependency

	stack := configstack.NewStack(&options.TerragruntOptions{WorkingDir: "circular-stack"})
	stack.Modules = configstack.TerraformModules{moduleA, moduleB, moduleC}

	expected := map[string][]string{
		"A": {"C", "B"},
		"B": {"A", "C"},
		"C": {"B", "A"},
	}

	// Run the function
	result := stack.ListStackDependentModules()

	if !reflect.DeepEqual(result, expected) {
		t.Errorf("Expected %v, got %v", expected, result)
	}
}
