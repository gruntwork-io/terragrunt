package config

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/aws/smithy-go"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	azurermbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	gcsbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/gcs"
	s3backend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/s3"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
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

// TestResolveStackFilePath pins resolveStackFilePath across dependency-target shapes (direct stack file, explicit terragrunt config, bare directory).
func TestResolveStackFilePath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	wantStack := filepath.Join(tmpDir, DefaultStackFile)

	cases := []struct {
		name   string
		raw    string
		target string
		want   string
		wantOK bool
	}{
		{"stackFileDirectly", filepath.Join(tmpDir, DefaultStackFile), wantStack, wantStack, true},
		{
			"explicitTerragruntHCL",
			filepath.Join(tmpDir, DefaultTerragruntConfigPath),
			filepath.Join(tmpDir, DefaultTerragruntConfigPath),
			"",
			false,
		},
		{
			"explicitTerragruntJSON",
			filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath),
			filepath.Join(tmpDir, DefaultTerragruntJSONConfigPath),
			"",
			false,
		},
		{
			"bareDirectory",
			tmpDir,
			filepath.Join(tmpDir, DefaultTerragruntConfigPath),
			wantStack,
			true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, ok := resolveStackFilePath(tc.raw, tc.target)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// FuzzResolveStackFilePath verifies the helper never panics and every handled candidate ends in DefaultStackFile.
func FuzzResolveStackFilePath(f *testing.F) {
	seeds := [][2]string{
		{"/abs/dir/" + DefaultStackFile, "/abs/dir/" + DefaultStackFile},
		{"/abs/dir", "/abs/dir/" + DefaultTerragruntConfigPath},
		{"/abs/dir", "/abs/dir/" + DefaultTerragruntJSONConfigPath},
		{"/abs/dir/" + DefaultTerragruntConfigPath, "/abs/dir/" + DefaultTerragruntConfigPath},
		{"relative/dir", "relative/dir/" + DefaultTerragruntConfigPath},
		{"", ""},
		{".", "./" + DefaultTerragruntConfigPath},
		{"/", "/" + DefaultTerragruntConfigPath},
		{"\x00", "\x00"},
		{"unicode/café", "unicode/café/" + DefaultTerragruntConfigPath},
	}

	for _, seed := range seeds {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, raw, target string) {
		got, ok := resolveStackFilePath(raw, target)
		if !ok {
			require.Empty(
				t,
				got,
				"resolveStackFilePath must return empty string when ok=false (raw=%q target=%q got=%q)",
				raw,
				target,
				got,
			)

			return
		}

		require.Equal(
			t,
			DefaultStackFile,
			filepath.Base(got),
			"resolveStackFilePath must return a path whose base is %s when ok=true (raw=%q target=%q got=%q)",
			DefaultStackFile,
			raw,
			target,
			got,
		)
	})
}

func TestApplyExtraArgsEnvVarsForOutput(t *testing.T) {
	t.Parallel()

	envVars := func(m map[string]string) *map[string]string { return &m }

	tcs := []struct {
		initial   map[string]string
		terraform *TerraformConfig
		want      map[string]string
		name      string
	}{
		{
			name:    "applies env vars when commands include output",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "secrets",
						Commands: []string{"output", "plan"},
						EnvVars:  envVars(map[string]string{"TF_VAR_passphrase": "secret"}),
					},
				},
			},
			want: map[string]string{"TF_VAR_passphrase": "secret"},
		},
		{
			name:    "skips env vars when commands exclude output",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "secrets",
						Commands: []string{"apply", "plan"},
						EnvVars:  envVars(map[string]string{"TF_VAR_passphrase": "secret"}),
					},
				},
			},
			want: map[string]string{},
		},
		{
			name:    "skips env vars when commands list is empty",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "secrets",
						Commands: nil,
						EnvVars:  envVars(map[string]string{"TF_VAR_passphrase": "secret"}),
					},
				},
			},
			want: map[string]string{},
		},
		{
			name:      "nil terraform config is a no-op",
			initial:   map[string]string{"EXISTING": "value"},
			terraform: nil,
			want:      map[string]string{"EXISTING": "value"},
		},
		{
			name:      "terraform config without extra args is a no-op",
			initial:   map[string]string{"EXISTING": "value"},
			terraform: &TerraformConfig{},
			want:      map[string]string{"EXISTING": "value"},
		},
		{
			name:    "nil env vars is a no-op",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{Name: "secrets", Commands: []string{"output"}, EnvVars: nil},
				},
			},
			want: map[string]string{},
		},
		{
			name:    "later block wins on overlapping keys",
			initial: map[string]string{},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "first",
						Commands: []string{"output"},
						EnvVars:  envVars(map[string]string{"KEY": "first"}),
					},
					{
						Name:     "second",
						Commands: []string{"output"},
						EnvVars:  envVars(map[string]string{"KEY": "second"}),
					},
				},
			},
			want: map[string]string{"KEY": "second"},
		},
		{
			name:    "extra args env vars override existing env",
			initial: map[string]string{"TF_VAR_passphrase": "old"},
			terraform: &TerraformConfig{
				ExtraArgs: []TerraformExtraArguments{
					{
						Name:     "secrets",
						Commands: []string{"output"},
						EnvVars:  envVars(map[string]string{"TF_VAR_passphrase": "new"}),
					},
				},
			},
			want: map[string]string{"TF_VAR_passphrase": "new"},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pctx := &ParsingContext{Venv: venvtest.NewOSWithEmptyEnv().WithEnv(tc.initial)}
			applyExtraArgsEnvVarsForOutput(pctx, tc.terraform)
			assert.Equal(t, tc.want, pctx.Venv.Env)
		})
	}
}

