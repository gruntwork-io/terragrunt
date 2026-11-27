package azurehelper_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// Common test fixtures used across azurehelper unit tests.
const (
	testAccount   = "acct"
	testSub       = "sub"
	testSASToken  = "sv=x"
	testPodClient = "pod-client"
	testPodTenant = "pod-tenant"
)

func TestBuild_AuthMethodPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		cfg     azurehelper.AzureSessionConfig
		name    string
		want    azurehelper.AuthMethod
		hasCred bool
	}{
		{
			name: "sas token wins over everything",
			cfg: azurehelper.AzureSessionConfig{
				StorageAccountName: testAccount,
				SasToken:           "sv=2023-01-01&sig=x",
				AccessKey:          "ignored",
				ClientID:           "ignored",
				ClientSecret:       "ignored",
				TenantID:           "ignored",
				SubscriptionID:     testSub,
			},
			want:    azurehelper.AuthMethodSasToken,
			hasCred: false,
		},
		{
			name: "access key wins over Entra and service principal",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID:     testSub,
				StorageAccountName: testAccount,
				AccessKey:          "key",
				ClientID:           "cid",
				ClientSecret:       "sec",
				TenantID:           "tid",
				UseAzureADAuth:     new(true),
			},
			want:    azurehelper.AuthMethodAccessKey,
			hasCred: false,
		},
		{
			name: "service principal when all three set",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: testSub,
				ClientID:       "cid",
				ClientSecret:   "sec",
				TenantID:       "tid",
			},
			want:    azurehelper.AuthMethodServicePrincipal,
			hasCred: true,
		},
		{
			name: "msi when use_msi true",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: testSub,
				UseMSI:         new(true),
			},
			want:    azurehelper.AuthMethodMSI,
			hasCred: true,
		},
		{
			name: "msi beats oidc when both enabled",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID:    testSub,
				TenantID:          "tid",
				ClientID:          "cid",
				OIDCTokenFilePath: "/var/run/secrets/azure/tokens/azure-identity-token",
				UseOIDC:           new(true),
				UseMSI:            new(true),
			},
			want:    azurehelper.AuthMethodMSI,
			hasCred: true,
		},
		{
			name: "azuread default fallback",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: testSub,
				UseAzureADAuth: new(true),
			},
			want:    azurehelper.AuthMethodAzureAD,
			hasCred: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&tc.cfg).
				Build(log.New(), isolatedEnv())
			require.NoError(t, err)
			assert.Equal(t, tc.want, got.Method)

			if tc.hasCred {
				assert.NotNil(t, got.Credential, "method %q must resolve a credential", tc.want)

				return
			}

			assert.Nil(t, got.Credential, "method %q must not resolve a credential", tc.want)
		})
	}
}

func TestBuild_EnvFallbacks(t *testing.T) {
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
		}).
		Build(log.New(), isolatedEnv(
			"ARM_SAS_TOKEN", "sv=test",
			"ARM_SUBSCRIPTION_ID", "sub-from-env",
		))
	require.NoError(t, err)
	assert.Equal(t, azurehelper.AuthMethodSasToken, cfg.Method)
	assert.Equal(t, "sv=test", cfg.SasToken)
}

func TestBuild_CarriesMSIResourceID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		configured string
		env        map[string]string
		want       string
	}{
		{
			name:       "configured resource ID",
			configured: "/subscriptions/configured/resourceGroups/identity/providers/Microsoft.ManagedIdentity/userAssignedIdentities/state",
			want:       "/subscriptions/configured/resourceGroups/identity/providers/Microsoft.ManagedIdentity/userAssignedIdentities/state",
		},
		{
			name: "environment resource ID",
			env: map[string]string{
				"ARM_MSI_RESOURCE_ID": "/subscriptions/environment/resourceGroups/identity/providers/Microsoft.ManagedIdentity/userAssignedIdentities/state",
			},
			want: "/subscriptions/environment/resourceGroups/identity/providers/Microsoft.ManagedIdentity/userAssignedIdentities/state",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			env := testCase.env
			if env == nil {
				env = map[string]string{}
			}

			cfg, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&azurehelper.AzureSessionConfig{
					SubscriptionID: testSub,
					MSIResourceID:  testCase.configured,
					UseMSI:         new(true),
				}).
				Build(log.New(), (&venv.Venv{}).WithEnv(env))
			require.NoError(t, err)
			assert.Equal(t, testCase.want, cfg.MSIResourceID)
		})
	}
}

func TestBuild_CarriesHTTPTransport(t *testing.T) {
	t.Parallel()

	httpClient := vhttp.NewNoNetworkClient()
	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			SasToken:           testSASToken,
		}).
		Build(log.New(), isolatedEnv().WithHTTP(httpClient))
	require.NoError(t, err)
	require.NotNil(t, cfg.ClientOptions.Transport)
	assert.Equal(t, httpClient, cfg.ClientOptions.Transport)
}

