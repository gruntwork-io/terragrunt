package azurehelper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/venv"
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
				WithVenv(isolatedEnv()).
				Build(log.New())
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
		WithVenv(isolatedEnv(
			"ARM_SAS_TOKEN", "sv=test",
			"ARM_SUBSCRIPTION_ID", "sub-from-env",
		)).
		Build(log.New())
	require.NoError(t, err)
	assert.Equal(t, azurehelper.AuthMethodSasToken, cfg.Method)
	assert.Equal(t, "sv=test", cfg.SasToken)
}

func TestBuild_TrimsWhitespaceFromEnvValues(t *testing.T) {
	t.Parallel()
	// CI often injects secrets with a trailing newline; it must be trimmed.
	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{StorageAccountName: testAccount, SasToken: testSASToken}).
		WithVenv(isolatedEnv("ARM_SUBSCRIPTION_ID", "sub-from-env\n")).
		Build(log.New())
	require.NoError(t, err)
	assert.Equal(t, "sub-from-env", cfg.SubscriptionID, "env values must be trimmed")
}

func TestBuild_StorageAccountNameEnvFallback(t *testing.T) {
	t.Parallel()
	// SAS auth requires a storage account; supply it only via AZURE_STORAGE_ACCOUNT.
	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{SasToken: testSASToken}).
		WithVenv(isolatedEnv("AZURE_STORAGE_ACCOUNT", "acct-from-env")).
		Build(log.New())
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
		WithVenv(isolatedEnv()).
		Build(log.New())
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
		WithVenv(isolatedEnv()).
		Build(log.New())
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
		WithVenv(isolatedEnv()).
		Build(log.New())
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
		WithVenv(isolatedEnv()).
		Build(log.New())
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
				WithVenv(isolatedEnv()).
				Build(log.New())
			require.NoError(t, err)
			assert.Equal(t, tc.want.ActiveDirectoryAuthorityHost, cfg.CloudConfig.ActiveDirectoryAuthorityHost)
		})
	}
}

func TestBuild_NilSessionConfig(t *testing.T) {
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithVenv(isolatedEnv("ARM_SUBSCRIPTION_ID", "sub")).
		Build(log.New())
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
		WithVenv(isolatedEnv()).
		BuildBlobClient(log.New())
	require.NoError(t, err, "BuildBlobClient")
	require.NotNil(t, bc)
	assert.Equal(t, testAccount, bc.AccountName)
}

func TestBuildBlobClient_PropagatesBuildError(t *testing.T) {
	t.Parallel()
	// No StorageAccountName set anywhere -> Build's validate() rejects.
	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{SasToken: testSASToken}).
		WithVenv(isolatedEnv()).
		BuildBlobClient(log.New())
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
		WithVenv(isolatedEnv()).
		Build(log.New())
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
		WithVenv(isolatedEnv()).
		BuildStorageAccountClient(log.New())
	require.Error(t, err, "ARM-plane fields are required for a storage account client")
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
		WithVenv(isolatedEnv()).
		Build(log.New())
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
		WithVenv(isolatedEnv()).
		Build(log.New())
	require.NoError(t, err)
	assert.False(t, sharedKey.UseAzureADAuth, "an unset use_azuread_auth must not imply Entra authorization")
}
