package configstack

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/codegen"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	stack, err := FindStackInSubfolders(context.Background(), terragruntOptions)
	require.NoError(t, err)

	var modulePaths []string

	for _, module := range stack.Modules {
		relPath := strings.Replace(module.Path, tempFolder, "", 1)
		relPath = filepath.ToSlash(util.JoinPath(relPath, config.DefaultTerragruntConfigPath))

		modulePaths = append(modulePaths, relPath)
	}

	for _, filePath := range filePaths {
		filePathFound := util.ListContainsElement(modulePaths, filePath)
		assert.True(t, filePathFound, "The filePath %s was not found by Terragrunt.\n", filePath)
	}
}

func TestGetModuleRunGraphApplyOrder(t *testing.T) {
	t.Parallel()

	stack := createTestStack()
	runGraph, err := stack.getModuleRunGraph(terraform.CommandNameApply)
	require.NoError(t, err)

	assert.Equal(
		t,
		[]TerraformModules{
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
	runGraph, err := stack.getModuleRunGraph(terraform.CommandNameDestroy)
	require.NoError(t, err)

	assert.Equal(
		t,
		[]TerraformModules{
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

func createTestStack() *Stack {
	// Create the following module stack:
	// - account-baseline (excluded)
	// - vpc; depends on account-baseline
	// - lambdafunc; depends on vpc (assume already applied)
	// - mysql; depends on vpc
	// - redis; depends on vpc
	// - myapp; depends on mysql and redis
	basePath := "/stage/mystack"
	accountBaseline := &TerraformModule{
		Path:         filepath.Join(basePath, "account-baseline"),
		FlagExcluded: true,
	}
	vpc := &TerraformModule{
		Path:         filepath.Join(basePath, "vpc"),
		Dependencies: TerraformModules{accountBaseline},
	}
	lambda := &TerraformModule{
		Path:                 filepath.Join(basePath, "lambda"),
		Dependencies:         TerraformModules{vpc},
		AssumeAlreadyApplied: true,
	}
	mysql := &TerraformModule{
		Path:         filepath.Join(basePath, "mysql"),
		Dependencies: TerraformModules{vpc},
	}
	redis := &TerraformModule{
		Path:         filepath.Join(basePath, "redis"),
		Dependencies: TerraformModules{vpc},
	}
	myapp := &TerraformModule{
		Path:         filepath.Join(basePath, "myapp"),
		Dependencies: TerraformModules{mysql, redis},
	}

	stack := NewStack(&options.TerragruntOptions{WorkingDir: "/stage/mystack"})
	stack.Modules = TerraformModules{
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
	tmpFolder, err := os.MkdirTemp("", "")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s\n", err.Error())
	}

	return filepath.ToSlash(tmpFolder)
}

// Create a dummy Terragrunt config file at each of the given paths
func writeDummyTerragruntConfigs(t *testing.T, tmpFolder string, paths []string) {
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
	expected := TerraformModules{}
	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleA}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneJsonModuleNoDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-a/"+config.DefaultTerragruntJsonConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/json-module-a/" + config.DefaultTerragruntJsonConfigPath}
	expected := TerraformModules{moduleA}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-b/terragrunt.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleB}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesReadConfigFromParentConfig(t *testing.T) {
	t.Parallel()

	childDir := "../test/fixture-modules/module-m/module-m-child"
	childConfigPath := filepath.Join(childDir, config.DefaultTerragruntConfigPath)

	parentDir := "../test/fixture-modules/module-m"
	parentCofnigPath := filepath.Join(parentDir, config.DefaultTerragruntConfigPath)

	localsConfigPaths := map[string]string{
		"env_vars":  "../test/fixture-modules/module-m/env.hcl",
		"tier_vars": "../test/fixture-modules/module-m/module-m-child/tier.hcl",
	}

	localsConfigs := make(map[string]interface{})

	for name, configPath := range localsConfigPaths {
		opts, err := options.NewTerragruntOptionsWithConfigPath(configPath)
		assert.NoError(t, err)

		ctx := config.NewParsingContext(context.Background(), opts)
		cfg, err := config.PartialParseConfigFile(ctx, configPath, nil)
		assert.NoError(t, err)

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
			"skip":                          false,
			"terraform_binary":              "",
			"terraform_version_constraint":  "",
			"terragrunt_version_constraint": "",
		}
	}

	moduleM := &TerraformModule{
		Path:         canonical(t, childDir),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-m/terragrunt.hcl")},
			},
			Locals:          localsConfigs,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
			FieldsMetadata: map[string]map[string]interface{}{
				"locals-env_vars": {
					"found_in_file": canonical(t, "../test/fixture-modules/module-m/terragrunt.hcl"),
				},
				"locals-tier_vars": {
					"found_in_file": canonical(t, "../test/fixture-modules/module-m/terragrunt.hcl"),
				},
			},
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, childConfigPath)),
	}

	configPaths := []string{childConfigPath}
	childTerragruntConfig := &config.TerragruntConfig{
		ProcessedIncludes: map[string]config.IncludeConfig{
			"": {
				Path: parentCofnigPath,
			},
		},
	}
	expected := TerraformModules{moduleM}

	mockOptions, _ := options.NewTerragruntOptionsForTest("running_module_test")
	mockOptions.OriginalTerragruntConfigPath = childConfigPath

	stack := NewStack(mockOptions, WithChildTerragruntConfig(childTerragruntConfig))
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneJsonModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-b/module-b-child"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/json-module-b/terragrunt.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-b/module-b-child/"+config.DefaultTerragruntJsonConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/json-module-b/module-b-child/" + config.DefaultTerragruntJsonConfigPath}
	expected := TerraformModules{moduleB}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesOneHclModuleWithIncludesNoDependencies(t *testing.T) {
	t.Parallel()

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/hcl-module-b/module-b-child"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/hcl-module-b/terragrunt.hcl.json")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/hcl-module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/hcl-module-b/module-b-child/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleB}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleA, moduleC}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesJsonModulesWithHclDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-c/"+config.DefaultTerragruntJsonConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/json-module-c/" + config.DefaultTerragruntJsonConfigPath}
	expected := TerraformModules{moduleA, moduleC}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesHclModulesWithJsonDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-a/"+config.DefaultTerragruntJsonConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/hcl-module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../json-module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/hcl-module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/json-module-a/" + config.DefaultTerragruntJsonConfigPath, "../test/fixture-modules/hcl-module-c/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleA, moduleC}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-a")}

	moduleA := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-a"),
		Dependencies:      TerraformModules{},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleA.FlagExcluded = true
	expected := TerraformModules{moduleA, moduleC}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependencyAndConflictingNaming(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-a")}

	moduleA := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-a"),
		Dependencies:      TerraformModules{},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleAbba := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-abba"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-abba/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-abba/" + config.DefaultTerragruntConfigPath}

	stack := NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleA.FlagExcluded = true
	expected := TerraformModules{moduleA, moduleC, moduleAbba}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithDependencyAndConflictingNamingAndGlob(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = globCanonical(t, "../test/fixture-modules/module-a*")

	moduleA := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-a"),
		Dependencies:      TerraformModules{},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleAbba := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-abba"),
		Dependencies:      TerraformModules{},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-abba/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-abba/" + config.DefaultTerragruntConfigPath}

	stack := NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	// construct the expected list
	moduleA.FlagExcluded = true
	moduleAbba.FlagExcluded = true
	expected := TerraformModules{moduleA, moduleC, moduleAbba}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesExcludedDirsWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-c")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:              canonical(t, "../test/fixture-modules/module-c"),
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleC.FlagExcluded = true
	expected := TerraformModules{moduleA, moduleC}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixture-modules/module-c")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleA.FlagExcluded = false
	expected := TerraformModules{moduleA, moduleC}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixture-modules/module-a")}
	opts.ExcludeByDefault = true

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath}

	stack := NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleC.FlagExcluded = true
	expected := TerraformModules{moduleA, moduleC}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesTwoModulesWithDependenciesIncludedDirsWithDependencyExcludeModuleWithNoDependency(t *testing.T) {
	t.Parallel()

	opts, _ := options.NewTerragruntOptionsForTest("running_module_test")
	opts.IncludeDirs = []string{canonical(t, "../test/fixture-modules/module-c"), canonical(t, "../test/fixture-modules/module-f")}
	opts.ExcludeDirs = []string{canonical(t, "../test/fixture-modules/module-f")}

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: opts.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleF := &TerraformModule{
		Path:                 canonical(t, "../test/fixture-modules/module-f"),
		Dependencies:         TerraformModules{},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-f/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: false,
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-f/" + config.DefaultTerragruntConfigPath}

	stack := NewStack(opts)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)

	// construct the expected list
	moduleF.FlagExcluded = true
	expected := TerraformModules{moduleA, moduleC, moduleF}

	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-b/terragrunt.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleD := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-d"),
		Dependencies: TerraformModules{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a", "../module-b/module-b-child", "../module-c"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-d/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-d/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleA, moduleB, moduleC, moduleD}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithMixedDependencies(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-b/module-b-child"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/json-module-b/terragrunt.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-b/module-b-child/"+config.DefaultTerragruntJsonConfigPath)),
	}

	moduleC := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-c"),
		Dependencies: TerraformModules{moduleA},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-c/"+config.DefaultTerragruntConfigPath)),
	}

	moduleD := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/json-module-d"),
		Dependencies: TerraformModules{moduleA, moduleB, moduleC},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-a", "../json-module-b/module-b-child", "../module-c"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/json-module-d/"+config.DefaultTerragruntJsonConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/json-module-b/module-b-child/" + config.DefaultTerragruntJsonConfigPath, "../test/fixture-modules/module-c/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/json-module-d/" + config.DefaultTerragruntJsonConfigPath}
	expected := TerraformModules{moduleA, moduleB, moduleC, moduleD}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithDependenciesWithIncludes(t *testing.T) {
	t.Parallel()

	moduleA := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-a"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-a/"+config.DefaultTerragruntConfigPath)),
	}

	moduleB := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-b/module-b-child"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			Terraform: &config.TerraformConfig{Source: ptr("...")},
			IsPartial: true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-b/terragrunt.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-b/module-b-child/"+config.DefaultTerragruntConfigPath)),
	}

	moduleE := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-e/module-e-child"),
		Dependencies: TerraformModules{moduleA, moduleB},
		Config: config.TerragruntConfig{
			Dependencies: &config.ModuleDependencies{Paths: []string{"../../module-a", "../../module-b/module-b-child"}},
			Terraform:    &config.TerraformConfig{Source: ptr("test")},
			IsPartial:    true,
			ProcessedIncludes: map[string]config.IncludeConfig{
				"": {Path: canonical(t, "../test/fixture-modules/module-e/terragrunt.hcl")},
			},
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-e/module-e-child/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-a/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-b/module-b-child/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-e/module-e-child/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleA, moduleB, moduleE}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithExternalDependencies(t *testing.T) {
	t.Parallel()

	moduleF := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-f"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-f/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: true,
	}

	moduleG := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-g"),
		Dependencies: TerraformModules{moduleF},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-f"}},
			Terraform:       &config.TerraformConfig{Source: ptr("test")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-g/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-g/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleF, moduleG}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesMultipleModulesWithNestedExternalDependencies(t *testing.T) {
	t.Parallel()

	moduleH := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-h"),
		Dependencies: TerraformModules{},
		Config: config.TerragruntConfig{
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-h/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: true,
	}

	moduleI := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-i"),
		Dependencies: TerraformModules{moduleH},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-h"}},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions:    mockOptions.Clone(canonical(t, "../test/fixture-modules/module-i/"+config.DefaultTerragruntConfigPath)),
		AssumeAlreadyApplied: true,
	}

	moduleJ := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-j"),
		Dependencies: TerraformModules{moduleI},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-i"}},
			Terraform:       &config.TerraformConfig{Source: ptr("temp")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-j/"+config.DefaultTerragruntConfigPath)),
	}

	moduleK := &TerraformModule{
		Path:         canonical(t, "../test/fixture-modules/module-k"),
		Dependencies: TerraformModules{moduleH},
		Config: config.TerragruntConfig{
			Dependencies:    &config.ModuleDependencies{Paths: []string{"../module-h"}},
			Terraform:       &config.TerraformConfig{Source: ptr("fire")},
			IsPartial:       true,
			GenerateConfigs: make(map[string]codegen.GenerateConfig),
		},
		TerragruntOptions: mockOptions.Clone(canonical(t, "../test/fixture-modules/module-k/"+config.DefaultTerragruntConfigPath)),
	}

	configPaths := []string{"../test/fixture-modules/module-j/" + config.DefaultTerragruntConfigPath, "../test/fixture-modules/module-k/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{moduleH, moduleI, moduleJ, moduleK}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.NoError(t, actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestResolveTerraformModulesInvalidPaths(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixture-modules/module-missing-dependency/" + config.DefaultTerragruntConfigPath}

	stack := NewStack(mockOptions)
	_, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	require.Error(t, actualErr)

	underlying, ok := errors.Unwrap(actualErr).(ProcessingModuleError)
	require.True(t, ok)

	unwrapped := errors.Unwrap(underlying.UnderlyingError)
	assert.True(t, os.IsNotExist(unwrapped), "Expected a file not exists error but got %v", underlying.UnderlyingError)
}