func TestBuild_TrimsWhitespaceFromEnvValues(t *testing.T) {
	t.Parallel()
	// CI often injects secrets with a trailing newline; it must be trimmed.
	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{StorageAccountName: testAccount, SasToken: testSASToken}).
		Build(log.New(), isolatedEnv("ARM_SUBSCRIPTION_ID", "sub-from-env\n"))
	require.NoError(t, err)
	assert.Equal(t, "sub-from-env", cfg.SubscriptionID, "env values must be trimmed")
}

func TestBuild_StorageAccountNameEnvFallback(t *testing.T) {
	t.Parallel()
	// SAS auth requires a storage account; supply it only via AZURE_STORAGE_ACCOUNT.
	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{SasToken: testSASToken}).
		Build(log.New(), isolatedEnv("AZURE_STORAGE_ACCOUNT", "acct-from-env"))
	require.NoError(t, err)
	assert.Equal(t, "acct-from-env", cfg.AccountName)
}

// TestBuild_SubscriptionNotRequiredForDataPlane pins that a Blob-only Entra
// config builds without a subscription id. Blob data-plane access needs only
// the account endpoint and a token; requiring a subscription here would reject
// configurations the native azurerm backend accepts.
func TestBuild_SubscriptionNotRequiredForDataPlane(t *testing.T) {
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			UseAzureADAuth:     new(true),
		}).
		Build(log.New(), isolatedEnv())
	require.NoError(t, err)
	assert.Empty(t, cfg.SubscriptionID)
}

// TestSubscriptionRequiredAtArmBoundary pins that the requirement moved rather
// than disappeared: the ARM clients still reject a missing subscription id.
func TestSubscriptionRequiredAtArmBoundary(t *testing.T) {
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			ResourceGroupName:  "rg",
			UseAzureADAuth:     new(true),
		}).
		Build(log.New(), isolatedEnv())
	require.NoError(t, err)

	_, err = azurehelper.NewStorageAccountClient(cfg)
	require.ErrorIs(t, err, azurehelper.ErrSubscriptionIDRequired)

	_, err = azurehelper.NewResourceGroupClient(cfg)
	require.ErrorIs(t, err, azurehelper.ErrSubscriptionIDRequired)
}

func TestBuild_SasTokenWithoutAccountFails(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SasToken: "sv=test",
		}).
		Build(log.New(), isolatedEnv())
	require.ErrorIs(t, err, azurehelper.ErrStorageAccountRequired)
}

func TestBuild_AccessKeyWithoutAccountFails(t *testing.T) {
	t.Parallel()
	// Mirror of the SAS-token case: access-key auth is data-plane only and
	// is meaningless without a target storage account.
	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			AccessKey: "a2V5", // base64("key")
		}).
		Build(log.New(), isolatedEnv())
	require.ErrorIs(t, err, azurehelper.ErrStorageAccountRequired)
}

func TestBuild_CloudEnvironmentMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		want cloud.Configuration
		env  string
	}{
		{env: "", want: cloud.AzurePublic},
		{env: "public", want: cloud.AzurePublic},
		{env: "government", want: cloud.AzureGovernment},
		{env: "USGOVERNMENT", want: cloud.AzureGovernment},
		{env: "china", want: cloud.AzureChina},
		{env: "AzureChinaCloud", want: cloud.AzureChina},
	}

	for _, tc := range tests {
		t.Run("env="+tc.env, func(t *testing.T) {
			t.Parallel()

			cfg, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&azurehelper.AzureSessionConfig{
					StorageAccountName: testAccount,
					SasToken:           testSASToken,
					CloudEnvironment:   tc.env,
				}).
				Build(log.New(), isolatedEnv())
			require.NoError(t, err)
			assert.Equal(t, tc.want.ActiveDirectoryAuthorityHost, cfg.CloudConfig.ActiveDirectoryAuthorityHost)
		})
	}
}

func TestBuild_NilSessionConfig(t *testing.T) {
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		Build(log.New(), isolatedEnv("ARM_SUBSCRIPTION_ID", "sub"))
	require.NoError(t, err)
	assert.Equal(t, "sub", cfg.SubscriptionID)
	assert.Equal(t, azurehelper.AuthMethodAzureAD, cfg.Method)
}

func TestBuildBlobClient_SasToken(t *testing.T) {
	t.Parallel()

	bc, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			SasToken:           testSASToken,
		}).
		BuildBlobClient(log.New(), isolatedEnv())
	require.NoError(t, err, "BuildBlobClient")
	require.NotNil(t, bc)
	assert.Equal(t, testAccount, bc.AccountName)
}

