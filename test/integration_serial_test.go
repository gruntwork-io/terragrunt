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

	cleanupTerraformFolder(t, TestFixtureProviderCacheFilesystemMirror)
	tmpEnvPath := copyEnvironment(t, TestFixtureProviderCacheFilesystemMirror)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureProviderCacheFilesystemMirror)

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
	defer os.Unsetenv(terraform.EnvNameTFCLIConfigFile)

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

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-provider-cache --terragrunt-provider-cache-registry-names example.com --terragrunt-provider-cache-registry-names registry.opentofu.org --terragrunt-provider-cache-registry-names registry.terraform.io --terragrunt-provider-cache-dir %s --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", providerCacheDir, appPath))

	expectedProviderInstallation := `provider_installation { "filesystem_mirror" { path = "%s" include = ["example.com/*/*"] exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "filesystem_mirror" { path = "%s" include = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "direct" { } }`
	expectedProviderInstallation = fmt.Sprintf(strings.Join(strings.Fields(expectedProviderInstallation), " "), providersMirrorPath, providerCacheDir)

	terraformrcBytes, err := os.ReadFile(filepath.Join(appPath, ".terraformrc"))
	require.NoError(t, err)
	terraformrc := strings.Join(strings.Fields(string(terraformrcBytes)), " ")

	assert.Contains(t, terraformrc, expectedProviderInstallation, "%s\n\n%s", terraformrc, expectedProviderInstallation)
}

func TestTerragruntProviderCacheWithNetworkMirror(t *testing.T) {
	// In this test we use os.Setenv to set the Terraform env var TF_CLI_CONFIG_FILE.

	cleanupTerraformFolder(t, TestFixtureProviderCacheNetworkMirror)
	tmpEnvPath := copyEnvironment(t, TestFixtureProviderCacheNetworkMirror)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureProviderCacheNetworkMirror)

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

	// when we run NetworkMirrorServer, we override the default transport to configure the self-signed certificate, we need to restor, after finishing we need to restore this value
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

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-provider-cache --terragrunt-provider-cache-registry-names example.com --terragrunt-provider-cache-registry-names registry.opentofu.org --terragrunt-provider-cache-registry-names registry.terraform.io --terragrunt-provider-cache-dir %s --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", providerCacheDir, appPath))

	expectedProviderInstallation := `provider_installation { "filesystem_mirror" { path = "%s" include = ["example.com/hashicorp/azurerm"] exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "network_mirror" { url = "%s" exclude = ["example.com/hashicorp/azurerm", "example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "filesystem_mirror" { path = "%s" include = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } "direct" { exclude = ["example.com/*/*", "registry.opentofu.org/*/*", "registry.terraform.io/*/*"] } }`
	expectedProviderInstallation = fmt.Sprintf(strings.Join(strings.Fields(expectedProviderInstallation), " "), providersFilesystemMirrorPath, networkMirrorURL.String(), providerCacheDir)

	terraformrcBytes, err := os.ReadFile(filepath.Join(appPath, ".terraformrc"))
	require.NoError(t, err)
	terraformrc := strings.Join(strings.Fields(string(terraformrcBytes)), " ")

	assert.Contains(t, terraformrc, expectedProviderInstallation, "%s\n\n%s", terraformrc, expectedProviderInstallation)
}

