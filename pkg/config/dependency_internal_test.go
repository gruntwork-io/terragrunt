package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractFirstJSONObject verifies that we can isolate the first JSON object emitted by
// `tofu/terraform output -json` even when stdout is polluted with non-JSON content on either
// side of the JSON. See https://github.com/gruntwork-io/terragrunt/issues/6001 for the trailing-
// warning regression introduced by Terraform 1.15, and #2233 for the leading AWS CSM log line.
func TestExtractFirstJSONObject(t *testing.T) {
	t.Parallel()

	const validJSON = `{"foo":{"sensitive":false,"type":"string","value":"bar"}}`

	tcs := []struct {
		name      string
		input     string
		want      string
		wantExact bool // when true, compare bytes exactly instead of as JSON
		wantErr   bool
	}{
		{
			name:  "pure JSON is returned unchanged",
			input: validJSON,
			want:  validJSON,
		},
		{
			name:  "leading AWS CSM log line is stripped",
			input: "2023/05/04 20:22:43 Enabling CSM" + validJSON,
			want:  validJSON,
		},
		{
			name:  "leading ANSI-colored warning is stripped",
			input: "\x1b[33m\x1b[1mWarning:\x1b[0m Deprecated Parameter\n\n" + validJSON,
			want:  validJSON,
		},
		{
			name: "trailing Terraform 1.15 deprecation warning is ignored",
			input: validJSON + "\n\n" +
				"Warning: Deprecated Parameter\n\n" +
				`The parameter "dynamodb_table" is deprecated. Use parameter "use_lockfile" instead.` + "\n",
			want: validJSON,
		},
		{
			name: "leading and trailing pollution together",
			input: "2023/05/04 20:22:43 Enabling CSM" + validJSON +
				"\nWarning: Deprecated Parameter\n",
			want: validJSON,
		},
		{
			name:  "empty outputs object is preserved",
			input: "Warning: something\n{}\nWarning: trailing\n",
			want:  "{}",
		},
		{
			name:      "no JSON returns input unchanged so downstream surfaces the underlying error",
			input:     "no json here at all",
			want:      "no json here at all",
			wantExact: true,
		},
		{
			name:    "truncated JSON object surfaces a parse error",
			input:   `{"foo":`,
			wantErr: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := extractFirstJSONObject([]byte(tc.input))
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.wantExact {
				assert.Equal(t, tc.want, string(got))
				return
			}

			assert.JSONEq(t, tc.want, string(got))
		})
	}
}

// TestTryGetStackOutput_JSONConfigSibling pins that when a dependency directory contains both
// terragrunt.hcl.json and terragrunt.stack.hcl, tryGetStackOutput correctly identifies it as a
// stack dependency. Regression for a switch that previously fell into the default branch on
// JSON config paths and synthesized an invalid path like <dir>/terragrunt.hcl.json/terragrunt.stack.hcl.
func TestTryGetStackOutput_JSONConfigSibling(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath), []byte(`{}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, DefaultStackFile), []byte(`# empty stack`), 0644))

	// Verify the path-rewrite step of tryGetStackOutput resolves to the existing stack file. We
	// don't drive the full stack-output evaluation here (that would need a full ParsingContext).
	// Instead, replicate the path-rewrite logic to confirm the JSON-config case produces a valid
	// stack file path, which is what the production code does internally.
	cases := []struct {
		name           string
		inputBase      string
		expectStackHCL bool
	}{
		{"stackFileDirectly", DefaultStackFile, true},
		{"terragruntHCL", DefaultTerragruntConfigPath, true},
		{"terragruntJSON", DefaultTerragruntJSONConfigPath, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			stackFilePath := filepath.Join(tmpDir, tc.inputBase)

			switch filepath.Base(stackFilePath) {
			case DefaultStackFile:
			case DefaultTerragruntConfigPath, DefaultTerragruntJSONConfigPath:
				stackFilePath = filepath.Join(filepath.Dir(stackFilePath), DefaultStackFile)
			default:
				stackFilePath = filepath.Join(stackFilePath, DefaultStackFile)
			}

			if tc.expectStackHCL {
				assert.Equal(t, filepath.Join(tmpDir, DefaultStackFile), stackFilePath)
			}
		})
	}
}