func TestBuildBlobClient_PropagatesBuildError(t *testing.T) {
	t.Parallel()
	// No StorageAccountName set anywhere -> Build's validate() rejects.
	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{SasToken: testSASToken}).
		BuildBlobClient(log.New(), isolatedEnv())
	require.ErrorIs(t, err, azurehelper.ErrStorageAccountRequired)
}

func TestBuild_RejectsUnknownCloudEnvironment(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			SasToken:           testSASToken,
			CloudEnvironment:   "governmnt", // typo
		}).
		Build(log.New(), isolatedEnv())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cloud environment")
}

func TestBuildStorageAccountClient_RequiresArmFields(t *testing.T) {
	t.Parallel()
	// SAS-token auth has no token credential -> NewStorageAccountClient errors.
	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			SasToken:           testSASToken,
		}).
		BuildStorageAccountClient(log.New(), isolatedEnv())
	require.Error(t, err, "ARM-plane fields are required for a storage account client")
}

func TestWithSessionConfig_NilKeepsCurrent(t *testing.T) {
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{StorageAccountName: testAccount, SasToken: testSASToken}).
		WithSessionConfig(nil).
		Build(log.New(), isolatedEnv())
	require.NoError(t, err)
	assert.Equal(t, testAccount, cfg.AccountName, "a nil session config must not discard the one already set")
}

// TestBuild_OIDC covers both OIDC credential sources: a federated token file
// (workload identity) and a CI token request url, each selected without an
// explicit use_oidc because the environment implies it.
func TestBuild_OIDC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		env  *venv.Venv
		name string
	}{
		{
			name: "token file implies oidc",
			env:  isolatedEnv("AZURE_FEDERATED_TOKEN_FILE", "/var/run/secrets/azure/tokens/azure-identity-token"),
		},
		{
			name: "github actions request url",
			env: isolatedEnv(
				"ACTIONS_ID_TOKEN_REQUEST_URL", "https://pipelines.example/token",
				"ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runner-token",
			),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&azurehelper.AzureSessionConfig{
					StorageAccountName: testAccount,
					TenantID:           "tid",
					ClientID:           "cid",
				}).
				Build(log.New(), tc.env)
			require.NoError(t, err)
			assert.Equal(t, azurehelper.AuthMethodOIDC, cfg.Method)
			assert.NotNil(t, cfg.Credential)
		})
	}
}

func TestBuild_OIDCTokenFromEnvironment(t *testing.T) {
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			TenantID:           "tid",
			ClientID:           "cid",
		}).
		Build(logger.CreateLogger(), isolatedEnv("ARM_USE_OIDC", "true", "ARM_OIDC_TOKEN", "assertion-jwt"))
	require.NoError(t, err)
	require.Equal(t, azurehelper.AuthMethodOIDC, cfg.Method)
	require.NotNil(t, cfg.Credential)
}

func TestBuild_OIDCTokenDoesNotImplyOIDC(t *testing.T) {
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			TenantID:           "tid",
			ClientID:           "cid",
		}).
		Build(logger.CreateLogger(), isolatedEnv("ARM_OIDC_TOKEN", "assertion-jwt"))
	require.NoError(t, err)
	require.Equal(t, azurehelper.AuthMethodAzureAD, cfg.Method)
}

