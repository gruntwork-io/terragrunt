//go:build tf

package test_test

import (
	"bufio"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/require"
)

const testFixtureTelemetryErrorStatus = "fixtures/telemetry-error-status"

// consoleSpan is the part of a span printed by the console trace exporter that
// these tests read.
type consoleSpan struct {
	Name   string `json:"Name"`
	Status struct {
		Code        string `json:"Code"`
		Description string `json:"Description"`
	} `json:"Status"`
	Attributes []consoleAttribute `json:"Attributes"`
}

// consoleAttribute is one attribute of a [consoleSpan].
type consoleAttribute struct {
	Value struct {
		Value any `json:"Value"`
	} `json:"Value"`
	Key string `json:"Key"`
}

// TestTFTelemetryTracesMarkFailedSpans runs real failing commands with the
// console trace exporter and checks that every failed span carries the Error
// status and an error.type naming the underlying failure, not a wrapper or
// aggregate type.
func TestTFTelemetryTracesMarkFailedSpans(t *testing.T) {
	t.Parallel()

	const (
		processErrorType = "github.com/gruntwork-io/terragrunt/internal/util.ProcessExecutionError"
		diagnosticsType  = "github.com/hashicorp/hcl/v2.Diagnostics"
	)

	tests := []struct {
		name        string
		args        string
		dir         string
		errorType   string
		failedSpans []string
	}{
		{
			name:        "tofu failure",
			args:        "run plan",
			dir:         "run-all/fails",
			errorType:   processErrorType,
			failedSpans: []string{"run_tofu"},
		},
		{
			name:        "config parse failure",
			args:        "run plan",
			dir:         "bad-config",
			errorType:   diagnosticsType,
			failedSpans: []string{"parse_config_file"},
		},
		{
			name:        "run all with one failed unit",
			args:        "run --all plan",
			dir:         "run-all",
			errorType:   processErrorType,
			failedSpans: []string{"run_tofu", "unit_run", "runner_pool_task"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, testFixtureTelemetryErrorStatus)
			tmpEnvPath := helpers.CopyEnvironment(t, testFixtureTelemetryErrorStatus)
			workingDir := filepath.Join(tmpEnvPath, testFixtureTelemetryErrorStatus, tt.dir)

			v := venv.OSVenv()
			v.Env["TG_TELEMETRY_TRACE_EXPORTER"] = "console"

			stdout, _, err := helpers.RunTerragruntCommandWithOutputWithContext(
				t,
				t.Context(),
				v,
				"terragrunt "+tt.args+" --non-interactive -no-color --working-dir "+workingDir,
			)
			require.Error(t, err)

			spans := parseConsoleSpans(t, stdout)
			failed := map[string]int{}

			for _, span := range spans {
				if span.Status.Code != "Error" {
					continue
				}

				failed[span.Name]++

				require.NotEmpty(t, span.Status.Description, "span %q", span.Name)
				require.Equal(t, tt.errorType, span.attribute("error.type"), "span %q", span.Name)
			}

			for _, name := range tt.failedSpans {
				require.Equal(t, 1, failed[name], "failed %q spans", name)
			}
		})
	}
}

// parseConsoleSpans returns the spans the console trace exporter printed to
// stdout, skipping the lines tofu and Terragrunt printed around them.
func parseConsoleSpans(t *testing.T, stdout string) []consoleSpan {
	t.Helper()

	var spans []consoleSpan

	scanner := bufio.NewScanner(strings.NewReader(stdout))
	scanner.Buffer(nil, 1<<20)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, `{"Name":`) {
			continue
		}

		var span consoleSpan
		require.NoError(t, json.Unmarshal([]byte(line), &span), line)

		spans = append(spans, span)
	}

	require.NoError(t, scanner.Err())
	require.NotEmpty(t, spans, "no spans in stdout")

	return spans
}

// attribute returns the value of the span attribute named key, or nil when the
// span has no such attribute.
func (span consoleSpan) attribute(key string) any {
	i := slices.IndexFunc(span.Attributes, func(attr consoleAttribute) bool {
		return attr.Key == key
	})
	if i < 0 {
		return nil
	}

	return span.Attributes[i].Value.Value
}
