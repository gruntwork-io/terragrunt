//nolint:paralleltest
package test_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gruntwork-io/terragrunt/test"
	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	terragruntinfo "github.com/gruntwork-io/terragrunt/cli/commands/terragrunt-info"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/terraform"
	"github.com/gruntwork-io/terragrunt/util"
)

// NOTE: We don't run these tests in parallel because it modifies the environment variable, so it can affect other tests

func TestTerragruntProviderCacheWithFilesystemMirror(t *testing.T) {
	// In this test we use os.Setenv to set the Terraform env var TF_CLI_CONFIG_FILE.

	helpers.CleanupTerraformFolder(t, testFixtureProviderCacheFilesystemMirror)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheFilesystemMirror)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureProviderCacheFilesystemMirror)

	appPath := filepath.Join(rootPath, "app")
	providersMirrorPath := filepath.Join(rootPath, "providers-mirror")

	fakeProvider := FakeProvider{
		RegistryName: "example.com",
		Namespace:    "hashicorp",
		Name:         "aws",
		Version:      "5.59.0",
		PlatformOS:   runtime.GOOS,
		PlatformArch: runtime.GOARCH,
	}
	fakeProvider.CreateMirror(t, providersMirrorPath)

	fakeProvider = FakeProvider{
		RegistryName: "example.com",
		Namespace:    "hashicorp",
		Name:         "azurerm",
		Version:      "3.113.0",
		PlatformOS:   runtime.GOOS,
		PlatformArch: runtime.GOARCH,
	}
	fakeProvider.CreateMirror(t, providersMirrorPath)

	providerCacheDir := filepath.Join(rootPath, "providers-cache")

	ctx := context.Background()
	defer ctx.Done()

	cliConfigFilename, err := os.CreateTemp("", "*")
	require.NoError(t, err)
	defer cliConfigFilename.Close()

	t.Setenv(terraform.EnvNameTFCLIConfigFile, cliConfigFilename.Name())

	t.Logf("%s=%s", terraform.EnvNameTFCLIConfigFile, cliConfigFilename.Name())

	cliConfigSettings := &test.CLIConfigSettings{
		FilesystemMirrorMethods: []test.CLIConfigProviderInstallationFilesystemMirror{
			{
				Path:    providersMirrorPath,
				Include: []string{"example.com/*/*"},
			},
		},
	}
	test.CreateCLIConfig(t, cliConfigFilename, cliConfigSettings)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-provider-cache --terragrunt-provider-cache-registry-names example.com --terragrunt-provider-cache-registry-names registry.opentofu.org --terragrunt-provider-cache-registry-names registry.terraform.io --terragrunt-provider-cache-dir %s --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", providerCacheDir, appPath))

	expectedProviderInstallation := `provider_installation { "filesystem_mirror" { path = "%s" include = ["example.com/*/*"] exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "filesystem_mirror" { path = "%s" include = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "direct" { } }`
	expectedProviderInstallation = fmt.Sprintf(strings.Join(strings.Fields(expectedProviderInstallation), " "), providersMirrorPath, providerCacheDir)

	terraformrcBytes, err := os.ReadFile(filepath.Join(appPath, ".terraformrc"))
	require.NoError(t, err)
	terraformrc := strings.Join(strings.Fields(string(terraformrcBytes)), " ")

	assert.Contains(t, terraformrc, expectedProviderInstallation, "%s\n\n%s", terraformrc, expectedProviderInstallation)
}