func TestBuild_AKSWorkloadIdentity(t *testing.T) {
	t.Parallel()

	podEnv := func(pairs ...string) *venv.Venv {
		return isolatedEnv(append([]string{
			"AZURE_CLIENT_ID", testPodClient,
			"AZURE_TENANT_ID", testPodTenant,
			"AZURE_FEDERATED_TOKEN_FILE", "/var/run/secrets/azure/tokens/azure-identity-token",
		}, pairs...)...)
	}

	tests := []struct {
		env      *venv.Venv
		name     string
		method   azurehelper.AuthMethod
		clientID string
		tenantID string
		cfg      azurehelper.AzureSessionConfig
	}{
		{
			name:     "flag in the config",
			cfg:      azurehelper.AzureSessionConfig{UseAzureADAuth: new(true), UseAKSWorkloadIdentity: new(true)},
			env:      podEnv(),
			method:   azurehelper.AuthMethodOIDC,
			clientID: testPodClient,
			tenantID: testPodTenant,
		},
		{
			name:     "flag in the environment",
			cfg:      azurehelper.AzureSessionConfig{UseAzureADAuth: new(true)},
			env:      podEnv("ARM_USE_AKS_WORKLOAD_IDENTITY", "true"),
			method:   azurehelper.AuthMethodOIDC,
			clientID: testPodClient,
			tenantID: testPodTenant,
		},
		{
			name: "another client and tenant in the config",
			cfg: azurehelper.AzureSessionConfig{
				UseAzureADAuth:         new(true),
				UseAKSWorkloadIdentity: new(true),
				ClientID:               "cid",
				TenantID:               "tid",
			},
			env:      podEnv(),
			method:   azurehelper.AuthMethodOIDC,
			clientID: testPodClient,
			tenantID: testPodTenant,
		},
		{
			name:     "use_oidc is off",
			cfg:      azurehelper.AzureSessionConfig{UseOIDC: new(false), UseAKSWorkloadIdentity: new(true)},
			env:      podEnv(),
			method:   azurehelper.AuthMethodOIDC,
			clientID: testPodClient,
			tenantID: testPodTenant,
		},
		{
			name: "use_oidc is on",
			cfg: azurehelper.AzureSessionConfig{
				UseOIDC:                new(true),
				UseAKSWorkloadIdentity: new(true),
				ClientID:               "cid",
				TenantID:               "tid",
			},
			env:      podEnv(),
			method:   azurehelper.AuthMethodOIDC,
			clientID: "cid",
			tenantID: "tid",
		},
		{
			name: "flag is off in the config",
			cfg: azurehelper.AzureSessionConfig{
				UseAzureADAuth:         new(true),
				UseAKSWorkloadIdentity: new(false),
				ClientID:               "cid",
				TenantID:               "tid",
			},
			env:      podEnv("ARM_USE_AKS_WORKLOAD_IDENTITY", "true"),
			method:   azurehelper.AuthMethodAzureAD,
			clientID: "cid",
			tenantID: "tid",
		},
		{
			name: "flag alone with another client and tenant in the config",
			cfg: azurehelper.AzureSessionConfig{
				UseAKSWorkloadIdentity: new(true),
				ClientID:               "cid",
				TenantID:               "tid",
			},
			env:      podEnv(),
			method:   azurehelper.AuthMethodOIDC,
			clientID: testPodClient,
			tenantID: testPodTenant,
		},
		{
			name:     "flag alone with another client in the environment",
			cfg:      azurehelper.AzureSessionConfig{UseAKSWorkloadIdentity: new(true)},
			env:      podEnv("ARM_CLIENT_ID", "other-client"),
			method:   azurehelper.AuthMethodOIDC,
			clientID: testPodClient,
			tenantID: testPodTenant,
		},
		{
			name: "flag alone with a token request and another client in the environment",
			cfg:  azurehelper.AzureSessionConfig{UseAKSWorkloadIdentity: new(true)},
			env: podEnv(
				"ACTIONS_ID_TOKEN_REQUEST_URL", "https://pipelines.example/token",
				"ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runner-token",
				"ARM_CLIENT_ID", "other-client",
				"ARM_TENANT_ID", "other-tenant",
			),
			method:   azurehelper.AuthMethodOIDC,
			clientID: testPodClient,
			tenantID: testPodTenant,
		},
		{
			name: "managed identity",
			cfg: azurehelper.AzureSessionConfig{
				UseMSI:                 new(true),
				UseAKSWorkloadIdentity: new(true),
				ClientID:               "cid",
				TenantID:               "tid",
			},
			env:      podEnv(),
			method:   azurehelper.AuthMethodMSI,
			clientID: "cid",
			tenantID: "tid",
		},
		{
			name: "service principal",
			cfg: azurehelper.AzureSessionConfig{
				UseAKSWorkloadIdentity: new(true),
				ClientID:               "cid",
				ClientSecret:           "sec",
				TenantID:               "tid",
			},
			env:      podEnv(),
			method:   azurehelper.AuthMethodServicePrincipal,
			clientID: "cid",
			tenantID: "tid",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.cfg.StorageAccountName = testAccount

			cfg, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&tc.cfg).
				Build(logger.CreateLogger(), tc.env)
			require.NoError(t, err)
			require.Equal(t, tc.method, cfg.Method)
			require.Equal(t, tc.clientID, cfg.ClientID)
			require.Equal(t, tc.tenantID, cfg.TenantID)
			require.NotNil(t, cfg.Credential)
		})
	}
}