func TestShouldFetchDependencyOutputFromState(t *testing.T) {
	t.Parallel()

	testDir := t.TempDir()
	oidcTokenPath := filepath.Join(testDir, "oidc-token")
	require.NoError(t, os.WriteFile(oidcTokenPath, []byte("test-token"), 0o600))

	gcsServiceAccountPath := filepath.Join(testDir, "gcs-service-account.json")
	require.NoError(t, os.WriteFile(gcsServiceAccountPath, []byte(`{"type":"service_account"}`), 0o600))

	gcsExternalAccountPath := filepath.Join(testDir, "gcs-external-account.json")
	require.NoError(t, os.WriteFile(gcsExternalAccountPath, []byte(`{"type":"external_account"}`), 0o600))

	dependencyExperiments := experiment.NewExperiments()
	require.NoError(t, dependencyExperiments.EnableExperiment(experiment.DependencyFetchOutputFromState))

	azureExperiments := experiment.NewExperiments()
	require.NoError(t, azureExperiments.EnableExperiment(experiment.DependencyFetchOutputFromState))
	require.NoError(t, azureExperiments.EnableExperiment(experiment.AzureBackend))

	testCases := []struct {
		backendConfig map[string]any
		env           map[string]string
		name          string
		backendName   string
		experiments   experiment.Experiments
		disabled      bool
		want          bool
	}{
		{
			name:        "S3",
			backendName: s3backend.BackendName,
			experiments: dependencyExperiments,
			want:        true,
		},
		{
			name:          "S3 explicit empty workspace prefix remains supported",
			backendName:   s3backend.BackendName,
			backendConfig: map[string]any{"workspace_key_prefix": ""},
			experiments:   dependencyExperiments,
			want:          true,
		},
		{
			name:          "S3 invalid workspace prefix falls back",
			backendName:   s3backend.BackendName,
			backendConfig: map[string]any{"workspace_key_prefix": "/workspaces"},
			experiments:   dependencyExperiments,
		},
		{
			name:        "GCS",
			backendName: gcsbackend.BackendName,
			experiments: dependencyExperiments,
			want:        true,
		},
		{
			name:          "GCS custom endpoint falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"storage_custom_endpoint": "https://storage.example.com"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS unsupported legacy path falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"path": "legacy/state.tfstate"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS empty legacy path still falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"path": ""},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS unknown config falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"typo": "value"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS invalid bucket type falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"bucket": 42},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS inline credentials fall back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"credentials": `{"type":"service_account"}`},
			experiments:   dependencyExperiments,
		},
		{
			name:        "GCS backend credentials environment falls back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"GOOGLE_BACKEND_CREDENTIALS": "credentials.json"},
			experiments: dependencyExperiments,
		},
		{
			name:        "GCS generic credentials environment falls back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"GOOGLE_CREDENTIALS": "/credentials.json"},
			experiments: dependencyExperiments,
		},
		{
			name:          "GCS relative configured credentials fall back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"credentials": "credentials.json"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS absolute configured credentials remain supported",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"credentials": gcsServiceAccountPath},
			experiments:   dependencyExperiments,
			want:          true,
		},
		{
			name:          "GCS missing absolute credentials file falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"credentials": filepath.Join(testDir, "missing-credentials.json")},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS external account credentials fall back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"credentials": gcsExternalAccountPath},
			experiments:   dependencyExperiments,
		},
		{
			name:        "GCS external account ADC falls back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": gcsExternalAccountPath},
			experiments: dependencyExperiments,
		},
		{
			name:        "GCS relative ADC credentials fall back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": "credentials.json"},
			experiments: dependencyExperiments,
		},
		{
			name:          "GCS impersonation falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"impersonate_service_account": "state@example.com"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS access token and ADC conflict falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"access_token": "token"},
			env:           map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": "credentials.json"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS empty access token suppressing environment falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"access_token": ""},
			env:           map[string]string{"GOOGLE_OAUTH_ACCESS_TOKEN": "token"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS empty encryption key suppressing environment falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"encryption_key": ""},
			env:           map[string]string{"GOOGLE_ENCRYPTION_KEY": "key"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS relative encryption key path falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"encryption_key": "keys/gcs.key"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS home-relative encryption key path falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"encryption_key": "~/gcs.key"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS absolute encryption key path remains supported",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"encryption_key": "/keys/gcs.key"},
			experiments:   dependencyExperiments,
			want:          true,
		},
		{
			name:          "GCS strict base64 encryption key remains supported",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"encryption_key": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))},
			experiments:   dependencyExperiments,
			want:          true,
		},
		{
			name:        "GCS emulator environment falls back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"STORAGE_EMULATOR_HOST": "http://127.0.0.1:14443"},
			experiments: dependencyExperiments,
		},
		{
			name:        "GCS changed ADC home falls back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"HOME": "/dependency-home-not-process-home"},
			experiments: dependencyExperiments,
		},
		{
			name:        "GCS changed external AWS credential environment falls back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"AWS_ACCESS_KEY_ID": "dependency-access-key"},
			experiments: dependencyExperiments,
		},
		{
			name:        "GCS changed metadata host falls back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"GCE_METADATA_HOST": "127.0.0.1:18080"},
			experiments: dependencyExperiments,
		},
		{
			name:        "GCS executable external account environment falls back",
			backendName: gcsbackend.BackendName,
			env:         map[string]string{"GOOGLE_EXTERNAL_ACCOUNT_ALLOW_EXECUTABLES": "1"},
			experiments: dependencyExperiments,
		},
		{
			name:        "GCS environment credentials and ADC conflict falls back",
			backendName: gcsbackend.BackendName,
			env: map[string]string{
				"GOOGLE_APPLICATION_CREDENTIALS": "adc.json",
				"GOOGLE_CREDENTIALS":             "backend.json",
			},
			experiments: dependencyExperiments,
		},
		{
			name:          "GCS CSEK and CMEK conflict falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"encryption_key": "csek", "kms_encryption_key": "cmek"},
			experiments:   dependencyExperiments,
		},
		{
			name:          "GCS configured CSEK and environment CMEK conflict falls back",
			backendName:   gcsbackend.BackendName,
			backendConfig: map[string]any{"encryption_key": "csek"},
			env:           map[string]string{"GOOGLE_KMS_ENCRYPTION_KEY": "cmek"},
			experiments:   dependencyExperiments,
		},
		{
			name:        "Azure requires its backend experiment",
			backendName: azurermbackend.BackendName,
			experiments: dependencyExperiments,
		},
		{
			name:        "Azure with both experiments",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"access_key":           "key",
				"container_name":       "state",
				"key":                  "state.tfstate",
				"storage_account_name": "stateaccount",
			},
			experiments: azureExperiments,
			want:        true,
		},
		{
			name:        "Azure service principal remains supported",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"resource_group_name":  "state-group",
				"storage_account_name": "stateaccount",
			},
			env: map[string]string{
				"ARM_CLIENT_ID":       "client",
				"ARM_CLIENT_SECRET":   "secret",
				"ARM_SUBSCRIPTION_ID": "subscription",
				"ARM_TENANT_ID":       "tenant",
			},
			experiments: azureExperiments,
			want:        true,
		},
		{
			name:        "Azure default credential chain falls back",
			backendName: azurermbackend.BackendName,
			experiments: azureExperiments,
		},
		{
			name:          "Azure customer provided key config falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"customer_provided_key": "secret"},
			experiments:   azureExperiments,
		},
		{
			name:          "Azure whitespace customer provided key config falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"customer_provided_key": " "},
			experiments:   azureExperiments,
		},
		{
			name:        "Azure customer provided key environment falls back",
			backendName: azurermbackend.BackendName,
			env:         map[string]string{"ARM_CUSTOMER_PROVIDED_KEY": "secret"},
			experiments: azureExperiments,
		},
		{
			name:        "Azure whitespace customer provided key environment falls back",
			backendName: azurermbackend.BackendName,
			env:         map[string]string{"ARM_CUSTOMER_PROVIDED_KEY": " "},
			experiments: azureExperiments,
		},
		{
			name:          "Azure disabled CLI config falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"use_cli": false},
			experiments:   azureExperiments,
		},
		{
			name:          "Azure metadata host config falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"metadata_host": "https://metadata.example.com"},
			experiments:   azureExperiments,
		},
		{
			name:        "Azure empty metadata host config preserves native validation",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"access_key":           "key",
				"container_name":       "state",
				"environment":          "",
				"key":                  "state.tfstate",
				"metadata_host":        "",
				"storage_account_name": "stateaccount",
			},
			experiments: azureExperiments,
		},
		{
			name:          "Azure AKS workload identity config falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"use_aks_workload_identity": true},
			experiments:   azureExperiments,
		},
		{
			name:        "Azure client ID file environment falls back",
			backendName: azurermbackend.BackendName,
			env:         map[string]string{"ARM_CLIENT_ID_FILE_PATH": "client-id.txt"},
			experiments: azureExperiments,
		},
		{
			name:          "Azure competing access key and SAS token fall back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"access_key": "key", "sas_token": "token"},
			experiments:   azureExperiments,
		},
		{
			name:        "Azure request URL without explicit OIDC falls back",
			backendName: azurermbackend.BackendName,
			env: map[string]string{
				"ARM_OIDC_REQUEST_TOKEN": "token",
				"ARM_OIDC_REQUEST_URL":   "https://pipelines.example.com/token",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure explicitly enabled request URL OIDC falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"resource_group_name": "state-group",
			},
			env: map[string]string{
				"ARM_CLIENT_ID":          "client",
				"ARM_OIDC_REQUEST_TOKEN": "token",
				"ARM_OIDC_REQUEST_URL":   "https://pipelines.example.com/token",
				"ARM_SUBSCRIPTION_ID":    "subscription",
				"ARM_TENANT_ID":          "tenant",
				"ARM_USE_OIDC":           "true",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure explicitly enabled token file OIDC remains supported",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"resource_group_name":  "state-group",
				"storage_account_name": "stateaccount",
				"use_oidc":             true,
			},
			env: map[string]string{
				"ARM_CLIENT_ID":            "client",
				"ARM_OIDC_TOKEN_FILE_PATH": oidcTokenPath,
				"ARM_SUBSCRIPTION_ID":      "subscription",
				"ARM_TENANT_ID":            "tenant",
			},
			experiments: azureExperiments,
			want:        true,
		},
		{
			name:        "Azure OIDC Kubernetes token proxy falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"resource_group_name":  "state-group",
				"storage_account_name": "stateaccount",
				"use_oidc":             true,
			},
			env: map[string]string{
				"ARM_CLIENT_ID":                "client",
				"ARM_OIDC_TOKEN_FILE_PATH":     oidcTokenPath,
				"ARM_SUBSCRIPTION_ID":          "subscription",
				"ARM_TENANT_ID":                "tenant",
				"AZURE_KUBERNETES_TOKEN_PROXY": "https://proxy.example.com/token",
			},
			experiments: azureExperiments,
		},
		{
			name:          "Azure empty request URL config falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"access_key": "key", "oidc_request_url": ""},
			experiments:   azureExperiments,
		},
		{
			name:        "Azure token file without explicit OIDC falls back",
			backendName: azurermbackend.BackendName,
			env: map[string]string{
				"ARM_OIDC_TOKEN_FILE_PATH": "/token",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure OIDC without a token source falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"storage_account_name": "stateaccount",
				"use_oidc":             true,
			},
			env: map[string]string{
				"ARM_CLIENT_ID": "client",
				"ARM_TENANT_ID": "tenant",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure OIDC token file without tenant falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"storage_account_name": "stateaccount",
				"use_oidc":             true,
			},
			env: map[string]string{
				"ARM_CLIENT_ID":            "client",
				"ARM_OIDC_TOKEN_FILE_PATH": "/token",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure missing OIDC token file falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"storage_account_name": "stateaccount",
				"use_oidc":             true,
			},
			env: map[string]string{
				"ARM_CLIENT_ID":            "client",
				"ARM_OIDC_TOKEN_FILE_PATH": filepath.Join(t.TempDir(), "missing-token"),
				"ARM_TENANT_ID":            "tenant",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure competing MSI and OIDC fall back",
			backendName: azurermbackend.BackendName,
			env: map[string]string{
				"ARM_USE_MSI":  "true",
				"ARM_USE_OIDC": "true",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure managed identity endpoint environment falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"resource_group_name":  "state-group",
				"storage_account_name": "stateaccount",
			},
			env: map[string]string{
				"ARM_SUBSCRIPTION_ID": "subscription",
				"ARM_USE_MSI":         "true",
				"IDENTITY_ENDPOINT":   "http://127.0.0.1/metadata/identity/oauth2/token",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure empty managed identity endpoint environment falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"resource_group_name":  "state-group",
				"storage_account_name": "stateaccount",
			},
			env: map[string]string{
				"ARM_SUBSCRIPTION_ID": "subscription",
				"ARM_USE_MSI":         "true",
				"IDENTITY_ENDPOINT":   "",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure regional authority environment falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"resource_group_name":  "state-group",
				"storage_account_name": "stateaccount",
			},
			env: map[string]string{
				"ARM_CLIENT_ID":                 "client",
				"ARM_CLIENT_SECRET":             "secret",
				"ARM_SUBSCRIPTION_ID":           "subscription",
				"ARM_TENANT_ID":                 "tenant",
				"AZURE_REGIONAL_AUTHORITY_NAME": "westus2",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure changed proxy environment falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"access_key":           "key",
				"container_name":       "state",
				"key":                  "state.tfstate",
				"storage_account_name": "stateaccount",
			},
			env:         map[string]string{"HTTPS_PROXY": "http://127.0.0.1:18443"},
			experiments: azureExperiments,
		},
		{
			name:        "Azure empty regional authority environment falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"resource_group_name":  "state-group",
				"storage_account_name": "stateaccount",
			},
			env: map[string]string{
				"ARM_CLIENT_ID":                 "client",
				"ARM_CLIENT_SECRET":             "secret",
				"ARM_SUBSCRIPTION_ID":           "subscription",
				"ARM_TENANT_ID":                 "tenant",
				"AZURE_REGIONAL_AUTHORITY_NAME": "",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure helper-only credential alias falls back",
			backendName: azurermbackend.BackendName,
			env: map[string]string{
				"AZURE_STORAGE_KEY": "key",
			},
			experiments: azureExperiments,
		},
		{
			name:          "Azure padded state key falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"access_key": "key", "key": " state.tfstate"},
			experiments:   azureExperiments,
		},
		{
			name:          "Azure unknown config falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"access_key": "key", "typo": "value"},
			experiments:   azureExperiments,
		},
		{
			name:        "Azure MSI resource ID environment falls back",
			backendName: azurermbackend.BackendName,
			env:         map[string]string{"ARM_MSI_RESOURCE_ID": "/subscriptions/example/identity"},
			experiments: azureExperiments,
		},
		{
			name:          "Azure unsupported cloud alias falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"access_key": "key", "environment": "global"},
			experiments:   azureExperiments,
		},
		{
			name:        "Azure service principal without ARM lookup fields falls back",
			backendName: azurermbackend.BackendName,
			env: map[string]string{
				"ARM_CLIENT_ID":     "client",
				"ARM_CLIENT_SECRET": "secret",
				"ARM_TENANT_ID":     "tenant",
			},
			experiments: azureExperiments,
		},
		{
			name:        "Azure Entra state auth does not require ARM lookup fields",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"container_name":       "state",
				"key":                  "state.tfstate",
				"storage_account_name": "stateaccount",
				"use_azuread_auth":     true,
			},
			env: map[string]string{
				"ARM_CLIENT_ID":     "client",
				"ARM_CLIENT_SECRET": "secret",
				"ARM_TENANT_ID":     "tenant",
			},
			experiments: azureExperiments,
			want:        true,
		},
		{
			name:        "Azure DNS zone endpoint environment falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"access_key":           "key",
				"container_name":       "state",
				"key":                  "state.tfstate",
				"storage_account_name": "stateaccount",
			},
			env:         map[string]string{"ARM_USE_DNS_ZONE_ENDPOINT": "true"},
			experiments: azureExperiments,
		},
		{
			name:        "Azure federated token alias falls back for Terraform compatibility",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"access_key":           "key",
				"container_name":       "state",
				"key":                  "state.tfstate",
				"storage_account_name": "stateaccount",
			},
			env:         map[string]string{"AZURE_FEDERATED_TOKEN_FILE": "/token"},
			experiments: azureExperiments,
		},
		{
			name:          "Azure uppercase storage account falls back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"access_key": "key", "container_name": "state", "storage_account_name": "StateAccount"},
			experiments:   azureExperiments,
		},
		{
			name:          "Azure invalid container hyphens fall back",
			backendName:   azurermbackend.BackendName,
			backendConfig: map[string]any{"access_key": "key", "container_name": "state--data", "storage_account_name": "stateaccount"},
			experiments:   azureExperiments,
		},
		{
			name:        "Azure token file and request URL conflict falls back",
			backendName: azurermbackend.BackendName,
			backendConfig: map[string]any{
				"resource_group_name": "state-group",
				"use_oidc":            true,
			},
			env: map[string]string{
				"ARM_CLIENT_ID":            "client",
				"ARM_OIDC_REQUEST_TOKEN":   "token",
				"ARM_OIDC_REQUEST_URL":     "https://pipelines.example.com/token",
				"ARM_OIDC_TOKEN_FILE_PATH": "/token",
				"ARM_SUBSCRIPTION_ID":      "subscription",
				"ARM_TENANT_ID":            "tenant",
			},
			experiments: azureExperiments,
		},
		{
			name:        "unsupported backend",
			backendName: "local",
			experiments: dependencyExperiments,
		},
		{
			name:        "explicit opt out",
			backendName: gcsbackend.BackendName,
			experiments: dependencyExperiments,
			disabled:    true,
		},
		{
			name:        "dependency experiment disabled",
			backendName: gcsbackend.BackendName,
			experiments: experiment.NewExperiments(),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			env := testCase.env
			if env == nil {
				env = map[string]string{}
			}

			pctx := &ParsingContext{
				Experiments:                      testCase.experiments,
				NoDependencyFetchOutputFromState: testCase.disabled,
				Venv:                             venv.OSVenv().WithEnv(env),
			}

			remoteState := remotestate.New(&remotestate.Config{
				BackendName:   testCase.backendName,
				BackendConfig: testCase.backendConfig,
			})
			assert.Equal(t, testCase.want, shouldFetchDependencyOutputFromState(pctx, remoteState))
		})
	}

	assert.False(t, shouldFetchDependencyOutputFromState(&ParsingContext{}, nil))
}