func TestTerragruntProviderCacheWithNetworkMirror(t *testing.T) {
	// In this test we use os.Setenv to set the Terraform env var TF_CLI_CONFIG_FILE.

	helpers.CleanupTerraformFolder(t, testFixtureProviderCacheNetworkMirror)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheNetworkMirror)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureProviderCacheNetworkMirror)

	appPath := filepath.Join(rootPath, "app")
	providersNetkworMirrorPath := filepath.Join(rootPath, "providers-network-mirror")
	providersFilesystemMirrorPath := filepath.Join(rootPath, "providers-filesystem-mirror")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	netowrkProvider := FakeProvider{
		RegistryName: "example.com",
		Namespace:    "hashicorp",
		Name:         "aws",
		Version:      "5.59.0",
		PlatformOS:   runtime.GOOS,
		PlatformArch: runtime.GOARCH,
	}
	netowrkProvider.CreateMirror(t, providersNetkworMirrorPath)

	filesystemProvider := FakeProvider{
		RegistryName: "example.com",
		Namespace:    "hashicorp",
		Name:         "azurerm",
		Version:      "3.113.0",
		PlatformOS:   runtime.GOOS,
		PlatformArch: runtime.GOARCH,
	}
	filesystemProvider.CreateMirror(t, providersFilesystemMirrorPath)

	// When we run NetworkMirrorServer, we override the default transport to configure the self-signed certificate.
	// After finishing, we need to restore this value.
	defaultTransport := http.DefaultTransport
	defer func() {
		http.DefaultTransport = defaultTransport
	}()

	token := "123456790"

	networkMirrorURL := runNetworkMirrorServer(t, ctx, "/providers/", providersNetkworMirrorPath, token)
	t.Logf("Network mirror URL: %s", networkMirrorURL)
	t.Logf("Provdiers network mirror path: %s", providersNetkworMirrorPath)
	t.Logf("Provdiers filesysmte mirror path: %s", providersFilesystemMirrorPath)

	providerCacheDir := filepath.Join(rootPath, "providers-cache")

	cliConfigFilename, err := os.CreateTemp("", "*")
	require.NoError(t, err)
	defer cliConfigFilename.Close()

	tokenEnvName := "TF_TOKEN_" + strings.ReplaceAll(networkMirrorURL.Hostname(), ".", "_")
	t.Setenv(tokenEnvName, token)
	defer os.Unsetenv(tokenEnvName)

	t.Setenv(terraform.EnvNameTFCLIConfigFile, cliConfigFilename.Name())
	defer os.Unsetenv(terraform.EnvNameTFCLIConfigFile)

	t.Logf("%s=%s", terraform.EnvNameTFCLIConfigFile, cliConfigFilename.Name())

	cliConfigSettings := &test.CLIConfigSettings{
		DirectMethods: []test.CLIConfigProviderInstallationDirect{
			{
				Exclude: []string{"example.com/*/*"},
			},
		},
		FilesystemMirrorMethods: []test.CLIConfigProviderInstallationFilesystemMirror{
			{
				Path:    providersFilesystemMirrorPath,
				Include: []string{"example.com/hashicorp/azurerm"},
			},
		},
		NetworkMirrorMethods: []test.CLIConfigProviderInstallationNetworkMirror{
			{
				URL:     networkMirrorURL.String(),
				Exclude: []string{"example.com/hashicorp/azurerm"},
			},
		},
	}
	test.CreateCLIConfig(t, cliConfigFilename, cliConfigSettings)

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-provider-cache --terragrunt-provider-cache-registry-names example.com --terragrunt-provider-cache-registry-names registry.opentofu.org --terragrunt-provider-cache-registry-names registry.terraform.io --terragrunt-provider-cache-dir %s --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", providerCacheDir, appPath))

	expectedProviderInstallation := `provider_installation { "filesystem_mirror" { path = "%s" include = ["example.com/hashicorp/azurerm"] exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "network_mirror" { url = "%s" exclude = ["example.com/hashicorp/azurerm", "example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "filesystem_mirror" { path = "%s" include = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "direct" { exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } }`
	expectedProviderInstallation = fmt.Sprintf(strings.Join(strings.Fields(expectedProviderInstallation), " "), providersFilesystemMirrorPath, networkMirrorURL.String(), providerCacheDir)

	terraformrcBytes, err := os.ReadFile(filepath.Join(appPath, ".terraformrc"))
	require.NoError(t, err)
	terraformrc := strings.Join(strings.Fields(string(terraformrcBytes)), " ")

	assert.Contains(t, terraformrc, expectedProviderInstallation, "%s\n\n%s", terraformrc, expectedProviderInstallation)
}