func TestBuild_AKSWorkloadIdentityWithoutPodIdentity(t *testing.T) {
	t.Parallel()

	const tokenFile = "/var/run/secrets/azure/tokens/azure-identity-token"

	tests := []struct {
		env    *venv.Venv
		name   string
		method azurehelper.AuthMethod
		cfg    azurehelper.AzureSessionConfig
	}{
		{
			name:   "no client id",
			cfg:    azurehelper.AzureSessionConfig{UseAzureADAuth: new(true), UseAKSWorkloadIdentity: new(true)},
			env:    isolatedEnv("AZURE_TENANT_ID", testPodTenant, "AZURE_FEDERATED_TOKEN_FILE", tokenFile),
			method: azurehelper.AuthMethodAzureAD,
		},
		{
			name: "client id only in the config",
			cfg: azurehelper.AzureSessionConfig{
				UseAzureADAuth:         new(true),
				UseAKSWorkloadIdentity: new(true),
				ClientID:               "cid",
			},
			env:    isolatedEnv("AZURE_TENANT_ID", testPodTenant, "AZURE_FEDERATED_TOKEN_FILE", tokenFile),
			method: azurehelper.AuthMethodAzureAD,
		},
		{
			name:   "no tenant id",
			cfg:    azurehelper.AzureSessionConfig{UseAzureADAuth: new(true), UseAKSWorkloadIdentity: new(true)},
			env:    isolatedEnv("AZURE_CLIENT_ID", testPodClient, "AZURE_FEDERATED_TOKEN_FILE", tokenFile),
			method: azurehelper.AuthMethodAzureAD,
		},
		{
			name: "token request url and no token file",
			cfg:  azurehelper.AzureSessionConfig{UseAKSWorkloadIdentity: new(true)},
			env: isolatedEnv(
				"AZURE_CLIENT_ID", testPodClient,
				"AZURE_TENANT_ID", testPodTenant,
				"ACTIONS_ID_TOKEN_REQUEST_URL", "https://token.example.com",
				"ACTIONS_ID_TOKEN_REQUEST_TOKEN", "request-token",
			),
			method: azurehelper.AuthMethodOIDC,
		},
		{
			name:   "no webhook variables",
			cfg:    azurehelper.AzureSessionConfig{UseAKSWorkloadIdentity: new(true)},
			env:    isolatedEnv(),
			method: azurehelper.AuthMethodAzureAD,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.cfg.StorageAccountName = testAccount

			cfg, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&tc.cfg).
				Build(logger.CreateLogger(), tc.env)
			require.NoError(t, err)
			require.Equal(t, tc.method, cfg.Method)
		})
	}
}

func TestBuild_AKSWorkloadIdentitySignsInAsPodIdentity(t *testing.T) {
	t.Parallel()

	tokenFile := filepath.Join(t.TempDir(), "azure-identity-token")
	require.NoError(t, os.WriteFile(tokenFile, []byte("pod-assertion"), 0o600))

	otherTokenFile := filepath.Join(t.TempDir(), "other-token")
	require.NoError(t, os.WriteFile(otherTokenFile, []byte("other-assertion"), 0o600))

	var (
		mu       sync.Mutex
		tokenURL string
		form     url.Values
	)

	jsonHeader := http.Header{"Content-Type": []string{"application/json"}}
	authority := "https://login.microsoftonline.com/" + testPodTenant

	entra := vhttp.NewMemClient(func(_ context.Context, req *http.Request) (*http.Response, error) {
		if strings.HasSuffix(req.URL.Path, "/.well-known/openid-configuration") {
			metadata := fmt.Sprintf(
				`{"issuer":"%[1]s/v2.0","authorization_endpoint":"%[1]s/oauth2/v2.0/authorize",`+
					`"token_endpoint":"%[1]s/oauth2/v2.0/token"}`,
				authority,
			)

			return vhttp.Respond(http.StatusOK, []byte(metadata), jsonHeader), nil
		}

		if !strings.HasSuffix(req.URL.Path, "/oauth2/v2.0/token") {
			return vhttp.Respond(http.StatusNotFound, nil, nil), nil
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}

		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, err
		}

		mu.Lock()
		defer mu.Unlock()

		tokenURL, form = req.URL.String(), values

		const response = `{"token_type":"Bearer","expires_in":3600,"access_token":"pod-access-token"}`

		return vhttp.Respond(http.StatusOK, []byte(response), jsonHeader), nil
	})

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			UseAzureADAuth:     new(true),
			ClientID:           "cid",
			TenantID:           "tid",
		}).
		Build(logger.CreateLogger(), isolatedEnv(
			"ARM_USE_AKS_WORKLOAD_IDENTITY", "true",
			"AZURE_CLIENT_ID", testPodClient,
			"AZURE_TENANT_ID", testPodTenant,
			"AZURE_FEDERATED_TOKEN_FILE", tokenFile,
			"ARM_OIDC_TOKEN_FILE_PATH", otherTokenFile,
		).WithHTTP(entra))
	require.NoError(t, err)

	token, err := cfg.Credential.GetToken(t.Context(), policy.TokenRequestOptions{
		Scopes: []string{"https://storage.azure.com/.default"},
	})
	require.NoError(t, err)
	require.Equal(t, "pod-access-token", token.Token)

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, authority+"/oauth2/v2.0/token", tokenURL)
	require.Equal(t, testPodClient, form.Get("client_id"))
	require.Equal(t, "pod-assertion", form.Get("client_assertion"))
}

