package azurehelper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/internal/vhttp"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Common test fixtures used across azurehelper unit tests.
const (
	testAccount  = "acct"
	testSub      = "sub"
	testSASToken = "sv=x"
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

// TestBuild_CredentialConstructionErrors verifies an SDK constructor failure
// names the auth method being built. Entra tenant ids allow only alphanumerics,
// '-' and '.', so the SDK rejects this one without any network call.
func TestBuild_CredentialConstructionErrors(t *testing.T) {
	t.Parallel()

	const badTenant = "bad tenant!"

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

	return (&venv.Venv{}).WithEnv(m)
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