func TestGCSDirectStateReadFallsBackWhenExecutableEnvironmentIsCleared(t *testing.T) {
	const executableEnv = "GOOGLE_EXTERNAL_ACCOUNT_ALLOW_EXECUTABLES"

	t.Setenv(executableEnv, "1")

	experiments := experiment.NewExperiments()
	require.NoError(t, experiments.EnableExperiment(experiment.DependencyFetchOutputFromState))

	pctx := &ParsingContext{
		Experiments: experiments,
		Venv:        venv.OSVenv().WithEnv(map[string]string{executableEnv: ""}),
	}
	remoteState := remotestate.New(&remotestate.Config{BackendName: gcsbackend.BackendName})

	assert.False(t, shouldFetchDependencyOutputFromState(pctx, remoteState))
}

func TestTerraformStateOutputsJSON(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		state     string
		want      string
		wantErr   string
		wantExact bool
	}{
		{
			name:  "outputs",
			state: `{"version":4,"outputs":{"answer":{"sensitive":false,"type":"number","value":42}}}`,
			want:  `{"answer":{"sensitive":false,"type":"number","value":42}}`,
		},
		{
			name:      "large integers retain their exact value",
			state:     `{"version":4,"outputs":{"answer":{"sensitive":false,"type":"number","value":9007199254740993}}}`,
			want:      `{"answer":{"sensitive":false,"type":"number","value":9007199254740993}}`,
			wantExact: true,
		},
		{
			name:  "empty outputs",
			state: `{"version":4,"outputs":{}}`,
			want:  `{}`,
		},
		{
			name:  "missing outputs preserves existing null contract",
			state: `{"version":4}`,
			want:  `null`,
		},
		{
			name:    "malformed state",
			state:   `{"version":`,
			wantErr: "parsing dependency state JSON from gs://bucket/state",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := terraformStateOutputsJSON([]byte(testCase.state), "gs://bucket/state")
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)

				return
			}

			require.NoError(t, err)

			if testCase.wantExact {
				assert.Equal(t, testCase.want, string(got))

				return
			}

			assert.JSONEq(t, testCase.want, string(got))
		})
	}
}

