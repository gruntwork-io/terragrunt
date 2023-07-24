package configstack

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/options"
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
	terragruntOptions, err := options.NewTerragruntOptions(envFolder)
	if err != nil {
		t.Fatalf("Failed when calling method under test: %s\n", err.Error())
	}

	terragruntOptions.WorkingDir = envFolder

	stack, err := FindStackInSubfolders(terragruntOptions, nil)
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
	runGraph, err := stack.getModuleRunGraph("apply")
	require.NoError(t, err)

	assert.Equal(
		t,
		[][]*TerraformModule{
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
	runGraph, err := stack.getModuleRunGraph("destroy")
	require.NoError(t, err)

	assert.Equal(
		t,
		[][]*TerraformModule{
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
		Dependencies: []*TerraformModule{accountBaseline},
	}
	lambda := &TerraformModule{
		Path:                 filepath.Join(basePath, "lambda"),
		Dependencies:         []*TerraformModule{vpc},
		AssumeAlreadyApplied: true,
	}
	mysql := &TerraformModule{
		Path:         filepath.Join(basePath, "mysql"),
		Dependencies: []*TerraformModule{vpc},
	}
	redis := &TerraformModule{
		Path:         filepath.Join(basePath, "redis"),
		Dependencies: []*TerraformModule{vpc},
	}
	myapp := &TerraformModule{
		Path:         filepath.Join(basePath, "myapp"),
		Dependencies: []*TerraformModule{mysql, redis},
	}
	return &Stack{
		Path: "/stage/mystack",
		Modules: []*TerraformModule{
			accountBaseline,
			vpc,
			lambda,
			mysql,
			redis,
			myapp,
		},
	}
}

func createTempFolder(t *testing.T) string {
	tmpFolder, err := ioutil.TempDir("", "")
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

		err := ioutil.WriteFile(absPath, contents, os.ModePerm)
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