func TestTerragruntInputsFromDependency(t *testing.T) {
	tmpEnvPath := copyEnvironment(t, TestFixtureInputsFromDependency)
	rootTerragruntPath := util.JoinPath(tmpEnvPath, TestFixtureInputsFromDependency)
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

			runTerragrunt(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s --terragrunt-download-dir=%s", appDir, testCase.downloadDir))
			config.ClearOutputCache()
		}

		if testCase.downloadDir != "" {
			entries, err := os.ReadDir(testCase.downloadDir)
			require.NoError(t, err)
			assert.Equal(t, len(appDirs), len(entries))
		}

		runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt output --terragrunt-non-interactive --terragrunt-working-dir %s  --terragrunt-download-dir=%s", appDir, testCase.downloadDir), &stdout, &stderr)

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
	cleanupTerraformFolder(t, testFixtureLocalRelativeDownloadPath)
	tmpEnvPath := copyEnvironment(t, TestFixtureGetOutput)

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
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "not-set"),
			"", // env
			"", // flag
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "not-set", TerragruntCache),
		},
		{
			"download dir set in config",
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config"),
			"", // env
			"", // flag
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config", ".download"),
		},
		{
			"download dir set in config and in env var",
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config"),
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config", ".env-var"),
			"", // flag
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config", ".env-var"),
		},
		{
			"download dir set in config and as a flag",
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config"),
			"", // env
			"--terragrunt-download-dir " + util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config", ".flag-download"),
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config", ".flag-download"),
		},
		{
			"download dir set in config env and as a flag",
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config"),
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config", ".env-var"),
			"--terragrunt-download-dir " + util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config", ".flag-download"),
			util.JoinPath(tmpEnvPath, TestFixtureGetOutput, "download-dir", "in-config", ".flag-download"),
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
			err := runTerragruntCommand(t, fmt.Sprintf("terragrunt terragrunt-info %s --terragrunt-non-interactive --terragrunt-working-dir %s", testCase.downloadDirFlag, testCase.rootPath), &stdout, &stderr)
			logBufferContentsLineByLine(t, stdout, "stdout")
			logBufferContentsLineByLine(t, stderr, "stderr")
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
	runTerragruntRedirectOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+TestFixtureExtraArgsPath, out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World from dev!")
}

func TestExtraArgumentsWithEnv(t *testing.T) {
	out := new(bytes.Buffer)
	t.Setenv("TF_VAR_env", "prod")
	runTerragruntRedirectOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+TestFixtureExtraArgsPath, out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World!")
}

func TestExtraArgumentsWithEnvVarBlock(t *testing.T) {
	out := new(bytes.Buffer)
	runTerragruntRedirectOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+TestFixtureEnvVarsBlockPath, out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "I'm set in extra_arguments env_vars")
}

func TestExtraArgumentsWithRegion(t *testing.T) {
	out := new(bytes.Buffer)
	t.Setenv("TF_VAR_region", "us-west-2")
	runTerragruntRedirectOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+TestFixtureExtraArgsPath, out, os.Stderr)
	t.Log(out.String())
	assert.Contains(t, out.String(), "Hello, World from Oregon!")
}

func TestPreserveEnvVarApplyAll(t *testing.T) {
	t.Setenv("TF_VAR_seed", "from the env")

	cleanupTerraformFolder(t, TestFixtureRegressions)
	tmpEnvPath := copyEnvironment(t, TestFixtureRegressions)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureRegressions, "apply-all-envvar")

	stdout := bytes.Buffer{}
	runTerragruntRedirectOutput(t, "terragrunt apply-all -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, os.Stderr)
	t.Log(stdout.String())

	// Check the output of each child module to make sure the inputs were overridden by the env var
	assertEnvVarModule := util.JoinPath(rootPath, "require-envvar")
	noRequireEnvVarModule := util.JoinPath(rootPath, "no-require-envvar")
	for _, mod := range []string{assertEnvVarModule, noRequireEnvVarModule} {
		stdout := bytes.Buffer{}
		err := runTerragruntCommand(t, "terragrunt output text -no-color --terragrunt-non-interactive --terragrunt-working-dir "+mod, &stdout, os.Stderr)
		require.NoError(t, err)
		assert.Contains(t, stdout.String(), "Hello from the env")
	}
}

func TestPriorityOrderOfArgument(t *testing.T) {
	out := new(bytes.Buffer)
	injectedValue := "Injected-directly-by-argument"
	runTerragruntRedirectOutput(t, fmt.Sprintf("terragrunt apply -auto-approve -var extra_var=%s --terragrunt-non-interactive --terragrunt-working-dir %s", injectedValue, TestFixtureExtraArgsPath), out, os.Stderr)
	t.Log(out.String())
	// And the result value for test should be the injected variable since the injected arguments are injected before the suplied parameters,
	// so our override of extra_var should be the last argument.
	assert.Contains(t, out.String(), fmt.Sprintf(`test = "%s"`, injectedValue))
}

