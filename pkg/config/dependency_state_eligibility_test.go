package config_test

import (
	"encoding/base64"
	"fmt"
	"maps"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dependencyStateEligibilityTestCase struct {
	name                   string
	backend                string
	backendConfig          map[string]string
	env                    map[string]string
	files                  map[string]string
	filesystem             vfs.FS
	producerTerraformExtra string
	wantRequest            string
	enableAzure            bool
	optOut                 bool
	wantDirect             bool
}

func TestDependencyStateEligibilityRoutesSafely(t *testing.T) {
	t.Parallel()

	azureAccessKey := base64.StdEncoding.EncodeToString(make([]byte, 32))
	s3Config := eligibilityS3Config()
	gcsConfig := eligibilityGCSConfig()
	azureConfig := eligibilityAzureConfig(azureAccessKey)

	testCases := []dependencyStateEligibilityTestCase{
		{
			name:          "S3 explicit empty workspace prefix remains direct",
			backend:       "s3",
			backendConfig: eligibilityConfig(s3Config, map[string]string{"workspace_key_prefix": `""`}),
			env: map[string]string{
				"AWS_ACCESS_KEY_ID":     "test-access-key",
				"AWS_SECRET_ACCESS_KEY": "test-secret-key",
				"TF_WORKSPACE":          "production",
			},
			wantRequest: "s3.example.com/state-bucket/production/service.tfstate",
			wantDirect:  true,
		},
		{
			name:          "S3 SSE-C config falls back",
			backend:       "s3",
			backendConfig: eligibilityConfig(s3Config, map[string]string{"sse_customer_key": `"c2VjcmV0"`}),
		},
		{
			name:          "S3 SSE-C environment falls back",
			backend:       "s3",
			backendConfig: s3Config,
			env:           map[string]string{"AWS_SSE_CUSTOMER_KEY": "c2VjcmV0"},
		},
		{
			name:          "S3 invalid workspace prefix type falls back",
			backend:       "s3",
			backendConfig: eligibilityConfig(s3Config, map[string]string{"workspace_key_prefix": "42"}),
		},
		{
			name:          "GCS empty legacy path remains native",
			backend:       "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{"path": `""`}),
		},
		{
			name:          "GCS unknown backend key falls back",
			backend:       "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{"typo": `"value"`}),
		},
		{
			name:          "GCS invalid bucket type falls back",
			backend:       "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{"bucket": "42"}),
		},
		{
			name:          "GCS inline credentials fall back",
			backend:       "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{"credentials": `jsonencode({ type = "service_account" })`}),
		},
		{
			name:          "GCS relative credential path falls back",
			backend:       "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{"credentials": `"credentials.json"`}),
		},
		{
			name:          "GCS external account credential file falls back",
			backend:       "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{"credentials": `"/credentials/external.json"`}),
			files:         map[string]string{"/credentials/external.json": `{"type":"external_account"}`},
		},
		{
			name:          "GCS access token and ADC conflict falls back",
			backend:       "gcs",
			backendConfig: gcsConfig,
			env:           map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": "/credentials/service-account.json"},
			files:         map[string]string{"/credentials/service-account.json": `{"type":"service_account"}`},
		},
		{
			name:    "GCS environment credentials and ADC conflict falls back",
			backend: "gcs",
			backendConfig: eligibilityConfig(
				gcsConfig,
				map[string]string{},
				"access_token",
			),
			env: map[string]string{
				"GOOGLE_APPLICATION_CREDENTIALS": "/credentials/service-account.json",
				"GOOGLE_CREDENTIALS":             "/credentials/backend.json",
			},
			files: map[string]string{
				"/credentials/service-account.json": `{"type":"service_account"}`,
				"/credentials/backend.json":         `{"type":"service_account"}`,
			},
		},
		{
			name:    "GCS empty access token suppresses environment token",
			backend: "gcs",
			backendConfig: eligibilityConfig(
				gcsConfig,
				map[string]string{"access_token": `""`},
			),
			env: map[string]string{"GOOGLE_OAUTH_ACCESS_TOKEN": "environment-token"},
		},
		{
			name:          "GCS impersonation remains native",
			backend:       "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{"impersonate_service_account": `"state@example.com"`}),
		},
		{
			name:    "GCS CSEK and CMEK conflict falls back",
			backend: "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{
				"encryption_key":     `"csek"`,
				"kms_encryption_key": `"projects/test/locations/global/keyRings/state/cryptoKeys/state"`,
			}),
		},
		{
			name:    "GCS strict base64 encryption key remains direct",
			backend: "gcs",
			backendConfig: eligibilityConfig(gcsConfig, map[string]string{
				"encryption_key": fmt.Sprintf("%q", base64.StdEncoding.EncodeToString(make([]byte, 32))),
			}),
			wantRequest: "storage.googleapis.com/state-bucket/environment/service/default.tfstate",
			wantDirect:  true,
		},
		{
			name:          "GCS explicit optimization opt-out falls back",
			backend:       "gcs",
			backendConfig: gcsConfig,
			optOut:        true,
		},
		{
			name:          "GCS invalid explicit workspace falls back",
			backend:       "gcs",
			backendConfig: gcsConfig,
			env:           map[string]string{"TF_WORKSPACE": "invalid/workspace"},
		},
		{
			name:          "GCS dot workspace falls back",
			backend:       "gcs",
			backendConfig: gcsConfig,
			env:           map[string]string{"TF_WORKSPACE": "."},
		},
		{
			name:          "GCS parent-directory workspace falls back",
			backend:       "gcs",
			backendConfig: gcsConfig,
			env:           map[string]string{"TF_WORKSPACE": ".."},
		},
		{
			name:          "GCS inherited HOME remains direct",
			backend:       "gcs",
			backendConfig: gcsConfig,
			env:           map[string]string{"HOME": "/home/user"},
			wantRequest:   "storage.googleapis.com/state-bucket/environment/service/default.tfstate",
			wantDirect:    true,
		},
		{
			name:          "GCS output proxy override falls back",
			backend:       "gcs",
			backendConfig: gcsConfig,
			env: map[string]string{
				"HTTPS_PROXY": "http://inherited.example.com",
			},
			producerTerraformExtra: `terraform {
  extra_arguments "output_proxy" {
    commands = ["output"]
    env_vars = {
      HTTPS_PROXY = "http://override.example.com"
    }
  }
}

`,
		},
		{
			name:          "Azure requires backend experiment",
			backend:       "azurerm",
			backendConfig: azureConfig,
		},
		{
			name:          "Azure snapshot boolean remains direct",
			backend:       "azurerm",
			backendConfig: eligibilityConfig(azureConfig, map[string]string{"snapshot": "true"}),
			enableAzure:   true,
			wantRequest:   "stateaccount.blob.core.windows.net/state/service.tfstate",
			wantDirect:    true,
		},
		{
			name:          "Azure invalid snapshot boolean falls back",
			backend:       "azurerm",
			backendConfig: eligibilityConfig(azureConfig, map[string]string{"snapshot": `"not-a-bool"`}),
			enableAzure:   true,
		},
		{
			name:          "Azure invalid OIDC boolean falls back",
			backend:       "azurerm",
			backendConfig: eligibilityConfig(azureConfig, map[string]string{"use_oidc": `"not-a-bool"`}),
			enableAzure:   true,
		},
		{
			name:          "Azure disabled CLI falls back",
			backend:       "azurerm",
			backendConfig: eligibilityConfig(azureConfig, map[string]string{"use_cli": "false"}),
			enableAzure:   true,
		},
		{
			name:          "Azure inherited HTTPS_PROXY remains direct",
			backend:       "azurerm",
			backendConfig: azureConfig,
			env:           map[string]string{"HTTPS_PROXY": "http://proxy.example.com"},
			enableAzure:   true,
			wantRequest:   "stateaccount.blob.core.windows.net/state/service.tfstate",
			wantDirect:    true,
		},
		{
			name:          "Azure output proxy override falls back",
			backend:       "azurerm",
			backendConfig: azureConfig,
			env:           map[string]string{"HTTPS_PROXY": "http://inherited.example.com"},
			enableAzure:   true,
			producerTerraformExtra: `terraform {
  extra_arguments "output_proxy" {
    commands = ["output"]
    env_vars = {
      HTTPS_PROXY = "http://override.example.com"
    }
  }
}

`,
		},
		{
			name:    "Azure competing access key and SAS token fall back",
			backend: "azurerm",
			backendConfig: eligibilityConfig(azureConfig, map[string]string{
				"sas_token": `"?sig=test"`,
			}),
			enableAzure: true,
		},
		{
			name:    "Azure competing MSI and OIDC fall back",
			backend: "azurerm",
			backendConfig: eligibilityConfig(
				azureConfig,
				map[string]string{},
				"access_key",
			),
			env: map[string]string{
				"ARM_USE_MSI":  "true",
				"ARM_USE_OIDC": "true",
			},
			enableAzure: true,
		},
		{
			name:    "Azure OIDC without token source falls back",
			backend: "azurerm",
			backendConfig: eligibilityConfig(
				azureConfig,
				map[string]string{
					"client_id": `"client"`,
					"tenant_id": `"tenant"`,
					"use_oidc":  "true",
				},
				"access_key",
			),
			enableAzure: true,
		},
		{
			name:          "Azure customer-provided key environment falls back",
			backend:       "azurerm",
			backendConfig: azureConfig,
			env:           map[string]string{"ARM_CUSTOMER_PROVIDED_KEY": "secret"},
			enableAzure:   true,
		},
		{
			name:          "Azure helper-only credential alias falls back",
			backend:       "azurerm",
			backendConfig: eligibilityConfig(azureConfig, map[string]string{}, "access_key"),
			env:           map[string]string{"AZURE_STORAGE_KEY": azureAccessKey},
			enableAzure:   true,
		},
		{
			name:          "Azure MSI resource ID environment falls back",
			backend:       "azurerm",
			backendConfig: azureConfig,
			env:           map[string]string{"ARM_MSI_RESOURCE_ID": "/subscriptions/example/identity"},
			enableAzure:   true,
		},
		{
			name:          "Azure unsupported cloud alias falls back",
			backend:       "azurerm",
			backendConfig: eligibilityConfig(azureConfig, map[string]string{"environment": `"global"`}),
			enableAzure:   true,
		},
		{
			name:          "Azure padded state key falls back",
			backend:       "azurerm",
			backendConfig: eligibilityConfig(azureConfig, map[string]string{"key": `" service.tfstate"`}),
			enableAzure:   true,
		},
		{
			name:    "Azure incomplete service principal falls back",
			backend: "azurerm",
			backendConfig: eligibilityConfig(
				azureConfig,
				map[string]string{},
				"access_key",
			),
			env: map[string]string{
				"ARM_CLIENT_ID":     "client",
				"ARM_CLIENT_SECRET": "secret",
				"ARM_TENANT_ID":     "tenant",
			},
			enableAzure: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("from-direct"))
			cfg, err := parseDependencyStateEligibilityFixture(t, recorder, &testCase)
			require.NoError(t, err)

			if testCase.wantDirect {
				assert.Equal(t, "from-direct", cfg.Inputs["result"])
				assert.Empty(t, recorder.invocations())
				require.Equal(t, []string{testCase.wantRequest}, recorder.requestPaths())

				return
			}

			assert.Equal(t, "from-native-output", cfg.Inputs["result"])
			assert.Empty(t, recorder.requestPaths())
			assertEligibilityOutputInvocation(t, recorder.invocations())
		})
	}
}

