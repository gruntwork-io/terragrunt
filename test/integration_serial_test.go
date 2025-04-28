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

	print "github.com/gruntwork-io/terragrunt/cli/commands/info/print"
	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/tf"
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

	cliConfigFilename, err := os.CreateTemp(t.TempDir(), "*")
	require.NoError(t, err)
	defer cliConfigFilename.Close()

	t.Setenv(tf.EnvNameTFCLIConfigFile, cliConfigFilename.Name())

	t.Logf("%s=%s", tf.EnvNameTFCLIConfigFile, cliConfigFilename.Name())

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

	expectedProviderInstallation := `provider_installation { "filesystem_mirror" { include = ["example.com/*/*"] exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] path = "%s" } "filesystem_mirror" { include = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] path = "%s" } "direct" { } }`
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

	appsPath := filepath.Join(rootPath, "apps")
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
		Name:         "aws",
		Version:      "5.58.0",
		PlatformOS:   runtime.GOOS,
		PlatformArch: runtime.GOARCH,
	}
	filesystemProvider.CreateMirror(t, providersFilesystemMirrorPath)

	filesystemProvider = FakeProvider{
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

	cliConfigFilename, err := os.CreateTemp(t.TempDir(), "*")
	require.NoError(t, err)
	defer cliConfigFilename.Close()

	tokenEnvName := "TF_TOKEN_" + strings.ReplaceAll(networkMirrorURL.Hostname(), ".", "_")
	t.Setenv(tokenEnvName, token)
	defer os.Unsetenv(tokenEnvName)

	t.Setenv(tf.EnvNameTFCLIConfigFile, cliConfigFilename.Name())
	defer os.Unsetenv(tf.EnvNameTFCLIConfigFile)

	t.Logf("%s=%s", tf.EnvNameTFCLIConfigFile, cliConfigFilename.Name())

	cliConfigSettings := &test.CLIConfigSettings{
		DirectMethods: []test.CLIConfigProviderInstallationDirect{
			{
				Exclude: []string{"example.com/*/*"},
			},
		},
		FilesystemMirrorMethods: []test.CLIConfigProviderInstallationFilesystemMirror{
			{
				Path:    providersFilesystemMirrorPath,
				Include: []string{"example.com/hashicorp/azurerm", "example.com/hashicorp/aws"},
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

	helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-provider-cache --terragrunt-provider-cache-registry-names example.com --terragrunt-provider-cache-registry-names registry.opentofu.org --terragrunt-provider-cache-registry-names registry.terraform.io --terragrunt-provider-cache-dir %s --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", providerCacheDir, appsPath))

	expectedProviderInstallation := `provider_installation { "filesystem_mirror" { include = ["example.com/hashicorp/azurerm", "example.com/hashicorp/aws"] exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] path = "%s" } "network_mirror" { exclude = ["example.com/hashicorp/azurerm", "example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] url = "%s" } "filesystem_mirror" { include = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] path = "%s" } "direct" { exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } }`
	expectedProviderInstallation = fmt.Sprintf(strings.Join(strings.Fields(expectedProviderInstallation), " "), providersFilesystemMirrorPath, networkMirrorURL.String(), providerCacheDir)

	for _, filename := range []string{"app0/.terraformrc", "app1/.terraformrc"} {
		terraformrcBytes, err := os.ReadFile(filepath.Join(appsPath, filename))
		require.NoError(t, err)
		terraformrc := strings.Join(strings.Fields(string(terraformrcBytes)), " ")

		assert.Contains(t, terraformrc, expectedProviderInstallation, "%s\n\n%s", terraformrc, expectedProviderInstallation)
	}
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

	for _, tc := range testCases {
		var (
			stdout bytes.Buffer
			stderr bytes.Buffer
		)

		var (
			appDir  string
			appDirs = []string{"c", "b", "a"}
		)

		for _, app := range appDirs {
			appDir = filepath.Join(tc.rootPath, app)

			helpers.RunTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --non-interactive --working-dir %s --download-dir=%s", appDir, tc.downloadDir))
			config.ClearOutputCache()
		}

		if tc.downloadDir != "" {
			entries, err := os.ReadDir(tc.downloadDir)
			require.NoError(t, err)
			assert.Equal(t, len(appDirs), len(entries))
		}

		helpers.RunTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt output --non-interactive --working-dir %s  --download-dir=%s", appDir, tc.downloadDir), &stdout, &stderr)

		expectedOutpus := map[string]string{
			"bar": "parent-bar",
			"baz": "b-baz",
			"foo": "c-foo",
		}

		output := stdout.String()
		for key, value := range expectedOutpus {
			assert.Contains(t, output, fmt.Sprintf("%s = %q\n", key, value))
		}

		// Check that we're getting a warning for usage of deprecated functionality.
		assert.Contains(
			t,
			stderr.String(),
			"Reading inputs from dependencies has been deprecated and will be removed in a future version of Terragrunt. If a value in a dependency is needed, use dependency outputs instead.",
		)
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.downloadDirEnv != "" {
				t.Setenv("TERRAGRUNT_DOWNLOAD", tc.downloadDirEnv)
			} else {
				// Clear the variable if it's not set. This is clearing the variable in case the variable is set outside the test process.
				require.NoError(t, os.Unsetenv("TERRAGRUNT_DOWNLOAD"))
			}
			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt info print %s --terragrunt-non-interactive --terragrunt-working-dir %s", tc.downloadDirFlag, tc.rootPath), &stdout, &stderr)
			helpers.LogBufferContentsLineByLine(t, stdout, "stdout")
			helpers.LogBufferContentsLineByLine(t, stderr, "stderr")
			require.NoError(t, err)

			var dat print.InfoOutput
			unmarshalErr := json.Unmarshal(stdout.Bytes(), &dat)
			require.NoError(t, unmarshalErr)
			// compare the results
			assert.Equal(t, tc.downloadDirReference, dat.DownloadDir)
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
	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}
	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	// check that output has Telemetry JSON traces
	assert.Contains(t, output, "\"SpanContext\":")
	assert.Contains(t, output, "\"TraceID\":")
	assert.Contains(t, output, "\"Name\":\"hook_after_hook_1\"")
	assert.Contains(t, output, "\"Name\":\"hook_after_hook_2\"")
}

func TestTerragruntStackProduceTelemetryTraces(t *testing.T) {
	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic, "live")

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt stack generate --working-dir "+rootPath)
	require.NoError(t, err)

	// check that output has Telemetry JSON traces
	assert.Contains(t, output, "\"SpanContext\":")
	assert.Contains(t, output, "\"TraceID\":")
	assert.Contains(t, output, "\"Name\":\"stack_generate_unit\"")
	assert.Contains(t, output, "\"Name\":\"stack_generate\"")
}

func TestTerragruntFindProduceTelemetryTraces(t *testing.T) {
	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic)

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt find --working-dir "+rootPath)
	require.NoError(t, err)

	// check that output have Telemetry json output
	assert.Contains(t, output, "\"SpanContext\":")
	assert.Contains(t, output, "\"TraceID\":")
	assert.Contains(t, output, "\"Name\":\"find_discover\"")
	assert.Contains(t, output, "\"Name\":\"find_discovered_to_found\"")
}

func TestTerragruntListProduceTelemetryTraces(t *testing.T) {
	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureStacksBasic)

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt list --working-dir "+rootPath)
	require.NoError(t, err)

	// check that output have Telemetry json output
	assert.Contains(t, output, "\"SpanContext\":")
	assert.Contains(t, output, "\"TraceID\":")
	assert.Contains(t, output, "\"Name\":\"list_discover\"")
	assert.Contains(t, output, "\"Name\":\"list_discovered_to_listed\"")
}

