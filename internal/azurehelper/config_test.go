//go:build azure

package azurehelper_test

import (
	"context"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Common test fixtures used across azurehelper unit tests.
const (
	testAccount  = "acct"
	testSub      = "sub"
	testSASToken = "sv=x"
)

// isolatedEnv returns an env map that shields the resolver from any ARM_*
// or AZURE_* values present in the developer's shell. Tests opt back in to
// specific values by passing additional pairs.
//
// Empty values are intentional: AzureConfigBuilder.firstEnv treats "key
// present with empty value" as "do not fall back to os.Getenv for this key".
func isolatedEnv(pairs ...string) map[string]string {
	m := map[string]string{
		"ARM_SUBSCRIPTION_ID":     "",
		"AZURE_SUBSCRIPTION_ID":   "",
		"ARM_TENANT_ID":           "",
		"AZURE_TENANT_ID":         "",
		"ARM_CLIENT_ID":           "",
		"AZURE_CLIENT_ID":         "",
		"ARM_CLIENT_SECRET":       "",
		"AZURE_CLIENT_SECRET":     "",
		"ARM_SAS_TOKEN":           "",
		"AZURE_STORAGE_SAS_TOKEN": "",
		"ARM_ACCESS_KEY":          "",
		"AZURE_STORAGE_KEY":       "",
		"ARM_ENVIRONMENT":         "",
		"AZURE_ENVIRONMENT":       "",
		"ARM_USE_MSI":             "",
		"ARM_USE_OIDC":            "",
	}

	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = pairs[i+1]
	}

	return m
}

func TestBuild_AuthMethodPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		want    azurehelper.AuthMethod
		cfg     azurehelper.AzureSessionConfig
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
			name: "access key wins over service principal",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID:     testSub,
				StorageAccountName: testAccount,
				AccessKey:          "key",
				ClientID:           "cid",
				ClientSecret:       "sec",
				TenantID:           "tid",
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
				UseMSI:         true,
			},
			want:    azurehelper.AuthMethodMSI,
			hasCred: true,
		},
		{
			name: "oidc beats msi",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: testSub,
				UseOIDC:        true,
				UseMSI:         true,
			},
			want:    azurehelper.AuthMethodOIDC,
			hasCred: true,
		},
		{
			name: "azuread default fallback",
			cfg: azurehelper.AzureSessionConfig{
				SubscriptionID: testSub,
				UseAzureADAuth: true,
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
				WithEnv(isolatedEnv()).
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
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
		}).
		WithEnv(isolatedEnv(
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
	t.Parallel()

	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			UseMSI: true,
		}).
		WithEnv(isolatedEnv()).
		Build(context.Background(), log.New())
	if err == nil {
		t.Fatal("expected error when subscription_id missing for MSI auth")
	}
}

func TestBuild_SasTokenWithoutAccountFails(t *testing.T) {
	t.Parallel()

	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			SasToken: "sv=test",
		}).
		WithEnv(isolatedEnv()).
		Build(context.Background(), log.New())
	if err == nil {
		t.Fatal("expected error when storage_account_name missing for SAS auth")
	}
}

func TestBuild_AccessKeyWithoutAccountFails(t *testing.T) {
	t.Parallel()
	// Mirror of the SAS-token case: access-key auth is data-plane only and
	// is meaningless without a target storage account.
	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			AccessKey: "a2V5", // base64("key")
		}).
		WithEnv(isolatedEnv()).
		Build(context.Background(), log.New())
	if err == nil {
		t.Fatal("expected error when storage_account_name missing for access-key auth")
	}
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
		{env: "unknown", want: cloud.AzurePublic},
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
				WithEnv(isolatedEnv()).
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
	t.Parallel()

	cfg, err := azurehelper.NewAzureConfigBuilder().
		WithEnv(isolatedEnv("ARM_SUBSCRIPTION_ID", "sub")).
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

func TestBuildBlobClient_SasToken(t *testing.T) {
	t.Parallel()

	bc, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			SasToken:           testSASToken,
		}).
		WithEnv(isolatedEnv()).
		BuildBlobClient(context.Background(), log.New())
	if err != nil {
		t.Fatalf("BuildBlobClient: %v", err)
	}

	if bc == nil {
		t.Fatal("BuildBlobClient returned nil client")
	}

	if bc.AccountName() != testAccount {
		t.Errorf("AccountName() = %q, want %q", bc.AccountName(), testAccount)
	}
}

func TestBuildBlobClient_PropagatesBuildError(t *testing.T) {
	t.Parallel()
	// No StorageAccountName set anywhere → Build's validate() rejects.
	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{SasToken: testSASToken}).
		WithEnv(isolatedEnv()).
		BuildBlobClient(context.Background(), log.New())
	if err == nil {
		t.Fatal("expected error when storage account name missing")
	}
}

func TestBuildStorageAccountClient_RequiresArmFields(t *testing.T) {
	t.Parallel()
	// SAS-token auth has no token credential → NewStorageAccountClient errors.
	_, err := azurehelper.NewAzureConfigBuilder().
		WithSessionConfig(&azurehelper.AzureSessionConfig{
			StorageAccountName: testAccount,
			SasToken:           testSASToken,
		}).
		WithEnv(isolatedEnv()).
		BuildStorageAccountClient(context.Background(), log.New())
	if err == nil {
		t.Fatal("expected error when ARM-plane fields missing for storage account client")
	}
}