func TestTerragruntInputsFromDependency(t *testing.T) {
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureInputsFromDependency)
	rootTerragruntPath := util.JoinPath(tmpEnvPath, testFixtureInputsFromDependency)
	rootPath := util.JoinPath(rootTerragruntPath, "apps")

	curDir, err := os.Getwd()
	require.NoError(t, err)

	relRootPath, err := filepath.Rel(curDir, rootPath)
	require.NoError(t, err)

	testCases := []struct {
		rootPath    string
		downloadDir string
	}{
		{
			rootPath:    rootPath,
			downloadDir: "",
		},
		{
			rootPath:    relRootPath,
			downloadDir: filepath.Join(rootTerragruntPath, "download-dir"),
		},
	}

	for _, testCase := range testCases {
		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)

		var (
			appDir  string
			appDirs = []string{"c", "b", "a"}
		)

		for _, app := range appDirs {
			appDir = filepath.Join(testCase.rootPath, app)

			helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-download-dir=%s", appDir, testCase.downloadDir))
			config.ClearOutputCache()
		}

		if testCase.downloadDir != "" {
			entries, err := os.ReadDir(testCase.downloadDir)
			require.NoError(t, err)
			assert.Equal(t, len(appDirs), len(entries))
		}

		helpers.RunTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt output --terragrunt-non-interactive --terragrunt-working-dir %s  --terragrunt-download-dir=%s", appDir, testCase.downloadDir), &stdout, &stderr)

		expectedOutpus := map[string]string{
			"bar": "parent-bar",
			"baz": "b-baz",
			"foo": "c-foo",
		}

		output := stdout.String()
		for key, value := range expectedOutpus {
			assert.Contains(t, output, fmt.Sprintf("%s = %q\n", key, value))
		}
	}
}

func TestTerragruntDownloadDir(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureLocalRelativeDownloadPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureGetOutput)

	/* we have 2 terragrunt dirs here. One of them doesn't set the download_dir in the config,
	the other one does. Here we'll be checking for precedence, and if the download_dir is set
	according to the specified settings
	*/
	testCases := []struct {
		name                 string
		rootPath             string
		downloadDirEnv       string // download dir set as an env var
		downloadDirFlag      string // download dir set as a flag
		downloadDirReference string // the expected result
	}{
		{
			"download dir not set",
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "not-set"),
			"", // env
			"", // flag
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "not-set", helpers.TerragruntCache),
		},
		{
			"download dir set in config",
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config"),
			"", // env
			"", // flag
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config", ".download"),
		},
		{
			"download dir set in config and in env var",
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config"),
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config", ".env-var"),
			"", // flag
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config", ".env-var"),
		},
		{
			"download dir set in config and as a flag",
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config"),
			"", // env
			"--terragrunt-download-dir " + util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config", ".flag-download"),
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config", ".flag-download"),
		},
		{
			"download dir set in config env and as a flag",
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config"),
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config", ".env-var"),
			"--terragrunt-download-dir " + util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config", ".flag-download"),
			util.JoinPath(tmpEnvPath, testFixtureGetOutput, "download-dir", "in-config", ".flag-download"),
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			if testCase.downloadDirEnv != "" {
				t.Setenv("TERRAGRUNT_DOWNLOAD", testCase.downloadDirEnv)
			} else {
				// Clear the variable if it's not set. This is clearing the variable in case the variable is set outside the test process.
				require.NoError(t, os.Unsetenv("TERRAGRUNT_DOWNLOAD"))
			}
			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt terragrunt-info %s --terragrunt-non-interactive --terragrunt-working-dir %s", testCase.downloadDirFlag, testCase.rootPath), &stdout, &stderr)
			helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
			helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
			require.NoError(t, err)

			var dat terragruntinfo.TerragruntInfoGroup
			unmarshalErr := json.Unmarshal(stdout.Bytes(), &dat)
			require.NoError(t, unmarshalErr)
			// compare the results
			assert.Equal(t, testCase.downloadDirReference, dat.DownloadDir)
		})
	}

}

func TestExtraArguments(t *testing.T) {
	out := new(bytes.Buffer)
	helpers.RunTerragruntRedirectOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testFixtureExtraArgsPath, out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World from dev!")
}

func TestExtraArgumentsWithEnv(t *testing.T) {
	out := new(bytes.Buffer)
	t.Setenv("TF_VAR_env", "prod")
	helpers.RunTerragruntRedirectOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testFixtureExtraArgsPath, out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World!")
}

func TestExtraArgumentsWithEnvVarBlock(t *testing.T) {
	out := new(bytes.Buffer)
	helpers.RunTerragruntRedirectOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testFixtureEnvVarsBlockPath, out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "I'm set in extra_arguments env_vars")
}