func TestIsRemoteStateMissing(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		err  error
		name string
		want bool
	}{
		{
			name: "S3 object",
			err:  fmt.Errorf("fetching S3 state: %w", &smithy.GenericAPIError{Code: "NoSuchKey"}),
			want: true,
		},
		{
			name: "GCS object",
			err:  fmt.Errorf("fetching GCS state: %w", storage.ErrObjectNotExist),
			want: true,
		},
		{
			name: "GCS bucket",
			err:  fmt.Errorf("fetching GCS state: %w", storage.ErrBucketNotExist),
			want: true,
		},
		{
			name: "Azure blob",
			err: fmt.Errorf("fetching Azure state: %w", &azcore.ResponseError{
				StatusCode: http.StatusNotFound,
				ErrorCode:  "BlobNotFound",
			}),
			want: true,
		},
		{
			name: "permission error",
			err: fmt.Errorf("fetching Azure state: %w", &azcore.ResponseError{
				StatusCode: http.StatusForbidden,
				ErrorCode:  "AuthorizationFailure",
			}),
		},
		// An ARM 404 means a misconfigured coordinate, not a state blob that is absent.
		{
			name: "Azure ARM resource group not found is a setup failure",
			err: fmt.Errorf("%w: resolving storage account key: %w",
				azurermbackend.ErrStateClientSetup,
				&azcore.ResponseError{StatusCode: http.StatusNotFound, ErrorCode: "ResourceGroupNotFound"}),
		},
		{
			name: "Azure ARM subscription not found is a setup failure",
			err: fmt.Errorf("%w: resolving storage account key: %w",
				azurermbackend.ErrStateClientSetup,
				&azcore.ResponseError{StatusCode: http.StatusNotFound, ErrorCode: "SubscriptionNotFound"}),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, isRemoteStateMissing(testCase.err))
		})
	}
}

