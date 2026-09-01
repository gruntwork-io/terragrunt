package test_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/info/print"
)

// NOTE: We don't run these tests in parallel because it modifies the environment variable, so it can affect other tests

func extractHostServiceLine(t *testing.T, terraformrc, service string) string {
	t.Helper()

	for line := range strings.SplitSeq(terraformrc, "\n") {
		if strings.Contains(line, `"`+service+`"`) {
			return line
		}
	}

	t.Fatalf("service %q not found in .terraformrc:\n%s", service, terraformrc)

	return ""
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
			name: "download dir not set",
			rootPath: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"not-set",
			),
			downloadDirReference: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"not-set",
				helpers.TerragruntCache,
			),
		},
		{
			name: "download dir set in config",
			rootPath: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
			),
			downloadDirReference: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
				".download",
			),
		},
		{
			name: "download dir set in config and in env var",
			rootPath: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
			),
			downloadDirEnv: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
				".env-var",
			),
			downloadDirReference: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
				".env-var",
			),
		},
		{
			name: "download dir set in config and as a flag",
			rootPath: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
			),
			downloadDirFlag: "--download-dir " + filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
				".flag-download",
			),
			downloadDirReference: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
				".flag-download",
			),
		},
		{
			name: "download dir set in config env and as a flag",
			rootPath: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
			),
			downloadDirEnv: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
				".env-var",
			),
			downloadDirFlag: "--download-dir " + filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
				".flag-download",
			),
			downloadDirReference: filepath.Join(
				tmpEnvPath,
				testFixtureGetOutput,
				"download-dir",
				"in-config",
				".flag-download",
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.downloadDirEnv != "" {
				t.Setenv("TG_DOWNLOAD_DIR", tc.downloadDirEnv)
			} else {
				// Clear the variable if it's not set. This is clearing the variable in case the variable is set outside the test process.
				require.NoError(t, os.Unsetenv("TG_DOWNLOAD_DIR"))
			}

			stdout := bytes.Buffer{}
			stderr := bytes.Buffer{}
			cmd := fmt.Sprintf(
				"terragrunt info print %s --non-interactive --working-dir %s",
				tc.downloadDirFlag,
				tc.rootPath,
			)
			err := helpers.RunTerragruntCommand(t, cmd, &stdout, &stderr)
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

func TestTerragruntValidateInputsWithEnvVar(t *testing.T) {
	t.Setenv("TF_VAR_input", "from the env")

	moduleDir := filepath.Join("fixtures/validate-inputs", "fail-no-inputs")
	helpers.RunTerragruntValidateInputs(t, moduleDir, nil, true)
}

func TestTerragruntValidateInputsWithUnusedEnvVar(t *testing.T) {
	t.Setenv("TF_VAR_unused", "from the env")

	moduleDir := filepath.Join("fixtures", "validate-inputs", "success-inputs-only")
	args := []string{"--strict"}
	helpers.RunTerragruntValidateInputs(t, moduleDir, args, false)
}

func TestTerragruntLogLevelEnvVarUnparsableLogsError(t *testing.T) {
	// NOTE: this matches logLevelEnvVar const in util/logger.go
	t.Setenv("TG_LOG_LEVEL", "unparsable")

	helpers.CleanupTerraformFolder(t, testFixtureInputs)
	tmpEnvPath := helpers.CopyEnvironment(t, ".")
	rootPath := filepath.Join(tmpEnvPath, testFixtureInputs)

	err := helpers.RunTerragruntCommand(
		t,
		"terragrunt validate --non-interactive --working-dir "+rootPath,
		os.Stdout,
		os.Stderr,
	)
	require.Error(t, err)

	assert.Contains(t, err.Error(), "invalid level")
}

func TestTerragruntStackProduceTelemetryTraces(t *testing.T) {
	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")

	helpers.CleanupTerraformFolder(t, testFixtureStacksBasic)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureStacksBasic)
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksBasic, "live")

	output, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt stack generate --working-dir "+rootPath,
	)
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
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksBasic)

	output, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt find --working-dir "+rootPath,
	)
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
	rootPath := filepath.Join(tmpEnvPath, testFixtureStacksBasic)

	output, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt list --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// check that output have Telemetry json output
	assert.Contains(t, output, "\"SpanContext\":")
	assert.Contains(t, output, "\"TraceID\":")
	assert.Contains(t, output, "\"Name\":\"list_discover\"")
	assert.Contains(t, output, "\"Name\":\"list_discovered_to_listed\"")
}

func TestTerragruntProduceTelemetryInCaseOfError(t *testing.T) {
	if helpers.IsWindows() {
		t.Skip("Skipping test on Windows since bash script execution is not supported")
	}

	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")
	t.Setenv("TRACEPARENT", "00-b2ff2d54551433d53dd807a6c94e81d1-0e6f631d793c718a-01")

	helpers.CleanupTerraformFolder(t, testFixtureHooksBeforeAndAfterPath)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureHooksBeforeAndAfterPath)
	rootPath := filepath.Join(tmpEnvPath, testFixtureHooksBeforeAndAfterPath)

	output, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt plan no-existing-command -auto-approve --non-interactive --working-dir "+rootPath,
	)
	require.Error(t, err)

	assert.Contains(t, output, "\"SpanContext\":{\"TraceID\":\"b2ff2d54551433d53dd807a6c94e81d1\"")
	assert.Contains(t, output, "\"SpanID\":\"0e6f631d793c718a\"")
	assert.Contains(t, output, "exception.message")
	assert.Contains(t, output, "\"Name\":\"exception\"")
}

func TestTerragruntTelemetryTraces(t *testing.T) {
	t.Setenv("TG_TELEMETRY_TRACE_EXPORTER", "console")

	helpers.CleanupTerraformFolder(t, testFixtureDependencyOutput)
	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureDependencyOutput)
	rootPath := filepath.Join(tmpEnvPath, testFixtureDependencyOutput)

	output, _, err := helpers.RunTerragruntCommandWithOutput(
		t,
		"terragrunt hcl format --working-dir "+rootPath,
	)
	require.NoError(t, err)

	// check that produced output has span traces
	assert.Contains(t, output, "\"SpanKind\":1")
	assert.Contains(t, output, "\"Parent\"")
}