func TestBuild_AKSWorkloadIdentityFingerprintCoversPodTokenFile(t *testing.T) {
	t.Parallel()

	fingerprint := func(t *testing.T, podTokenFile string, pairs ...string) string {
		t.Helper()

		cfg, err := azurehelper.NewAzureConfigBuilder().
			WithSessionConfig(&azurehelper.AzureSessionConfig{
				StorageAccountName:     testAccount,
				UseAzureADAuth:         new(true),
				UseAKSWorkloadIdentity: new(true),
			}).
			Build(logger.CreateLogger(), isolatedEnv(append([]string{
				"AZURE_CLIENT_ID", testPodClient,
				"AZURE_TENANT_ID", testPodTenant,
				"AZURE_FEDERATED_TOKEN_FILE", podTokenFile,
			}, pairs...)...))
		require.NoError(t, err)

		return cfg.CredentialFingerprint
	}

	const podTokenFile = "/var/run/secrets/azure/tokens/azure-identity-token"

	base := fingerprint(t, podTokenFile)

	require.Equal(t, base, fingerprint(t, podTokenFile, "ARM_OIDC_TOKEN_FILE_PATH", "/var/run/secrets/other/token"))
	require.NotEqual(t, base, fingerprint(t, "/var/run/secrets/other/token"))
}

func TestBuild_CredentialFingerprintCoversOIDCSources(t *testing.T) {
	t.Parallel()

	fingerprint := func(token, serviceConnectionID string) string {
		cfg, err := azurehelper.NewAzureConfigBuilder().
			WithSessionConfig(&azurehelper.AzureSessionConfig{
				StorageAccountName:     testAccount,
				TenantID:               "tid",
				ClientID:               "cid",
				UseOIDC:                new(true),
				OIDCToken:              token,
				ADOServiceConnectionID: serviceConnectionID,
			}).
			Build(logger.CreateLogger(), isolatedEnv())
		require.NoError(t, err)

		return cfg.CredentialFingerprint
	}

	base := fingerprint("first-jwt", "first-connection")

	require.Equal(t, base, fingerprint("first-jwt", "first-connection"))
	require.NotEqual(t, base, fingerprint("second-jwt", "first-connection"))
	require.NotEqual(t, base, fingerprint("first-jwt", "second-connection"))
	require.NotContains(t, base, "first-jwt")
}

func TestBuild_CredentialFingerprintCoversAzureDevOpsRequest(t *testing.T) {
	t.Parallel()

	const (
		firstURL   = "https://dev.azure.example/first/oidctoken"
		firstToken = "first-request-token"
	)

	fingerprint := func(t *testing.T, pairs ...string) string {
		t.Helper()

		cfg, err := azurehelper.NewAzureConfigBuilder().
			WithSessionConfig(&azurehelper.AzureSessionConfig{
				StorageAccountName:     testAccount,
				TenantID:               "tid",
				ClientID:               "cid",
				UseOIDC:                new(true),
				ADOServiceConnectionID: "connection",
			}).
			Build(logger.CreateLogger(), isolatedEnv(pairs...))
		require.NoError(t, err)

		return cfg.CredentialFingerprint
	}

	base := fingerprint(t, "SYSTEM_OIDCREQUESTURI", firstURL, "SYSTEM_ACCESSTOKEN", firstToken)

	require.Equal(t, base, fingerprint(t, "SYSTEM_OIDCREQUESTURI", firstURL, "SYSTEM_ACCESSTOKEN", firstToken))
	require.NotContains(t, base, firstToken)

	tests := []struct {
		name string
		env  []string
	}{
		{
			name: "request url",
			env: []string{
				"SYSTEM_OIDCREQUESTURI", "https://dev.azure.example/second/oidctoken",
				"SYSTEM_ACCESSTOKEN", firstToken,
			},
		},
		{
			name: "access token",
			env:  []string{"SYSTEM_OIDCREQUESTURI", firstURL, "SYSTEM_ACCESSTOKEN", "second-request-token"},
		},
		{
			name: "request token",
			env: []string{
				"SYSTEM_OIDCREQUESTURI", firstURL,
				"SYSTEM_ACCESSTOKEN", firstToken,
				"ARM_OIDC_REQUEST_TOKEN", "second-request-token",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			require.NotEqual(t, base, fingerprint(t, tc.env...))
		})
	}
}