func TestGCSStateObjectKey(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		workspace string
		want      string
		config    gcsbackend.RemoteStateConfigGCS
	}{
		{
			name:      "default prefix",
			workspace: defaultStateWorkspace,
			want:      "default.tfstate",
		},
		{
			name:      "nested prefix",
			workspace: defaultStateWorkspace,
			config:    gcsbackend.RemoteStateConfigGCS{Prefix: "environment/service"},
			want:      "environment/service/default.tfstate",
		},
		{
			name:      "leading slash is normalized",
			workspace: defaultStateWorkspace,
			config:    gcsbackend.RemoteStateConfigGCS{Prefix: "/environment/service"},
			want:      "environment/service/default.tfstate",
		},
		{
			name:      "named workspace",
			workspace: "production",
			config:    gcsbackend.RemoteStateConfigGCS{Prefix: "environment/service"},
			want:      "environment/service/production.tfstate",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, testCase.want, gcsStateObjectKey(&testCase.config, testCase.workspace))
		})
	}
}

func TestWorkspaceStateKeys(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "state.tfstate", s3StateObjectKey(backend.Config{"key": "state.tfstate"}, defaultStateWorkspace))
	assert.Equal(t, "env:/production/state.tfstate", s3StateObjectKey(backend.Config{"key": "state.tfstate"}, "production"))
	assert.Equal(t, "workspaces/production/state.tfstate", s3StateObjectKey(
		backend.Config{"key": "state.tfstate", "workspace_key_prefix": "workspaces"},
		"production",
	))
	assert.Equal(t, "production/state.tfstate", s3StateObjectKey(
		backend.Config{"key": "state.tfstate", "workspace_key_prefix": ""},
		"production",
	))
	assert.Equal(t, "state.tfstate", azurermStateBlobKey("state.tfstate", defaultStateWorkspace))
	assert.Equal(t, "state.tfstateenv:production", azurermStateBlobKey("state.tfstate", "production"))
}

