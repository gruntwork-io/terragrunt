//go:build azure

package azureauth_test

import (
	"context"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/azure/azureauth"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAuthConfig(t *testing.T) {
	logger := log.New()

	tests := []struct {
		config         map[string]interface{}
		envVars        map[string]string
		expectedAuth   func(*azureauth.AuthConfig) bool
		name           string
		expectedMethod azureauth.AuthMethod
	}{
		{
			name: "service principal from config",
			config: map[string]interface{}{
				"subscription_id": "sub-123",
				"client_id":       "client-123",
				"client_secret":   "secret-123",
				"tenant_id":       "tenant-123",
			},
			expectedMethod: azureauth.AuthMethodServicePrincipal,
			expectedAuth: func(cfg *azureauth.AuthConfig) bool {
				return cfg.ClientID == "client-123" &&
					cfg.ClientSecret == "secret-123" &&
					cfg.TenantID == "tenant-123" &&
					cfg.SubscriptionID == "sub-123"
			},
		},
		{
			name: "azure ad auth",
			config: map[string]interface{}{
				"subscription_id":  "sub-123",
				"use_azuread_auth": true,
			},
			expectedMethod: azureauth.AuthMethodAzureAD,
			expectedAuth: func(cfg *azureauth.AuthConfig) bool {
				return cfg.UseAzureAD && cfg.SubscriptionID == "sub-123"
			},
		},
		{
			name: "msi auth",
			config: map[string]interface{}{
				"subscription_id": "sub-123",
				"use_msi":         true,
			},
			expectedMethod: azureauth.AuthMethodMSI,
			expectedAuth: func(cfg *azureauth.AuthConfig) bool {
				return cfg.UseMSI && cfg.SubscriptionID == "sub-123"
			},
		},
		{
			name: "sas token auth",
			config: map[string]interface{}{
				"storage_account_name": "teststorage",
				"sas_token":            "sv=2020-08-04&ss=b&srt=c&sp=rwdlaciytfx&se=2023-04-19T17:39:00Z&st=2023-04-19T09:39:00Z&spr=https&sig=example",
			},
			expectedMethod: azureauth.AuthMethodSasToken,
			expectedAuth: func(cfg *azureauth.AuthConfig) bool {
				return cfg.SasToken != "" && cfg.StorageAccountName == "teststorage"
			},
		},
		{
			name:   "service principal from env",
			config: map[string]interface{}{},
			envVars: map[string]string{
				"AZURE_CLIENT_ID":       "env-client-123",
				"AZURE_CLIENT_SECRET":   "env-secret-123",
				"AZURE_TENANT_ID":       "env-tenant-123",
				"AZURE_SUBSCRIPTION_ID": "env-sub-123",
			},
			expectedMethod: azureauth.AuthMethodServicePrincipal,
			expectedAuth: func(cfg *azureauth.AuthConfig) bool {
				return cfg.ClientID == "env-client-123" &&
					cfg.ClientSecret == "env-secret-123" &&
					cfg.TenantID == "env-tenant-123" &&
					cfg.SubscriptionID == "env-sub-123" &&
					cfg.UseEnvironment
			},
		},
		{
			name: "sas token from env",
			config: map[string]interface{}{
				"storage_account_name": "teststorage",
			},
			envVars: map[string]string{
				"AZURE_STORAGE_SAS_TOKEN": "sv=2020-08-04&ss=b&srt=c&sp=rwdlaciytfx&se=2023-04-19T17:39:00Z&st=2023-04-19T09:39:00Z&spr=https&sig=example",
			},
			expectedMethod: azureauth.AuthMethodSasToken,
			expectedAuth: func(cfg *azureauth.AuthConfig) bool {
				return cfg.SasToken != "" &&
					cfg.StorageAccountName == "teststorage" &&
					cfg.UseEnvironment
			},
		},
		{
			name:           "default to azure ad",
			config:         map[string]interface{}{},
			envVars:        map[string]string{},
			expectedMethod: azureauth.AuthMethodAzureAD,
			expectedAuth: func(cfg *azureauth.AuthConfig) bool {
				return cfg.UseAzureAD
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}

			cfg, err := azureauth.GetAuthConfig(context.Background(), logger, tt.config)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedMethod, cfg.Method)
			assert.True(t, tt.expectedAuth(cfg), "Auth config validation failed")
		})
	}
}

//nolint:paralleltest // This test uses t.Setenv which is incompatible with t.Parallel
func TestValidateAuthConfig(t *testing.T) {
	tests := []struct {
		config    *azureauth.AuthConfig
		name      string
		expectErr bool
	}{
		{
			name: "valid service principal",
			config: &azureauth.AuthConfig{
				Method:         azureauth.AuthMethodServicePrincipal,
				ClientID:       "client-123",
				ClientSecret:   "secret-123",
				TenantID:       "tenant-123",
				SubscriptionID: "sub-123",
			},
			expectErr: false,
		},
		{
			name: "invalid service principal - missing client id",
			config: &azureauth.AuthConfig{
				Method:         azureauth.AuthMethodServicePrincipal,
				ClientSecret:   "secret-123",
				TenantID:       "tenant-123",
				SubscriptionID: "sub-123",
			},
			expectErr: true,
		},
		{
			name: "valid sas token",
			config: &azureauth.AuthConfig{
				Method:             azureauth.AuthMethodSasToken,
				SasToken:           "sv=2020-08-04&ss=b&srt=c&sp=rwdlaciytfx&se=2023-04-19T17:39:00Z&st=2023-04-19T09:39:00Z&spr=https&sig=example",
				StorageAccountName: "teststorage",
			},
			expectErr: false,
		},
		{
			name: "invalid sas token - missing token",
			config: &azureauth.AuthConfig{
				Method:             azureauth.AuthMethodSasToken,
				StorageAccountName: "teststorage",
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := azureauth.ValidateAuthConfig(tt.config)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

//nolint:paralleltest // Simple test with no parallelizable subtests
func TestGetAzureStorageURL(t *testing.T) {
	tests := []struct {
		name           string
		storageAccount string
		endpointSuffix string
		expectedURL    string
	}{
		{
			name:           "default endpoint",
			storageAccount: "teststorage",
			endpointSuffix: "",
			expectedURL:    "https://teststorage.blob.core.windows.net",
		},
		{
			name:           "custom endpoint",
			storageAccount: "teststorage",
			endpointSuffix: "core.cloudapi.de",
			expectedURL:    "https://teststorage.blob.core.cloudapi.de",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := azureauth.GetAzureStorageURL(tt.storageAccount, tt.endpointSuffix)
			assert.Equal(t, tt.expectedURL, url)
		})
	}
}

//nolint:paralleltest // Simple integration test
func TestIsAzureError(t *testing.T) {
	// This is more of an integration test and would require real Azure errors
	// We'll just test the function signature here
	assert.NotPanics(t, func() {
		azureauth.IsAzureError(nil, "StorageAccountNotFound")
	})
}
