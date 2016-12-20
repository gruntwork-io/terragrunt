package spin

import (
	"testing"
	"io/ioutil"
	"path/filepath"
	"os"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/stretchr/testify/assert"
	"github.com/gruntwork-io/terragrunt/util"
	"strings"
)

func TestFindStackInSubfolders(t *testing.T) {
	t.Parallel()

	filePaths := []string{
		"/stage/data-stores/redis/.terragrunt",
		"/stage/data-stores/postgres/.terragrunt",
		"/stage/ecs-cluster/.terragrunt",
		"/stage/kms-master-key/.terragrunt",
		"/stage/vpc/.terragrunt",
	}

	tempFolder := createTempFolder(t)
	writeAsEmptyFiles(t, tempFolder, filePaths)

	envFolder := filepath.Join(tempFolder + "/stage")
	terragruntOptions := options.NewTerragruntOptions(envFolder)
	terragruntOptions.WorkingDir = envFolder

	stack, err := FindStackInSubfolders(terragruntOptions)
	if err != nil {
		t.Fatalf("Failed when calling method under test: %s\n", err.Error())
	}

	var modulePaths []string

	for _, module := range stack.Modules {
		relPath := strings.Replace(module.Path, tempFolder, "", 1)
		relPath = filepath.Join(relPath, ".terragrunt")

		modulePaths = append(modulePaths, relPath)
	}

	for _, filePath := range filePaths {
		filePathFound := util.ListContainsElement(modulePaths, filePath)
		assert.True(t, filePathFound, "The filePath %s was not found by Terragrunt.\n", filePath)
	}

}

func createTempFolder(t *testing.T) string {
	tmpFolder, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %s\n", err.Error())
	}

	return tmpFolder
}

// Create an empty file at each of the given paths
func writeAsEmptyFiles(t *testing.T, tmpFolder string, paths []string) {
	for _, path := range paths {
		absPath := filepath.Join(tmpFolder, path)

		containingDir := filepath.Dir(absPath)
		createDirIfNotExist(t, containingDir)

		err := ioutil.WriteFile(absPath, nil, os.ModePerm)
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