func TestDependencyStateWorkspace(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		env           map[string]string
		name          string
		persisted     string
		persistedPath string
		want          string
		wantErr       string
	}{
		{name: "defaults when no selection exists", want: defaultStateWorkspace},
		{name: "persisted workspace", persisted: "staging\n", want: "staging"},
		{
			name:      "TF_WORKSPACE overrides persisted workspace",
			env:       map[string]string{"TF_WORKSPACE": "production"},
			persisted: "staging",
			want:      "production",
		},
		{
			name:      "empty TF_WORKSPACE uses persisted workspace",
			env:       map[string]string{"TF_WORKSPACE": ""},
			persisted: "staging",
			want:      "staging",
		},
		{
			name:          "custom relative TF_DATA_DIR",
			env:           map[string]string{"TF_DATA_DIR": ".tofu-data"},
			persisted:     "production",
			persistedPath: ".tofu-data",
			want:          "production",
		},
		{
			name:          "custom absolute TF_DATA_DIR",
			env:           map[string]string{"TF_DATA_DIR": "/tofu-data"},
			persisted:     "production",
			persistedPath: "/tofu-data",
			want:          "production",
		},
		{
			name:      "persisted workspace is not path validated",
			persisted: "invalid/workspace",
			want:      "invalid/workspace",
		},
		{
			name:    "invalid TF_WORKSPACE falls back",
			env:     map[string]string{"TF_WORKSPACE": "invalid/workspace"},
			wantErr: "invalid TF_WORKSPACE",
		},
		{
			name: "empty TF_DATA_DIR uses default data directory",
			env:  map[string]string{"TF_DATA_DIR": ""},
			want: defaultStateWorkspace,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			const workingDir = "/workspace"

			env := testCase.env
			if env == nil {
				env = map[string]string{}
			}

			v := venvtest.New().WithEnv(env)

			if testCase.persisted != "" {
				dataDir := testCase.persistedPath
				if dataDir == "" {
					dataDir = tf.DefaultTFDataDir
				}

				workspaceDir := dataDir
				if !filepath.IsAbs(workspaceDir) {
					workspaceDir = filepath.Join(workingDir, workspaceDir)
				}

				require.NoError(t, v.FS.MkdirAll(workspaceDir, 0o700))
				require.NoError(t, vfs.WriteFile(
					v.FS,
					filepath.Join(workspaceDir, "environment"),
					[]byte(testCase.persisted),
					0o600,
				))
			}

			got, err := dependencyStateWorkspace(&ParsingContext{Venv: v}, workingDir)
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, testCase.want, got)
		})
	}
}