func TestExtraArgumentsWithRegion(t *testing.T) {
	out := new(bytes.Buffer)
	t.Setenv("TF_VAR_region", "us-west-2")
	helpers.RunTerragruntRedirectOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testFixtureExtraArgsPath, out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World from Oregon!")
}

func TestPreserveEnvVarApplyAll(t *testing.T) {
	t.Setenv("TF_VAR_seed", "from the env")

	helpers.CleanupTerraformFolder(t, testFixtureRegressions)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureRegressions, "apply-all-envvar")

	stdout := bytes.Buffer{}
	helpers.RunTerragruntRedirectOutput(t, "terragrunt apply-all -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, os.Stderr)
	t.Log(stdout.String())

	// Check the output of each child module to make sure the inputs were overridden by the env var
	assertEnvVarModule := util.JoinPath(rootPath, "require-envvar")
	noRequireEnvVarModule := util.JoinPath(rootPath, "no-require-envvar")
	for _, mod := range []string{assertEnvVarModule, noRequireEnvVarModule} {
		stdout := bytes.Buffer{}
		err := helpers.RunTerragruntCommand(t, "terragrunt output text -no-color --terragrunt-non-interactive --terragrunt-working-dir "+mod, &stdout, os.Stderr)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Hello from the env")
	}
}

func TestPriorityOrderOfArgument(t *testing.T) {
	out := new(bytes.Buffer)
	injectedValue := "Injected-directly-by-argument"
	helpers.RunTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt apply -auto-approve -var extra_var=%s --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir %s", injectedValue, testFixtureExtraArgsPath), out, os.Stderr)
	t.Log(out.String())
	// And the result value for test should be the injected variable since the injected arguments are injected before the supplied parameters,
	// so our override of extra_var should be the last argument.
	assert.Contains(t, out.String(), fmt.Sprintf(`test = "%s"`, injectedValue))
}

func TestTerragruntValidateInputsWithEnvVar(t *testing.T) {
	t.Setenv("TF_VAR_input", "from the env")

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-no-inputs")
	helpers.RunTerragruntValidateInputs(t, moduleDir, nil, true)
}

func TestTerragruntValidateInputsWithUnusedEnvVar(t *testing.T) {
	t.Setenv("TF_VAR_unused", "from the env")

	moduleDir := filepath.Join("fixtures", "validate-inputs", "success-inputs-only")
	args := []string{"--terragrunt-strict-validate"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, false)
}

func TestTerragruntSourceMapEnvArg(t *testing.T) {
	fixtureSourceMapPath := filepath.Join("fixtures", "source-map")
	helpers.CleanupTerraformFolder(t, fixtureSourceMapPath)
	tmpEnvPath := helpers.CopyEnvironment(t, fixtureSourceMapPath)
	rootPath := filepath.Join(tmpEnvPath, fixtureSourceMapPath)

	t.Setenv(
		"TERRAGRUNT_SOURCE_MAP",
		strings.Join(
			[]string{
				"git::ssh://git@github.com/gruntwork-io/i-dont-exist.git=" + tmpEnvPath,
				"git::ssh://git@github.com/gruntwork-io/another-dont-exist.git=" + tmpEnvPath,
			},
			",",
		),
	)
	tgPath := filepath.Join(rootPath, "multiple-match")
	tgArgs := "terragrunt run-all apply -auto-approve --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir " + tgPath
	helpers.RunTerragrunt(t, tgArgs)
}

func TestTerragruntLogLevelEnvVarOverridesDefault(t *testing.T) {
	// NOTE: this matches logLevelEnvVar const in util/logger.go
	t.Setenv("TERRAGRUNT_LOG_LEVEL", "debug")

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInputs)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	require.NoError(
		t,
		helpers.RunTerragruntCommand(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)
	output := stderr.String()
	assert.Contains(t, output, "level=debug")
}

func TestTerragruntLogLevelEnvVarUnparsableLogsError(t *testing.T) {
	// NOTE: this matches logLevelEnvVar const in util/logger.go
	t.Setenv("TERRAGRUNT_LOG_LEVEL", "unparsable")

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")
	rootPath := util.JoinPath(tmpEnvPath, testFixtureInputs)

	err := helpers.RunTerragruntCommand(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, os.Stdout, os.Stderr)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "invalid level")
}

