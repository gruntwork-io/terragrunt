//go:build engine

package test_test

import (
	"fmt"

	"github.com/hashicorp/go-getter/v2"

	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/gruntwork-io/terragrunt/config"

	"github.com/gruntwork-io/terragrunt/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureEngineDependency     = "fixtures/engine/engine-dependencies"
	testFixtureLocalEngine          = "fixtures/engine/local-engine"
	testFixtureRemoteEngine         = "fixtures/engine/remote-engine"
	testFixtureOpenTofuEngine       = "fixtures/engine/opentofu-engine"
	testFixtureOpenTofuRunAll       = "fixtures/engine/opentofu-run-all"
	testFixtureOpenTofuLatestRunAll = "fixtures/engine/opentofu-latest-run-all"
	testFixtureEngineTraceParent    = "fixtures/engine/trace-parent"

	envVarExperimental = "TG_EXPERIMENTAL_ENGINE"
)

var (
	engineAssetName    = "terragrunt-iac-engine-opentofu_rpc_" + testEngineVersion() + "_" + runtime.GOOS + "_" + runtime.GOARCH
	engineAssetArchive = engineAssetName + ".zip"
	downloadURL        = fmt.Sprintf("https://github.com/gruntwork-io/terragrunt-engine-opentofu/releases/download/%s/%s", testEngineVersion(), engineAssetArchive)
)

//nolint:paralleltest
func TestEngineLocalPlan(t *testing.T) {
	rootPath := setupLocalEngine(t)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt plan --non-interactive --tf-forward-stdout --working-dir %s --log-level trace", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, engineAssetName)
	assert.Contains(t, stderr, "Tofu Initialization started")
	assert.Contains(t, stderr, "Tofu Initialization completed")
	assert.Contains(t, stderr, "Tofu Shutdown completed")
	assert.Contains(t, stdout, "1 to add, 0 to change, 0 to destroy.")
}

//nolint:paralleltest
func TestEngineLocalApply(t *testing.T) {
	rootPath := setupLocalEngine(t)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --tf-forward-stdout --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, engineAssetName)
	assert.Contains(t, stderr, "Tofu Initialization started")
	assert.Contains(t, stderr, "Tofu Initialization completed")
	assert.Contains(t, stderr, "Tofu Shutdown completed")
	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

func TestEngineOpentofu(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	helpers.CleanupTerraformFolder(t, testFixtureOpenTofuEngine)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOpenTofuEngine)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureOpenTofuEngine)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --tf-forward-stdout --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Tofu Initialization started")
	assert.Contains(t, stderr, "Tofu Initialization completed")
	assert.Contains(t, stderr, "Tofu Shutdown completed")
	assert.Contains(t, stdout, "OpenTofu has been successfully initialized")
	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

func TestEngineRunAllOpentofu(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	helpers.CleanupTerraformFolder(t, testFixtureOpenTofuRunAll)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOpenTofuRunAll)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureOpenTofuRunAll)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --tf-forward-stdout --working-dir %s -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stderr, "Tofu Initialization started")
	assert.Contains(t, stdout, "resource \"local_file\" \"test\"")
	assert.Contains(t, stdout, "filename             = \"./test.txt\"\n")
	assert.Contains(t, stdout, "OpenTofu has been successful")
	assert.Contains(t, stderr, "Tofu Shutdown completed")
	assert.Contains(t, stdout, "Apply complete!")
}

func TestEngineRunAllOpentofuCustomPath(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	cacheDir, rootPath := setupEngineCache(t)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --tf-forward-stdout --working-dir %s -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stdout, "OpenTofu has been successful")
	assert.Contains(t, stderr, "Tofu Shutdown completed")
	assert.Contains(t, stdout, "Apply complete!")

	// check if cache folder is not empty
	files, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.NotEmpty(t, files)
}

func TestEngineDownloadOverHttp(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	helpers.CleanupTerraformFolder(t, testFixtureRemoteEngine)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRemoteEngine)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRemoteEngine)

	platform := runtime.GOOS
	arch := runtime.GOARCH

	helpers.CopyAndFillMapPlaceholders(t, util.JoinPath(testFixtureRemoteEngine, "terragrunt.hcl"), util.JoinPath(rootPath, config.DefaultTerragruntConfigPath), map[string]string{
		"__hardcoded_url__": fmt.Sprintf("https://github.com/gruntwork-io/terragrunt-engine-opentofu/releases/download/v0.0.4/terragrunt-iac-engine-opentofu_rpc_v0.0.4_%s_%s.zip", platform, arch),
	})

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --tf-forward-stdout --working-dir "+rootPath)
	require.NoError(t, err)

	assert.Contains(t, stderr, "Tofu Initialization started")
	assert.Contains(t, stderr, "Tofu Initialization completed")
	assert.Contains(t, stderr, "Tofu Shutdown completed")
	assert.Contains(t, stdout, "OpenTofu has been successfully initialized")
	assert.Contains(t, stdout, "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.")
}

func TestEngineChecksumVerification(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	cachePath, rootPath := setupEngineCache(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --working-dir %s -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)

	// change the checksum of the package file
	version := "v0.0.16"
	platform := runtime.GOOS
	arch := runtime.GOARCH
	executablePath := fmt.Sprintf("terragrunt/plugins/iac-engine/rpc/%s/%s/%s/terragrunt-iac-engine-opentofu_rpc_%s_%s_%s", version, platform, arch, version, platform, arch)
	fullPath := util.JoinPath(cachePath, executablePath)

	// open the file and write some data
	file, err := os.OpenFile(fullPath, os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	nonExecutableData := []byte{0x00}
	if _, err := file.Write(nonExecutableData); err != nil {
		require.NoError(t, err)
	}

	require.NoError(t, file.Close())
	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --working-dir %s -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)

	require.Contains(t, stderr, "checksum list has unexpected SHA-256 hash")
}