func TestTerragruntValidateInputsWithEnvVar(t *testing.T) {
	t.Setenv("TF_VAR_input", "from the env")

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-no-inputs")
	runTerragruntValidateInputs(t, moduleDir, nil, true)
}

func TestTerragruntValidateInputsWithUnusedEnvVar(t *testing.T) {
	t.Setenv("TF_VAR_unused", "from the env")

	moduleDir := filepath.Join("fixtures", "validate-inputs", "success-inputs-only")
	args := []string{"--terragrunt-strict-validate"}
	runTerragruntValidateInputs(t, moduleDir, args, false)
}

func TestTerragruntSourceMapEnvArg(t *testing.T) {
	fixtureSourceMapPath := filepath.Join("fixtures", "source-map")
	cleanupTerraformFolder(t, fixtureSourceMapPath)
	tmpEnvPath := copyEnvironment(t, fixtureSourceMapPath)
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
	tgArgs := "terragrunt run-all apply -auto-approve --terragrunt-log-level debug --terragrunt-non-interactive --terragrunt-working-dir " + tgPath
	runTerragrunt(t, tgArgs)
}

func TestTerragruntLogLevelEnvVarOverridesDefault(t *testing.T) {
	// NOTE: this matches logLevelEnvVar const in util/logger.go
	t.Setenv("TERRAGRUNT_LOG_LEVEL", "debug")

	cleanupTerraformFolder(t, TestFixtureInputs)
	tmpEnvPath := copyEnvironment(t, ".")
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureInputs)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	require.NoError(
		t,
		runTerragruntCommand(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+rootPath, &stdout, &stderr),
	)
	output := stderr.String()
	assert.Contains(t, output, "level=debug")
}

func TestTerragruntLogLevelEnvVarUnparsableLogsErrorButContinues(t *testing.T) {
	// NOTE: this matches logLevelEnvVar const in util/logger.go
	t.Setenv("TERRAGRUNT_LOG_LEVEL", "unparsable")

	cleanupTerraformFolder(t, TestFixtureInputs)
	tmpEnvPath := copyEnvironment(t, ".")
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureInputs)

	// Ideally, we would check stderr to introspect the error message, but the global fallback logger only logs to real
	// stderr and we can't capture the output, so in this case we only make sure that the command runs successfully to
	// completion.
	runTerragrunt(t, "terragrunt validate --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
}