func TestDependencyStateEligibilityUsesPersistedWorkspaceInCustomDataDir(t *testing.T) {
	t.Parallel()

	recorder := newDependencyStateRecorder(t, http.StatusOK, terraformState("from-direct"))
	testCase := dependencyStateEligibilityTestCase{
		backend:       "gcs",
		backendConfig: eligibilityGCSConfig(),
		env:           map[string]string{"TF_DATA_DIR": "/workspace-data"},
		files:         map[string]string{"/workspace-data/environment": "staging\n"},
		wantRequest:   "storage.googleapis.com/state-bucket/environment/service/staging.tfstate",
		wantDirect:    true,
	}

	cfg, err := parseDependencyStateEligibilityFixture(t, recorder, &testCase)
	require.NoError(t, err)
	assert.Equal(t, "from-direct", cfg.Inputs["result"])
	assert.Empty(t, recorder.invocations())
	require.Equal(t, []string{testCase.wantRequest}, recorder.requestPaths())
}

func parseDependencyStateEligibilityFixture(
	t *testing.T,
	recorder *dependencyStateRecorder,
	testCase *dependencyStateEligibilityTestCase,
) (*config.TerragruntConfig, error) {
	t.Helper()

	const (
		consumerPath = "/eligibility/consumer/terragrunt.hcl"
		producerPath = "/eligibility/producer/terragrunt.hcl"
	)

	env := testCase.env
	if env == nil {
		env = map[string]string{}
	}

	effectiveEnv := maps.Clone(env)
	if effectiveEnv == nil {
		effectiveEnv = map[string]string{}
	}

	v := venvtest.New().
		WithEnv(effectiveEnv).
		WithExec(recorder.exec()).
		WithHTTP(recorder.httpClient())

	if testCase.filesystem != nil {
		v = v.WithFS(testCase.filesystem)
	}

	for _, dir := range []string{filepath.Dir(consumerPath), filepath.Dir(producerPath)} {
		require.NoError(t, v.FS.MkdirAll(dir, 0o700))
	}

	for path, contents := range testCase.files {
		require.NoError(t, v.FS.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, vfs.WriteFile(v.FS, path, []byte(contents), 0o600))
	}

	producer := fmt.Sprintf(`%sremote_state {
  backend = %q
  config = {
%s
  }
}
`, testCase.producerTerraformExtra, testCase.backend, eligibilityConfigHCL(testCase.backendConfig))
	consumer := `dependency "producer" {
  config_path = "../producer"
}

inputs = {
  result = dependency.producer.outputs.producer_value
}
`

	require.NoError(t, vfs.WriteFile(v.FS, producerPath, []byte(producer), 0o600))
	require.NoError(t, vfs.WriteFile(v.FS, consumerPath, []byte(consumer), 0o600))

	ctx, pctx := newTestParsingContext(t, v, consumerPath)
	ctx = config.WithConfigValues(ctx)
	pctx.OriginalTerragruntConfigPath = consumerPath
	pctx.NoDependencyFetchOutputFromState = testCase.optOut

	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.DependencyFetchOutputFromState))

	if testCase.enableAzure {
		require.NoError(t, pctx.Experiments.EnableExperiment(experiment.AzureBackend))
	}

	return config.ParseConfigFile(ctx, pctx, logger.CreateLogger(), consumerPath, nil)
}