func TestTerragruntProduceTelemetryTraces(t *testing.T) {
	t.Setenv("TERRAGRUNT_TELEMETRY_TRACE_EXPORTER", "console")

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	// check that output have Telemetry json output
	assert.Contains(t, output, "\"SpanContext\":")
	assert.Contains(t, output, "\"TraceID\":")
	assert.Contains(t, output, "\"Name\":\"hook_after_hook_1\"")
	assert.Contains(t, output, "\"Name\":\"hook_after_hook_2\"")
}

func TestTerragruntProduceTelemetryMetrics(t *testing.T) {
	t.Setenv("TERRAGRUNT_TELEMETRY_METRIC_EXPORTER", "console")

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -no-color -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	// sleep for a bit to allow the metrics to be flushed
	time.Sleep(1 * time.Second)

	// check that output have Telemetry json output
	assert.Contains(t, output, "{\"Name\":\"hook_after_hook_2_duration\"")
	assert.Contains(t, output, "{\"Name\":\"run_")
	assert.Contains(t, output, ",\"IsMonotonic\":true}}")
}

func TestTerragruntProduceTelemetryTracesWithRootSpanAndTraceID(t *testing.T) {
	t.Setenv("TERRAGRUNT_TELEMETRY_TRACE_EXPORTER", "console")
	t.Setenv("TRACEPARENT", "00-b2ff2d54551433d53dd807a6c94e81d1-0e6f631d793c718a-01")

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	// check that output have Telemetry json output
	assert.Contains(t, output, "\"SpanContext\":{\"TraceID\":\"b2ff2d54551433d53dd807a6c94e81d1\"")
	assert.Contains(t, output, "\"SpanID\":\"0e6f631d793c718a\"")
	assert.Contains(t, output, "\"SpanContext\":")
	assert.Contains(t, output, "\"TraceID\":")
	assert.Contains(t, output, "\"Name\":\"hook_after_hook_1\"")
	assert.Contains(t, output, "\"Name\":\"hook_after_hook_2\"")
}

func TestTerragruntProduceTelemetryInCasOfError(t *testing.T) {
	t.Setenv("TERRAGRUNT_TELEMETRY_TRACE_EXPORTER", "console")
	t.Setenv("TRACEPARENT", "00-b2ff2d54551433d53dd807a6c94e81d1-0e6f631d793c718a-01")

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt no-existing-command -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.Error(t, err)

	assert.Contains(t, output, "\"SpanContext\":{\"TraceID\":\"b2ff2d54551433d53dd807a6c94e81d1\"")
	assert.Contains(t, output, "\"SpanID\":\"0e6f631d793c718a\"")
	assert.Contains(t, output, "exception.message")
	assert.Contains(t, output, "\"Name\":\"exception\"")
}

// Since this test launches a large number of terraform processes, which sometimes fails with the message `Failed to write to log, write |1: file already closed`, for stability, we need to run it not parallel.
func TestTerragruntProviderCache(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureProviderCacheDirect)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureProviderCacheDirect)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureProviderCacheDirect)

	cacheDir, err := util.GetCacheDir()
	require.NoError(t, err)
	providerCacheDir := filepath.Join(cacheDir, "provider-cache-test-direct")

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-provider-cache --terragrunt-provider-cache-dir %s --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", providerCacheDir, rootPath))

	providers := map[string][]string{
		"first": {
			"hashicorp/aws/5.36.0",
			"hashicorp/azurerm/3.95.0",
		},
		"second": {
			"hashicorp/aws/5.40.0",
			"hashicorp/azurerm/3.95.0",
			"hashicorp/kubernetes/2.27.0",
		},
	}

	registryName := "registry.opentofu.org"
	if isTerraform() {
		registryName = "registry.terraform.io"
	}

	for subDir, providers := range providers {
		var (
			actualApps   int
			expectedApps = 10
		)

		subDir = filepath.Join(rootPath, subDir)

		entries, err := os.ReadDir(subDir)
		require.NoError(t, err)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			actualApps++

			appPath := filepath.Join(subDir, entry.Name())

			lockfilePath := filepath.Join(appPath, ".terraform.lock.hcl")
			lockfileContent, err := os.ReadFile(lockfilePath)
			require.NoError(t, err)

			lockfile, diags := hclwrite.ParseConfig(lockfileContent, lockfilePath, hcl.Pos{Line: 1, Column: 1})
			assert.False(t, diags.HasErrors())

			for _, provider := range providers {
				var (
					actualProviderSymlinks   int
					expectedProviderSymlinks = 1
					provider                 = path.Join(registryName, provider)
				)

				providerBlock := lockfile.Body().FirstMatchingBlock("provider", []string{filepath.Dir(provider)})
				assert.NotNil(t, providerBlock)

				providerPath := filepath.Join(appPath, ".terraform/providers", provider)
				assert.True(t, util.FileExists(providerPath))

				entries, err := os.ReadDir(providerPath)
				require.NoError(t, err)

				for _, entry := range entries {
					actualProviderSymlinks++
					assert.Equal(t, fs.ModeSymlink, entry.Type())

					symlinkPath := filepath.Join(providerPath, entry.Name())

					actualPath, err := os.Readlink(symlinkPath)
					require.NoError(t, err)

					expectedPath := filepath.Join(providerCacheDir, provider, entry.Name())
					assert.Contains(t, actualPath, expectedPath)
				}
				assert.Equal(t, expectedProviderSymlinks, actualProviderSymlinks)
			}
		}
		assert.Equal(t, expectedApps, actualApps)
	}
}

