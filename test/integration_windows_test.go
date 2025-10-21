//go:build windows
// +build windows

package test_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureDownloadPath                         = "fixtures/download"
	testFixtureLocalRelativeArgsWindowsDownloadPath = "fixtures/download/local-windows"
	testFixtureManifestRemoval                      = "fixtures/manifest-removal"
	testFixtureTflintNoIssuesFound                  = "fixtures/tflint/no-issues-found"

	tempDir = `C:\tmp`
)

func TestMain(m *testing.M) {
	// By default, t.TempDir() creates a temporary directory inside the user directory
	// `C:/Users/circleci/AppData/Local/Temp/`, which ends up exceeding the maximum allowed length
	// and causes the error: "fatal: '$GIT_DIR' too big". Example:
	// "C:/Users/circleci/AppData/Local/Temp/TestWindowsLocalWithRelativeExtraArgsWindows1263358614/001/fixtures/download/local-windows/.terragrunt-cache/rviFlp3V5mrXldwi6Hbi8p2rDL0/U0tL3quoR7Yt-oR6jROJomrYpTs".
	// Solution, shorten the TMP path.

	envVars := map[string]string{"TMP": "", "TEMP": ""}

	// Save current values to restore them at the end.
	for name := range envVars {
		envVars[name] = os.Getenv(name)
	}

	defer func() {
		// Restore previous values.
		for name, val := range envVars {
			os.Setenv(name, val)
		}
	}()

	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		if err := os.Mkdir(tempDir, os.ModePerm); err != nil {
			fmt.Printf("Failed to create temp dir due to error: %v", err)
			os.Exit(1)
		}
	} else {
		// Verify write permissions
		testFile := filepath.Join(tempDir, ".write_test")
		if err := os.WriteFile(testFile, []byte(""), 0666); err != nil {
			fmt.Printf("Temp dir %s is not writable: %v", tempDir, err)
			os.Exit(1)
		}

		os.Remove(testFile)
	}

	// Set temporary values.
	for name := range envVars {
		if err := os.Setenv(name, tempDir); err != nil {
			fmt.Printf("Failed to set env var %s=%s due to error: %v", name, tempDir, err)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

func TestWindowsLocalWithRelativeExtraArgsWindows(t *testing.T) {
	t.Parallel()

	rootPath := CopyEnvironmentWithTflint(t, testFixtureDownloadPath)
	modulePath := util.JoinPath(rootPath, testFixtureLocalRelativeArgsWindowsDownloadPath)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s", modulePath))

	// Run a second time to make sure the temporary folder can be reused without errors
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s", modulePath))
}

// TestWindowsTerragruntSourceMapDebug copies the test/fixtures/source-map directory to a new Windows path
// and then ensures that the TG_SOURCE_MAP env var can be used to swap out git sources for local modules
func TestWindowsTerragruntSourceMapDebug(t *testing.T) {
	testCases := []struct {
		name string
	}{
		{
			name: "multiple-match",
		},
		{
			name: "multiple-with-dependency",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fixtureSourceMapPath := "fixtures/source-map"
			helpers.CleanupTerraformFolder(t, fixtureSourceMapPath)
			targetPath := "C:\\test\\infrastructure-modules/"
			CopyEnvironmentToPath(t, fixtureSourceMapPath, targetPath)
			rootPath := filepath.Join(targetPath, fixtureSourceMapPath)

			t.Setenv(
				"TG_SOURCE_MAP",
				strings.Join(
					[]string{
						fmt.Sprintf("git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=%s", targetPath),
						fmt.Sprintf("git::ssh://git@github.com/gruntwork-io/another-dont-exist.git=%s", targetPath),
					},
					",",
				),
			)
			tgPath := filepath.Join(rootPath, tc.name)
			tgArgs := fmt.Sprintf("terragrunt run --all apply --non-interactive --log-level trace --working-dir '%s'", tgPath)
			helpers.RunTerragrunt(t, tgArgs)
		})
	}
}

func TestWindowsTflintIsInvoked(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := CopyEnvironmentWithTflint(t, testFixtureTflintNoIssuesFound)
	modulePath := util.JoinPath(rootPath, testFixtureTflintNoIssuesFound)
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt plan --log-level trace --working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	assert.NotContains(t, errOut.String(), "Error while running tflint with args:")
	assert.NotContains(t, errOut.String(), "Tflint found issues in the project. Check for the tflint logs above.")

	found, err := regexp.MatchString(fmt.Sprintf("--config %s/.terragrunt-cache/.*/.tflint.hcl", modulePath), errOut.String())
	assert.NoError(t, err)
	assert.True(t, found)
}

func TestWindowsManifestFileIsRemoved(t *testing.T) {
	out := new(bytes.Buffer)
	errOut := new(bytes.Buffer)
	rootPath := CopyEnvironmentWithTflint(t, testFixtureManifestRemoval)
	modulePath := util.JoinPath(rootPath, testFixtureManifestRemoval, "app")
	err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt plan --non-interactive --log-level trace --working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	info1, err := fileInfo(modulePath, ".terragrunt-module-manifest")
	assert.NoError(t, err)
	assert.NotNil(t, info1)

	out = new(bytes.Buffer)
	errOut = new(bytes.Buffer)
	err = helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt plan --non-interactive --log-level trace --working-dir %s", modulePath), out, errOut)
	assert.NoError(t, err)

	info2, err := fileInfo(modulePath, ".terragrunt-module-manifest")
	assert.NoError(t, err)
	assert.NotNil(t, info2)

	// ensure that .terragrunt-module-manifest was recreated
	assert.True(t, (*info2).ModTime().After((*info1).ModTime()))
}

func fileInfo(path, fileName string) (*os.FileInfo, error) {
	var fileInfo *os.FileInfo
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if fileInfo != nil {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() && info.Name() == fileName {
			fileInfo = &info
			return nil
		}
		return nil
	})
	return fileInfo, err
}

