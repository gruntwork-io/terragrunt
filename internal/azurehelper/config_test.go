package azurehelper_test

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// envForTest returns an env map containing only the keys the test wants to set,
// shielding the resolution logic from values present in the developer's shell.
// (We can't unset os.Getenv via this map; tests that need *exclusion* of an
// env var must use t.Setenv with an empty string instead.)
func envForTest(pairs ...string) map[string]string {
	m := make(map[string]string, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}
	return m
}

func clearAzureEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"ARM_SUBSCRIPTION_ID", "AZURE_SUBSCRIPTION_ID",
		"ARM_TENANT_ID", "AZURE_TENANT_ID",
		"ARM_CLIENT_ID", "AZURE_CLIENT_ID",
		"ARM_CLIENT_SECRET", "AZURE_CLIENT_SECRET",
		"ARM_SAS_TOKEN", "AZURE_STORAGE_SAS_TOKEN",
		"ARM_ACCESS_KEY", "AZURE_STORAGE_KEY",
		"ARM_ENVIRONMENT", "AZURE_ENVIRONMENT",
		"ARM_USE_MSI", "ARM_USE_OIDC",
	} {
		t.Setenv(k, "")
	}
}

func TestBuild_AuthMethodPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		cfg     azurehelper.AzureSessionConfig
		want    azurehelper.AuthMethod
		hasCred bool
	}{
		{
			name: "sas token wins over everything",
			cfg: azurehelper.AzureSessionConfig{
				StorageAccountName: "acct",
				SasToken:           "sv=2023-01-01&sig=x",
				AccessKey:          "ignored",
				ClientID:           "ignored",
				ClientSecret:       "ignored",
				TenantID:           "ignored",
				SubscriptionID:     "sub",
			},
			want:    azurehelper.AuthMethodSasToken,
			hasCred: false,
		},
		{
			name: "access key wins over service principal",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: "sub",
				AccessKey:      "key",
				ClientID:       "cid",
				ClientSecret:   "sec",
				TenantID:       "tid",
			},
			want:    azurehelper.AuthMethodAccessKey,
			hasCred: false,
		},
		{
			name: "service principal when all three set",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: "sub",
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
				SubscriptionID: "sub",
				UseMSI:         true,
			},
			want:    azurehelper.AuthMethodMSI,
			hasCred: true,
		},
		{
			name: "oidc beats msi",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: "sub",
				UseOIDC:        true,
				UseMSI:         true,
			},
			want:    azurehelper.AuthMethodOIDC,
			hasCred: true,
		},
		{
			name: "azuread default fallback",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: "sub",
				UseAzureADAuth: true,
			},
			want:    azurehelper.AuthMethodAzureAD,
			hasCred: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearAzureEnv(t)

			got, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&tc.cfg).
				Build(context.Background(), log.New())
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if got.Method != tc.want {
				t.Errorf("Method = %q, want %q", got.Method, tc.want)
			}
			if tc.hasCred && got.Credential == nil {
				t.Errorf("expected non-nil Credential for method %q", tc.want)
			}
			if !tc.hasCred && got.Credential != nil {
				t.Errorf("expected nil Credential for method %q", tc.want)
			}
		})
	}
}

func TestBuild_EnvFallbacks(t *testing.T) {
	clearAzureEnv(t)

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: "acct",
		}).
		WithEnv(envForTest(
			"ARM_SAS_TOKEN", "sv=test",
			"ARM_SUBSCRIPTION_ID", "sub-from-env",
		)).
		Build(context.Background(), log.New())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if cfg.Method != azurehelper.AuthMethodSasToken {
		t.Errorf("Method = %q, want sas-token", cfg.Method)
	}
	if cfg.SasToken != "sv=test" {
		t.Errorf("SasToken = %q, want %q", cfg.SasToken, "sv=test")
	}
}

func TestBuild_SubscriptionRequired(t *testing.T) {
	clearAzureEnv(t)

	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			UseMSI: true,
		}).
		Build(context.Background(), log.New())
	if err == nil {
		t.Fatal("expected error when subscription_id missing for MSI auth")
	}
}

func TestBuild_SasTokenWithoutAccountFails(t *testing.T) {
	clearAzureEnv(t)

	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SasToken: "sv=test",
		}).
		Build(context.Background(), log.New())
	if err == nil {
		t.Fatal("expected error when storage_account_name missing for SAS auth")
	}
}

func TestBuild_CloudEnvironmentMapping(t *testing.T) {
	tests := []struct {
		env  string
		want cloud.Configuration
	}{
		{"", cloud.AzurePublic},
		{"public", cloud.AzurePublic},
		{"government", cloud.AzureGovernment},
		{"USGOVERNMENT", cloud.AzureGovernment},
		{"china", cloud.AzureChina},
		{"AzureChinaCloud", cloud.AzureChina},
		{"unknown", cloud.AzurePublic},
	}
	for _, tc := range tests {
		t.Run("env="+tc.env, func(t *testing.T) {
			clearAzureEnv(t)

			cfg, err := azurehelper.NewAzureConfigBuilder().
				WithSessionConfig(&azurehelper.AzureSessionConfig{
					StorageAccountName: "acct",
					SasToken:           "sv=x",
					CloudEnvironment:   tc.env,
				}).
				Build(context.Background(), log.New())
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}
			if cfg.CloudConfig.ActiveDirectoryAuthorityHost != tc.want.ActiveDirectoryAuthorityHost {
				t.Errorf("ActiveDirectoryAuthorityHost = %q, want %q",
					cfg.CloudConfig.ActiveDirectoryAuthorityHost, tc.want.ActiveDirectoryAuthorityHost)
			}
		})
	}
}

func TestBuild_NilSessionConfig(t *testing.T) {
	clearAzureEnv(t)

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithEnv(envForTest("ARM_SUBSCRIPTION_ID", "sub")).
		Build(context.Background(), log.New())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if cfg.SubscriptionID != "sub" {
		t.Errorf("SubscriptionID = %q, want %q", cfg.SubscriptionID, "sub")
	}
	if cfg.Method != azurehelper.AuthMethodAzureAD {
		t.Errorf("Method = %q, want azuread", cfg.Method)
	}
}