// TestBuild_CredentialConstructionErrors verifies an SDK constructor failure
// names the auth method being built. Entra tenant ids allow only alphanumerics,
// '-' and '.', so the SDK rejects this one without any network call.
func TestBuild_CredentialConstructionErrors(t *testing.T) {
	t.Parallel()

	const (
		badTenant     = "bad tenant!"
		assertionFile = "/var/run/secrets/azure/tokens/azure-identity-token"
	)

	tests := []struct {
		env  *venv.Venv
		cfg  azurehelper.AzureSessionConfig
		name string
		want string
	}{
		{
			name: "service principal",
			cfg:  azurehelper.AzureSessionConfig{TenantID: badTenant, ClientID: "cid", ClientSecret: "sec"},
			env:  isolatedEnv(),
			want: "creating service principal credential",
		},
		{
			name: "oidc request url",
			cfg:  azurehelper.AzureSessionConfig{UseOIDC: new(true), TenantID: badTenant, ClientID: "cid"},
			env: isolatedEnv(
				"ACTIONS_ID_TOKEN_REQUEST_URL", "https://pipelines.example/token",
				"ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runner-token",
			),
			want: "creating OIDC client assertion credential",
		},
		{
			name: "oidc workload identity",
			cfg: azurehelper.AzureSessionConfig{
				UseOIDC:           new(true),
				TenantID:          badTenant,
				ClientID:          "cid",
				OIDCTokenFilePath: "/var/run/secrets/azure/tokens/azure-identity-token",
			},
			env:  isolatedEnv(),
			want: "creating OIDC workload identity credential",
		},
		{
			name: "oidc token",
			cfg:  azurehelper.AzureSessionConfig{UseOIDC: new(true), TenantID: badTenant, ClientID: "cid"},
			env:  isolatedEnv("ARM_OIDC_TOKEN", "assertion-jwt"),
			want: "creating OIDC client assertion credential",
		},
		{
			name: "oidc token wins over token file",
			cfg: azurehelper.AzureSessionConfig{
				UseOIDC:           new(true),
				TenantID:          badTenant,
				ClientID:          "cid",
				OIDCTokenFilePath: assertionFile,
			},
			env:  isolatedEnv("ARM_OIDC_TOKEN", "assertion-jwt"),
			want: "creating OIDC client assertion credential",
		},
		{
			name: "oidc token file wins over request url",
			cfg: azurehelper.AzureSessionConfig{
				UseOIDC:           new(true),
				TenantID:          badTenant,
				ClientID:          "cid",
				OIDCTokenFilePath: assertionFile,
			},
			env: isolatedEnv(
				"ACTIONS_ID_TOKEN_REQUEST_URL", "https://pipelines.example/token",
				"ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runner-token",
			),
			want: "creating OIDC workload identity credential",
		},
		{
			name: "federated token file wins over request url",
			cfg:  azurehelper.AzureSessionConfig{UseOIDC: new(true), TenantID: badTenant, ClientID: "cid"},
			env: isolatedEnv(
				"AZURE_FEDERATED_TOKEN_FILE", assertionFile,
				"ACTIONS_ID_TOKEN_REQUEST_URL", "https://pipelines.example/token",
				"ACTIONS_ID_TOKEN_REQUEST_TOKEN", "runner-token",
			),
			want: "creating OIDC workload identity credential",
		},
		{
			name: "azure devops service connection wins over token file",
			cfg: azurehelper.AzureSessionConfig{
				UseOIDC:           new(true),
				TenantID:          badTenant,
				ClientID:          "cid",
				OIDCTokenFilePath: assertionFile,
			},
			env: isolatedEnv(
				"ARM_ADO_PIPELINE_SERVICE_CONNECTION_ID", "service-connection",
				"SYSTEM_OIDCREQUESTURI", "https://dev.azure.example/oidc",
				"ARM_OIDC_REQUEST_TOKEN", "request-token",
			),
			want: "creating OIDC client assertion credential",
		},
		{
			name: "azure devops service connection without a request token leaves the token file",
			cfg: azurehelper.AzureSessionConfig{
				UseOIDC:           new(true),
				TenantID:          badTenant,
				ClientID:          "cid",
				OIDCTokenFilePath: assertionFile,
			},
			env: isolatedEnv(
				"ARM_ADO_PIPELINE_SERVICE_CONNECTION_ID", "service-connection",
				"SYSTEM_OIDCREQUESTURI", "https://dev.azure.example/oidc",
			),
			want: "creating OIDC workload identity credential",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&tc.cfg).
				Build(log.New(), tc.env)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

// TestBuild_ManagedIdentityCredentialError drives the SDK's own rejection of a
// user-assigned identity on Cloud Shell. The SDK picks the managed identity
// source from the process environment, so this test isolates it with
// t.Setenv and therefore cannot run in parallel.
func TestBuild_ManagedIdentityCredentialError(t *testing.T) {
	for _, key := range []string{"IDENTITY_ENDPOINT", "IDENTITY_HEADER", "IMDS_ENDPOINT", "MSI_SECRET"} {
		t.Setenv(key, "")
	}

	// MSI_ENDPOINT without MSI_SECRET is how the SDK recognizes Cloud Shell.
	t.Setenv("MSI_ENDPOINT", "http://127.0.0.1:50342/oauth2/token")

	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{UseMSI: new(true), ClientID: "cid"}).
		Build(log.New(), isolatedEnv())
	require.ErrorContains(t, err, "creating managed identity credential")
}

// TestBuild_DefaultCredentialError verifies a DefaultAzureCredential failure
// is surfaced. The SDK reads AZURE_TOKEN_CREDENTIALS from the process
// environment, so this test sets it with t.Setenv and cannot run in parallel.
func TestBuild_DefaultCredentialError(t *testing.T) {
	t.Setenv("AZURE_TOKEN_CREDENTIALS", "not-a-credential")

	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{StorageAccountName: testAccount}).
		Build(log.New(), isolatedEnv())
	require.ErrorContains(t, err, "creating default Azure credential")
}