func TestResolveTerraformModuleNoTerraformConfig(t *testing.T) {
	t.Parallel()

	configPaths := []string{"../test/fixture-modules/module-l/" + config.DefaultTerragruntConfigPath}
	expected := TerraformModules{}

	stack := NewStack(mockOptions)
	actualModules, actualErr := stack.ResolveTerraformModules(context.Background(), configPaths)
	assert.Nil(t, actualErr, "Unexpected error: %v", actualErr)
	assertModuleListsEqual(t, expected, actualModules)
}

func TestBasicDependency(t *testing.T) {
	moduleC := &TerraformModule{Path: "C", Dependencies: TerraformModules{}}
	moduleB := &TerraformModule{Path: "B", Dependencies: TerraformModules{moduleC}}
	moduleA := &TerraformModule{Path: "A", Dependencies: TerraformModules{moduleB}}

	stack := NewStack(&options.TerragruntOptions{WorkingDir: "test-stack"})
	stack.Modules = TerraformModules{moduleA, moduleB, moduleC}

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
	moduleD := &TerraformModule{Path: "D", Dependencies: TerraformModules{}}
	moduleC := &TerraformModule{Path: "C", Dependencies: TerraformModules{moduleD}}
	moduleB := &TerraformModule{Path: "B", Dependencies: TerraformModules{moduleC}}
	moduleA := &TerraformModule{Path: "A", Dependencies: TerraformModules{moduleB}}

	// Create a mock stack
	stack := NewStack(&options.TerragruntOptions{WorkingDir: "nested-stack"})
	stack.Modules = TerraformModules{moduleA, moduleB, moduleC, moduleD}

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
	// Mock modules with circular dependencies
	moduleA := &TerraformModule{Path: "A"}
	moduleB := &TerraformModule{Path: "B"}
	moduleC := &TerraformModule{Path: "C"}

	moduleA.Dependencies = TerraformModules{moduleB}
	moduleB.Dependencies = TerraformModules{moduleC}
	moduleC.Dependencies = TerraformModules{moduleA} // Circular dependency

	stack := NewStack(&options.TerragruntOptions{WorkingDir: "circular-stack"})
	stack.Modules = TerraformModules{moduleA, moduleB, moduleC}

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