func TestGetTerragruntOutputJSONFromRemoteStateGCS(t *testing.T) {
	t.Parallel()

	const (
		bucket = "state-bucket"
		prefix = "environment/service"
	)

	state := []byte(`{"version":4,"outputs":{"producer_value":{"sensitive":false,"type":"string","value":"from-gcs"}}}`)
	encryptionKey := bytes.Repeat([]byte{0x2a}, 32)
	encodedEncryptionKey := base64.StdEncoding.EncodeToString(encryptionKey)

	var (
		requestPath   string
		requestHeader http.Header
	)

	v := venvtest.New()
	v = v.WithHTTP(vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		requestPath = req.URL.Path
		requestHeader = req.Header.Clone()

		return vhttp.Respond(http.StatusOK, state, nil), nil
	}))

	keyPath := filepath.Join("/config", "gcs.key")
	require.NoError(t, vfs.WriteFile(v.FS, keyPath, []byte(encodedEncryptionKey), 0o600))

	pctx := &ParsingContext{
		TerragruntConfigPath: filepath.Join("/config", DefaultTerragruntConfigPath),
		Venv:                 v,
	}
	remoteState := remotestate.New(&remotestate.Config{
		BackendName: gcsbackend.BackendName,
		BackendConfig: map[string]any{
			"access_token":   "test-token",
			"bucket":         bucket,
			"encryption_key": keyPath,
			"prefix":         prefix,
		},
	})

	got, err := getTerragruntOutputJSONFromRemoteStateGCS(
		t.Context(),
		logger.CreateLogger(),
		pctx,
		remoteState,
		defaultStateWorkspace,
	)
	require.NoError(t, err)
	assert.JSONEq(t, `{"producer_value":{"sensitive":false,"type":"string","value":"from-gcs"}}`, string(got))

	assert.Contains(t, requestPath, bucket)
	assert.Contains(t, requestPath, "environment/service/default.tfstate")
	require.NotNil(t, requestHeader)
	assert.Equal(t, "Bearer test-token", requestHeader.Get("Authorization"))
	assert.Equal(t, "AES256", requestHeader.Get("X-Goog-Encryption-Algorithm"))
	assert.Equal(t, encodedEncryptionKey, requestHeader.Get("X-Goog-Encryption-Key"))
	assert.NotEmpty(t, requestHeader.Get("X-Goog-Encryption-Key-Sha256"))
}

func TestGCSObjectWithEncryptionKeyRejectsInvalidKeys(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		key     string
		wantErr string
	}{
		{
			name:    "invalid base64",
			key:     "not-base64",
			wantErr: "decoding encryption_key as base64",
		},
		{
			name:    "wrong decoded length",
			key:     base64.StdEncoding.EncodeToString([]byte("short")),
			wantErr: "expected 32 bytes, got 5",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			pctx := &ParsingContext{
				TerragruntConfigPath: filepath.Join("/config", DefaultTerragruntConfigPath),
				Venv:                 venvtest.New(),
			}

			_, err := gcsObjectWithEncryptionKey(pctx, new(storage.ObjectHandle), testCase.key)
			require.ErrorContains(t, err, testCase.wantErr)
			assert.NotContains(t, err.Error(), testCase.key, "error must not expose key material")
		})
	}
}