func TestWindowsFindParent(t *testing.T) {
	t.Parallel()

	helpers.CleanupTerraformFolder(t, testFixtureFindParent)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run --all plan --non-interactive --working-dir %s", testFixtureFindParent))

	// second run shouldn't fail with find_in_parent_folders("root.hcl") issue
	helpers.RunTerragrunt(t, "terragrunt run --all --non-interactive --working-dir "+testFixtureFindParent+" -- apply -auto-approve")
}

func TestWindowsScaffold(t *testing.T) {
	t.Parallel()

	// create temp dir
	tmpDir, err := os.MkdirTemp("", "terragrunt-test")
	assert.NoError(t, err)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt scaffold github.com/gruntwork-io/terragrunt-infrastructure-modules-example//modules/mysql --working-dir '%s'", tmpDir))

	// check that terragrunt.hcl was created
	_, err = os.Stat(filepath.Join(tmpDir, "terragrunt.hcl"))
	assert.NoError(t, err)
}

func TestWindowsScaffoldRef(t *testing.T) {
	t.Parallel()

	// create temp dir
	tmpDir, err := os.MkdirTemp("", "terragrunt-test")
	assert.NoError(t, err)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt scaffold github.com/gruntwork-io/terragrunt-infrastructure-modules-example//modules/mysql?ref=v0.8.1 --working-dir '%s'", tmpDir))

	// check that terragrunt.hcl was created
	_, err = os.Stat(filepath.Join(tmpDir, "terragrunt.hcl"))
	assert.NoError(t, err)
}

func CopyEnvironmentToPath(t *testing.T, environmentPath, targetPath string) {
	if err := os.MkdirAll(targetPath, 0777); err != nil {
		t.Fatalf("Failed to create temp dir %s due to error %v", targetPath, err)
	}

	copyErr := util.CopyFolderContents(createLogger(), environmentPath, util.JoinPath(targetPath, environmentPath), ".terragrunt-test", nil, nil)
	require.NoError(t, copyErr)
}

func CopyEnvironmentWithTflint(t *testing.T, environmentPath string) string {
	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	tmpDir, err := os.MkdirTemp(currentDir, "terragrunt-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir due to error: %v", err)
	}

	t.Logf("Copying %s to %s", environmentPath, tmpDir)

	require.NoError(t, util.CopyFolderContents(createLogger(), environmentPath, util.JoinPath(tmpDir, environmentPath), ".terragrunt-test", []string{".tflint.hcl"}, []string{}))

	return tmpDir
}