func TestTerragruntProduceTelemetryMetrics(t *testing.T) {
	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}
	t.Setenv("TG_TELEMETRY_METRIC_EXPORTER", "console")

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
	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")
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
	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}
	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")
	t.Setenv("TRACEPARENT", "00-b2ff2d54551433d53dd807a6c94e81d1-0e6f631d793c718a-01")

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	output, _, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt plan no-existing-command -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
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

func TestParseTFLog(t *testing.T) {
	t.Setenv("TF_LOG", "info")

	helpers.CleanupTerraformFolder(t, testFixtureLogFormatter)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureLogFormatter)
	rootPath := util.JoinPath(tmpEnvPath, testFixtureLogFormatter)

	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	_, stderr, err := helpers.RunTerragruntCommandWithOutput(t, "terragrunt run-all init --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-log-format=pretty --no-color --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	for _, prefixName := range []string{"app", "dep"} {
		assert.Contains(t, stderr, "INFO   ["+prefixName+"] "+wrappedBinary()+`: TF_LOG: Go runtime version`)
	}
}

// This test is flaky when run in parallel. Need to figure it out. Most likely after these changes
// https://github.com/gruntwork-io/terragrunt/issues/3864 or because of global variables in codes.
func TestTerragruntGraphNonTerraformCommandExecution(t *testing.T) {
	testCases := []struct {
		args string
	}{
		{"graph render-json --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-graph-root %s"},
		// NOTE: This command doesn't have an equivalent in the new CLI redesign, as it doesn't really make sense to support a `graph` flag for the `render` command.
		{"render-json --graph --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-graph-root %s"},
	}

	for _, tc := range testCases {
		t.Run("terragrunt args: "+tc.args, func(t *testing.T) {
			tmpEnvPath := prepareGraphFixture(t)
			tmpModulePath := util.JoinPath(tmpEnvPath, testFixtureGraph, "eks")

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}

			err := helpers.RunTerragruntCommand(t, fmt.Sprintf("terragrunt "+tc.args, tmpModulePath, tmpEnvPath), &stdout, &stderr)
			require.NoError(t, err)

			// check that terragrunt_rendered.json is created in mod1/mod2/mod3
			for _, module := range []string{"services/eks-service-1", "eks"} {
				_, err = os.Stat(util.JoinPath(tmpEnvPath, testFixtureGraph, module, "terragrunt_rendered.json"))
				require.NoError(t, err)
			}
		})
	}
}