func TestTerragruntProduceTelemetryTraces(t *testing.T) {
	t.Setenv("TERRAGRUNT_TELEMETRY_TRACE_EXPORTER", "console")

	cleanupTerraformFolder(t, TestFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := copyEnvironment(t, TestFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureHooksBeforeAndAfterPath)

	output, _, err := runTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	// check that output have Telemetry json output
	assert.Contains(t, output, "\"SpanContext\":")
	assert.Contains(t, output, "\"TraceID\":")
	assert.Contains(t, output, "\"Name\":\"hook_after_hook_1\"")
	assert.Contains(t, output, "\"Name\":\"hook_after_hook_2\"")
}

func TestTerragruntProduceTelemetryMetrics(t *testing.T) {
	t.Setenv("TERRAGRUNT_TELEMETRY_METRIC_EXPORTER", "console")

	cleanupTerraformFolder(t, TestFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := copyEnvironment(t, TestFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureHooksBeforeAndAfterPath)

	output, _, err := runTerragruntCommandWithOutput(t, "terragrunt apply -no-color -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.NoError(t, err)

	// sleep for a bit to allow the metrics to be flushed
	time.Sleep(1 * time.Second)

	// check that output have Telemetry json output
	assert.Contains(t, output, "{\"Name\":\"hook_after_hook_2_duration\"")
	assert.Contains(t, output, "{\"Name\":\"run_")
	assert.Contains(t, output, ",\"IsMonotonic\":true}}")
}

func TestTerragruntOutputJson(t *testing.T) {
	// no parallel test execution since JSON output is global
	defer func() {
		util.DisableJSONFormat()
	}()

	tmpEnvPath := copyEnvironment(t, TestFixtureNotExistingSource)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TestFixtureNotExistingSource)

	_, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt apply --terragrunt-json-log --terragrunt-non-interactive --terragrunt-working-dir "+testPath)
	require.Error(t, err)

	// for windows OS
	output := bytes.ReplaceAll([]byte(stderr), []byte("\r\n"), []byte("\n"))

	multipeJSONs := bytes.Split(output, []byte("\n"))

	var msgs = make([]string, 0, len(multipeJSONs))

	for _, jsonBytes := range multipeJSONs {
		if len(jsonBytes) == 0 {
			continue
		}

		var output map[string]interface{}

		err = json.Unmarshal(jsonBytes, &output)
		require.NoError(t, err)

		msg, ok := output["msg"].(string)
		assert.True(t, ok)
		msgs = append(msgs, msg)
	}

	assert.Contains(t, strings.Join(msgs, ""), "Downloading Terraform configurations from git::https://github.com/gruntwork-io/terragrunt.git?ref=v0.9.9")
}

func TestTerragruntTerraformOutputJson(t *testing.T) {
	// no parallel test execution since JSON output is global
	defer util.DisableJSONFormat()

	tmpEnvPath := copyEnvironment(t, TestFixtureInitError)
	cleanupTerraformFolder(t, tmpEnvPath)
	testPath := util.JoinPath(tmpEnvPath, TestFixtureInitError)

	_, stderr, err := runTerragruntCommandWithOutput(t, "terragrunt apply --no-color --terragrunt-json-log --terragrunt-tf-logs-to-json --terragrunt-non-interactive --terragrunt-forward-tf-stdout --terragrunt-working-dir "+testPath)
	require.Error(t, err)

	assert.Contains(t, stderr, "\"msg\":\"Initializing the backend...")

	// check if output can be extracted in json
	jsonStrings := strings.Split(stderr, "\n")
	for _, jsonString := range jsonStrings {
		if len(jsonString) == 0 {
			continue
		}
		var output map[string]interface{}
		err = json.Unmarshal([]byte(jsonString), &output)
		require.NoErrorf(t, err, "Failed to parse json %s", jsonString)
		assert.NotNil(t, output["level"])
		assert.NotNil(t, output["time"])
	}
}

func TestTerragruntOutputFromDependencyLogsJson(t *testing.T) {
	// no parallel test execution since JSON output is global
	testCases := []struct {
		arg string
	}{
		{"--terragrunt-json-log"},
		{"--terragrunt-json-log --terragrunt-tf-logs-to-json"},
		{"--terragrunt-forward-tf-stdout"},
		{"--terragrunt-json-log --terragrunt-tf-logs-to-json --terragrunt-forward-tf-stdout"},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run("terragrunt output with "+testCase.arg, func(t *testing.T) {
			defer func() {
				util.DisableJSONFormat()
			}()
			tmpEnvPath := copyEnvironment(t, TestFixtureDependencyOutput)
			rootTerragruntPath := util.JoinPath(tmpEnvPath, TestFixtureDependencyOutput)
			// apply dependency first
			dependencyTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "dependency")
			_, _, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir %s ", dependencyTerragruntConfigPath))
			require.NoError(t, err)
			appTerragruntConfigPath := util.JoinPath(rootTerragruntPath, "app")
			stdout, stderr, err := runTerragruntCommandWithOutput(t, fmt.Sprintf("terragrunt plan --terragrunt-non-interactive --terragrunt-working-dir %s %s", appTerragruntConfigPath, testCase.arg))
			require.NoError(t, err)
			output := fmt.Sprintf("%s %s", stderr, stdout)
			assert.NotContains(t, output, "invalid character")
		})

	}
}

func TestTerragruntJsonPlanJsonOutput(t *testing.T) {
	// no parallel test execution since JSON output is global
	testCases := []struct {
		arg string
	}{
		{"--terragrunt-json-log"},
		{"--terragrunt-json-log --terragrunt-tf-logs-to-json"},
		{"--terragrunt-forward-tf-stdout"},
		{"--terragrunt-json-log --terragrunt-tf-logs-to-json --terragrunt-forward-tf-stdout"},
	}
	for _, testCase := range testCases {
		testCase := testCase
		t.Run("terragrunt with "+testCase.arg, func(t *testing.T) {
			defer func() {
				util.DisableJSONFormat()
			}()
			tmpDir := t.TempDir()
			_, _, _, err := testRunAllPlan(t, fmt.Sprintf("--terragrunt-json-out-dir %s %s", tmpDir, testCase.arg))
			require.NoError(t, err)
			list, err := findFilesWithExtension(tmpDir, ".json")
			require.NoError(t, err)
			assert.Len(t, list, 2)
			for _, file := range list {
				assert.Equal(t, "tfplan.json", filepath.Base(file))
				// verify that file is not empty
				content, err := os.ReadFile(file)
				require.NoError(t, err)
				assert.NotEmpty(t, content)
				// check that produced json is valid and can be unmarshalled
				var plan map[string]interface{}
				err = json.Unmarshal(content, &plan)
				require.NoError(t, err)
				// check that plan is not empty
				assert.NotEmpty(t, plan)
			}
		})

	}
}

func TestTerragruntProduceTelemetryTracesWithRootSpanAndTraceID(t *testing.T) {
	t.Setenv("TERRAGRUNT_TELEMETRY_TRACE_EXPORTER", "console")
	t.Setenv("TRACEPARENT", "00-b2ff2d54551433d53dd807a6c94e81d1-0e6f631d793c718a-01")

	cleanupTerraformFolder(t, TestFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := copyEnvironment(t, TestFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureHooksBeforeAndAfterPath)

	output, _, err := runTerragruntCommandWithOutput(t, "terragrunt apply -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
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

	cleanupTerraformFolder(t, TestFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := copyEnvironment(t, TestFixtureHooksBeforeAndAfterPath)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureHooksBeforeAndAfterPath)

	output, _, err := runTerragruntCommandWithOutput(t, "terragrunt no-existing-command -auto-approve --terragrunt-non-interactive --terragrunt-working-dir "+rootPath)
	require.Error(t, err)

	assert.Contains(t, output, "\"SpanContext\":{\"TraceID\":\"b2ff2d54551433d53dd807a6c94e81d1\"")
	assert.Contains(t, output, "\"SpanID\":\"0e6f631d793c718a\"")
	assert.Contains(t, output, "exception.message")
	assert.Contains(t, output, "\"Name\":\"exception\"")
}

// Since this test launches a large number of terraform processes, which sometimes fails with the message `Failed to write to log, write |1: file already closed`, for stability, we need to run it not parallel.
func TestTerragruntProviderCache(t *testing.T) {
	cleanupTerraformFolder(t, TestFixtureProviderCacheDirect)
	tmpEnvPath := copyEnvironment(t, TestFixtureProviderCacheDirect)
	rootPath := util.JoinPath(tmpEnvPath, TestFixtureProviderCacheDirect)

	cacheDir, err := util.GetCacheDir()
	require.NoError(t, err)
	providerCacheDir := filepath.Join(cacheDir, "provider-cache-test-direct")

	runTerragrunt(t, fmt.Sprintf("terragrunt run-all init --terragrunt-provider-cache --terragrunt-provider-cache-dir %s --terragrunt-log-level trace --terragrunt-non-interactive --terragrunt-working-dir %s", providerCacheDir, rootPath))

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