func eligibilityS3Config() map[string]string {
	return map[string]string{
		"bucket":                      `"state-bucket"`,
		"endpoint":                    `"https://s3.example.com"`,
		"force_path_style":            "true",
		"key":                         `"service.tfstate"`,
		"region":                      `"us-east-1"`,
		"skip_credentials_validation": "true",
	}
}

func eligibilityGCSConfig() map[string]string {
	return map[string]string{
		"access_token": `"test-token"`,
		"bucket":       `"state-bucket"`,
		"prefix":       `"environment/service"`,
	}
}

func eligibilityAzureConfig(accessKey string) map[string]string {
	return map[string]string{
		"access_key":           fmt.Sprintf("%q", accessKey),
		"container_name":       `"state"`,
		"key":                  `"service.tfstate"`,
		"storage_account_name": `"stateaccount"`,
	}
}

func eligibilityConfig(base map[string]string, overrides map[string]string, remove ...string) map[string]string {
	config := maps.Clone(base)
	maps.Copy(config, overrides)

	for _, key := range remove {
		delete(config, key)
	}

	return config
}

func eligibilityConfigHCL(values map[string]string) string {
	keys := slices.Sorted(maps.Keys(values))

	var body strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&body, "    %s = %s\n", key, values[key])
	}

	return body.String()
}

func assertEligibilityOutputInvocation(t *testing.T, invocations []vexec.Invocation) {
	t.Helper()

	require.NotEmpty(t, invocations)
	assert.True(t, slices.ContainsFunc(invocations, func(inv vexec.Invocation) bool {
		return slices.Contains(inv.Args, "output") && slices.Contains(inv.Args, "-json")
	}))
}