func TestReadTerragruntAuthProviderCmdRemoteState(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "remote-state")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	s3BucketName := "terragrunt-test-bucket-" + strings.ToLower(helpers.UniqueID())
	defer helpers.DeleteS3Bucket(t, helpers.TerraformRemoteStateS3Region, s3BucketName)

	rootTerragruntConfigPath := util.JoinPath(rootPath, config.DefaultTerragruntConfigPath)
	helpers.CopyTerragruntConfigAndFillPlaceholders(t, rootTerragruntConfigPath, rootTerragruntConfigPath, s3BucketName, "not-used", helpers.TerraformRemoteStateS3Region)

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	// I'm not sure why, but this test doesn't work with tenv
	os.Setenv("AWS_ACCESS_KEY_ID", "")     //nolint: tenv
	os.Setenv("AWS_SECRET_ACCESS_KEY", "") //nolint: tenv

	defer func() {
		os.Setenv("AWS_ACCESS_KEY_ID", accessKeyID)
		os.Setenv("AWS_SECRET_ACCESS_KEY", secretAccessKey)
	}()

	credsConfig := util.JoinPath(rootPath, "creds.config")

	helpers.CopyAndFillMapPlaceholders(t, credsConfig, credsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s", rootPath, mockAuthCmd))
}

func TestReadTerragruntAuthProviderCmdCredsForDependency(t *testing.T) {
	helpers.CleanupTerraformFolder(t, testFixtureAuthProviderCmd)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureAuthProviderCmd)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureAuthProviderCmd, "creds-for-dependency")
	mockAuthCmd := filepath.Join(tmpEnvPath, testFixtureAuthProviderCmd, "mock-auth-cmd.sh")

	accessKeyID := os.Getenv("AWS_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("AWS_SECRET_ACCESS_KEY")
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")

	dependencyCredsConfig := util.JoinPath(rootPath, "dependency", "creds.config")
	helpers.CopyAndFillMapPlaceholders(t, dependencyCredsConfig, dependencyCredsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})

	dependentCredsConfig := util.JoinPath(rootPath, "dependent", "creds.config")
	helpers.CopyAndFillMapPlaceholders(t, dependentCredsConfig, dependentCredsConfig, map[string]string{
		"__FILL_AWS_ACCESS_KEY_ID__":     accessKeyID,
		"__FILL_AWS_SECRET_ACCESS_KEY__": secretAccessKey,
	})
	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run-all apply --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-auth-provider-cmd %s", rootPath, mockAuthCmd))
}

func TestParseTFLog(t *testing.T) {
	t.Setenv("TF_LOG", "info")

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all init --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-log-format=pretty -no-color --terragrunt-no-color --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	for _, prefixName := range []string{"app", "dep"} {
		assert.Contains(t, stderr, "INFO   ["+prefixName+"] "+wrappedBinary()+`: TF_LOG: Go runtime version`)
	}
}