func TestEngineDisableChecksumCheck(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	cachePath, rootPath := setupEngineCache(t)

	_, _, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --working-dir %s -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)

	err = filepath.Walk(cachePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(filepath.Base(path), "_SHA256SUMS") {
			// clean checksum list
			if err := os.Truncate(path, 0); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)

	// create separated directory for new tests
	helpers.CleanupTerraformFolder(t, testFixtureOpenTofuRunAll)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOpenTofuRunAll)
	rootPath = util.JoinPath(tmpEnvPath, testFixtureOpenTofuRunAll)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --working-dir %s -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)
	require.Contains(t, stderr, "verification failure")

	// disable checksum check
	t.Setenv("TG_ENGINE_SKIP_CHECK", "1")

	_, _, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --working-dir %s -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)
}

func TestEngineOpentofuLatestRunAll(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	helpers.CleanupTerraformFolder(t, testFixtureOpenTofuLatestRunAll)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOpenTofuLatestRunAll)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureOpenTofuLatestRunAll)

	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --tf-forward-stdout --working-dir %s -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)

	assert.Contains(t, stdout, "resource \"local_file\" \"test\"")
	assert.Contains(t, stdout, "filename             = \"./test.txt\"\n")
	assert.Contains(t, stderr, "Tofu Shutdown completed")
	assert.Contains(t, stdout, "Apply complete!")
}

func TestEngineDependency(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	helpers.CleanupTerraformFolder(t, testFixtureEngineDependency)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEngineDependency)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureEngineDependency)

	terragruntCmd := "terragrunt apply -log-level debug -no-color -auto-approve --non-interactive --tf-forward-stdout --working-dir %s"

	// Run apply in app1, make sure it uses engine
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(terragruntCmd, util.JoinPath(rootPath, "app1")))
	require.NoError(t, err)
	assert.Contains(t, stderr, "Using engine to run command: tofu apply -auto-approve -no-color")
	assert.Contains(t, stdout, "Changes to Outputs:")
	assert.Contains(t, stdout, "value = \"app1-test\"")

	// Run apply in app2, make sure it uses engine for both app1 output and apply
	stdout, stderr, err = helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf(terragruntCmd, util.JoinPath(rootPath, "app2")))
	require.NoError(t, err)
	assert.Contains(t, stderr, "prefix=../app1 msg=Using engine to run command: tofu output -json")
	assert.Contains(t, stderr, "msg=Using engine to run command: tofu apply -auto-approve -no-color")
	assert.Contains(t, stdout, "resource \"local_file\" \"test\"")
	assert.Contains(t, stdout, "content              = \"app1-test\"")
	assert.Contains(t, stdout, "filename             = \"./test.txt\"\n")
}

func TestEngineLogLevel(t *testing.T) {
	t.Setenv(envVarExperimental, "1")

	helpers.CleanupTerraformFolder(t, testFixtureOpenTofuLatestRunAll)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOpenTofuLatestRunAll)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureOpenTofuLatestRunAll)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt run --all --non-interactive --tf-forward-stdout --working-dir %s --log-level trace -- apply -no-color -auto-approve", rootPath))
	require.NoError(t, err)
	assert.Contains(t, stderr, "level=debug")
	assert.Contains(t, stderr, "[DEBUG] terragrunt-iac-engine-opentofu_rpc")
	assert.Contains(t, stderr, "[DEBUG] plugin exited")
}

func TestEngineTelemetry(t *testing.T) {
	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")
	t.Setenv(envVarExperimental, "1")

	helpers.CleanupTerraformFolder(t, testFixtureEngineTraceParent)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureEngineTraceParent)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureEngineTraceParent)

	str, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --non-interactive --working-dir "+rootPath)
	require.NoError(t, err)

	helpers.ValidateHookTraceParent(t, "hook_print_traceparent", str)
}

func setupEngineCache(t *testing.T) (string, string) {
	t.Helper()

	// create a temporary folder
	cacheDir := t.TempDir()
	t.Setenv("TG_ENGINE_CACHE_PATH", cacheDir)

	helpers.CleanupTerraformFolder(t, testFixtureOpenTofuRunAll)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureOpenTofuRunAll)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureOpenTofuRunAll)
	return cacheDir, rootPath
}

func setupLocalEngine(t *testing.T) string {
	t.Helper()

	t.Setenv(envVarExperimental, "1")

	helpers.CleanupTerraformFolder(t, testFixtureLocalEngine)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLocalEngine)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLocalEngine)

	// download engine to a local directory
	engineDir := util.JoinPath(rootPath, "engine")
	if err := os.MkdirAll(engineDir, 0755); err != nil {
		require.NoError(t, err)
	}
	_, err := getter.GetAny(t.Context(), engineDir, downloadURL)
	require.NoError(t, err)

	helpers.CopyAndFillMapPlaceholders(t, util.JoinPath(testFixtureLocalEngine, "terragrunt.hcl"), util.JoinPath(rootPath, config.DefaultTerragruntConfigPath), map[string]string{
		"__engine_source__": filepath.Join(engineDir, engineAssetName),
	})
	return rootPath
}

// testEngineVersion returns the version of the engine to be used in the test
func testEngineVersion() string {
	value, found := os.LookupEnv("TOFU_ENGINE_VERSION")
	if !found {
		return "v0.0.16"
	}
	return value
}