func TestBuildStorageAccountClient(t *testing.T) {
	t.Parallel()

	tests := []struct {
		check func(t *testing.T, sc *azurehelper.StorageAccountClient, err error)
		env   *venv.Venv
		cfg   azurehelper.AzureSessionConfig
		name  string
	}{
		{
			// ARM_ACCESS_KEY is caught before Build, same as an explicit access key.
			name: "access key from env is rejected up front",
			cfg:  azurehelper.AzureSessionConfig{StorageAccountName: testAccount},
			env:  isolatedEnv("ARM_ACCESS_KEY", "a2V5"),
			check: func(t *testing.T, _ *azurehelper.StorageAccountClient, err error) {
				t.Helper()

				var unsupported *azurehelper.UnsupportedAuthForOpError
				require.ErrorAs(t, err, &unsupported)
				assert.Equal(t, azurehelper.AuthMethodAccessKey, unsupported.Method)
			},
		},
		{
			name: "build error is propagated",
			cfg: azurehelper.AzureSessionConfig{
				StorageAccountName: testAccount,
				UseAzureADAuth:     new(true),
				CloudEnvironment:   "governmnt",
			},
			env: isolatedEnv(),
			check: func(t *testing.T, _ *azurehelper.StorageAccountClient, err error) {
				t.Helper()

				var unknown *azurehelper.UnknownCloudEnvironmentError
				require.ErrorAs(t, err, &unknown)
			},
		},
		{
			name: "token credential builds a client",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID:     testSub,
				ResourceGroupName:  "rg",
				StorageAccountName: testAccount,
				UseAzureADAuth:     new(true),
			},
			env: isolatedEnv(),
			check: func(t *testing.T, sc *azurehelper.StorageAccountClient, err error) {
				t.Helper()

				require.NoError(t, err)
				assert.NotNil(t, sc)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sc, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&tc.cfg).
				BuildStorageAccountClient(log.New(), tc.env)
			tc.check(t, sc, err)
		})
	}
}

// isolatedEnv builds a virtualized environment from (key, value) pairs; the
// builder never reads the process environment, so resolution stays hermetic.
func isolatedEnv(pairs ...string) *venv.Venv {
	m := make(map[string]string, len(pairs)/2)

	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}

	return (&venv.Venv{FS: vfs.NewMemMapFS()}).WithEnv(m)
}

// TestBuild_CarriesAuthorizationMode pins that the blob authorization mode the
// user asked for survives config resolution. The azurerm backend reads it to
// decide between shared-key and Microsoft Entra authorization, matching the
// native backend; if it were dropped, every identity would get Entra
// authorization and those without a blob data-plane role would see 403s.
func TestBuild_CarriesAuthorizationMode(t *testing.T) {
	t.Parallel()

	entra, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     testSub,
			StorageAccountName: testAccount,
			UseAzureADAuth:     new(true),
		}).
		Build(log.New(), isolatedEnv())
	require.NoError(t, err)
	assert.True(t, entra.UseAzureADAuth, "use_azuread_auth must reach the resolved config")

	sharedKey, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SubscriptionID:     testSub,
			StorageAccountName: testAccount,
			TenantID:           "tid",
			ClientID:           "cid",
			ClientSecret:       "sec",
		}).
		Build(log.New(), isolatedEnv())
	require.NoError(t, err)
	assert.False(t, sharedKey.UseAzureADAuth, "an unset use_azuread_auth must not imply Entra authorization")
}
