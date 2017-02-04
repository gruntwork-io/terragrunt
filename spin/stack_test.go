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
		"/stage/data-stores/redis/terraform.tfvars",
		"/stage/data-stores/postgres/terraform.tfvars",
		"/stage/ecs-cluster/terraform.tfvars",
		"/stage/kms-master-key/terraform.tfvars",
		"/stage/vpc/terraform.tfvars",
	}

	tempFolder := createTempFolder(t)
	writeAsEmptyFiles(t, tempFolder, filePaths)

	envFolder := filepath.ToSlash(util.JoinPath(tempFolder + "/stage"))
	terragruntOptions := options.NewTerragruntOptions(envFolder)
	terragruntOptions.WorkingDir = envFolder

	stack, err := FindStackInSubfolders(terragruntOptions)
	if err != nil {
		t.Fatalf("Failed when calling method under test: %s\n", err.Error())
	}

	var modulePaths []string

	for _, module := range stack.Modules {
		relPath := strings.Replace(module.Path, tempFolder, "", 1)
		relPath = filepath.ToSlash(util.JoinPath(relPath, "terraform.tfvars"))

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

	return filepath.ToSlash(tmpFolder)
}

// Create an empty file at each of the given paths
func writeAsEmptyFiles(t *testing.T, tmpFolder string, paths []string) {
	for _, path := range paths {
		absPath := util.JoinPath(tmpFolder, path)

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
