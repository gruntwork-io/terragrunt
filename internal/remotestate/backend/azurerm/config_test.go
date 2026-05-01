package azurerm_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testStorageAccount = "tfstate1234"
	testContainer      = "tfstate"
	testKey            = "prod.tfstate"
	testRG             = "rg"
	testSAShort        = "sa"
	testContShort      = "c"
	testKeyShort       = "k.tfstate"

	keyStorageAccount = "storage_account_name"
	keyContainer      = "container_name"
	keyKey            = "key"
	keyResourceGroup  = "resource_group_name"
)

func TestConfig_FilterOutTerragruntKeys(t *testing.T) {
	t.Parallel()

	in := azurerm.Config{
		keyStorageAccount:                testStorageAccount,
		keyContainer:                     testContainer,
		keyKey:                           testKey,
		keyResourceGroup:                 testRG,
		"use_azuread_auth":               true,
		"location":                       "westeurope",
		"skip_resource_group_creation":   false,
		"skip_storage_account_creation":  true,
		"skip_container_creation":        false,
		"skip_versioning":                true,
		"enable_soft_delete":             true,
		"soft_delete_retention_days":     14,
		"account_tier":                   "Standard",
		"account_replication_type":       "GRS",
		"account_kind":                   "StorageV2",
		"access_tier":                    "Hot",
		"tags":                           map[string]string{"env": "prod"},
		"assign_storage_blob_data_owner": true,
	}

	got := in.FilterOutTerragruntKeys()

	want := azurerm.Config{
		keyStorageAccount:  testStorageAccount,
		keyContainer:       testContainer,
		keyKey:             testKey,
		keyResourceGroup:   testRG,
		"use_azuread_auth": true,
	}

	assert.Equal(t, want, got)
}

func TestConfig_ParseExtendedAzureRMConfig(t *testing.T) {
	t.Parallel()

	in := azurerm.Config{
		keyStorageAccount:               testStorageAccount,
		keyContainer:                    testContainer,
		keyKey:                          testKey,
		keyResourceGroup:                testRG,
		"subscription_id":               "00000000-0000-0000-0000-000000000000",
		"tenant_id":                     "11111111-1111-1111-1111-111111111111",
		"use_azuread_auth":              true,
		"environment":                   "public",
		"location":                      "westeurope",
		"enable_soft_delete":            true,
		"soft_delete_retention_days":    14,
		"tags":                          map[string]string{"env": "prod"},
		"skip_resource_group_creation":  true,
		"skip_storage_account_creation": false,
	}

	cfg, err := in.ExtendedAzureRMConfig()
	require.NoError(t, err)

	assert.Equal(t, testStorageAccount, cfg.RemoteStateConfigAzureRM.StorageAccountName)
	assert.Equal(t, testContainer, cfg.RemoteStateConfigAzureRM.ContainerName)
	assert.Equal(t, testKey, cfg.RemoteStateConfigAzureRM.Key)
	assert.Equal(t, testRG, cfg.RemoteStateConfigAzureRM.ResourceGroupName)
	assert.True(t, cfg.RemoteStateConfigAzureRM.UseAzureADAuth)
	assert.Equal(t, "public", cfg.RemoteStateConfigAzureRM.Environment)
	assert.Equal(t, "westeurope", cfg.Location)
	assert.True(t, cfg.EnableSoftDelete)
	assert.Equal(t, 14, cfg.SoftDeleteRetentionDays)
	assert.Equal(t, map[string]string{"env": "prod"}, cfg.Tags)
	assert.True(t, cfg.SkipResourceGroupCreation)
}

func TestExtendedRemoteStateConfigAzureRM_Validate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		cfg     azurerm.Config
		name    string
		wantErr bool
	}{
		{
			name: "valid_minimum",
			cfg: azurerm.Config{
				keyStorageAccount: testSAShort,
				keyContainer:      testContShort,
				keyKey:            testKeyShort,
				keyResourceGroup:  testRG,
			},
		},
		{
			name: "missing_storage_account",
			cfg: azurerm.Config{
				keyContainer:     testContShort,
				keyKey:           testKeyShort,
				keyResourceGroup: testRG,
			},
			wantErr: true,
		},
		{
			name: "missing_container",
			cfg: azurerm.Config{
				keyStorageAccount: testSAShort,
				keyKey:            testKeyShort,
				keyResourceGroup:  testRG,
			},
			wantErr: true,
		},
		{
			name: "missing_key",
			cfg: azurerm.Config{
				keyStorageAccount: testSAShort,
				keyContainer:      testContShort,
				keyResourceGroup:  testRG,
			},
			wantErr: true,
		},
		{
			name: "missing_resource_group_when_arm_required",
			cfg: azurerm.Config{
				keyStorageAccount: testSAShort,
				keyContainer:      testContShort,
				keyKey:            testKeyShort,
			},
			wantErr: true,
		},
		{
			name: "resource_group_optional_when_all_skips_set",
			cfg: azurerm.Config{
				keyStorageAccount:               testSAShort,
				keyContainer:                    testContShort,
				keyKey:                          testKeyShort,
				"skip_resource_group_creation":  true,
				"skip_storage_account_creation": true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := tc.cfg.ExtendedAzureRMConfig()
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_IsEqual(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	cases := []struct {
		a, b azurerm.Config
		name string
		want bool
	}{
		{name: "both_empty", a: azurerm.Config{}, b: azurerm.Config{}, want: true},
		{name: "same_keys", a: azurerm.Config{keyKey: "x"}, b: azurerm.Config{keyKey: "x"}, want: true},
		{name: "differ", a: azurerm.Config{keyKey: "x"}, b: azurerm.Config{keyKey: "y"}, want: false},
		{
			name: "terragrunt_keys_ignored",
			a: azurerm.Config{
				keyKey:     "x",
				"location": "westeurope",
			},
			b:    azurerm.Config{keyKey: "x"},
			want: true,
		},
		{
			name: "string_bool_normalised",
			a:    azurerm.Config{"snapshot": true},
			b:    azurerm.Config{"snapshot": "true"},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, tc.a.IsEqual(tc.b, l))
		})
	}
}

func TestBackend_GetTFInitArgs(t *testing.T) {
	t.Parallel()

	rb := azurerm.NewBackend()

	got := rb.GetTFInitArgs(backend.Config{
		keyStorageAccount: testSAShort,
		keyContainer:      testContShort,
		keyKey:            testKeyShort,
		keyResourceGroup:  testRG,
		"location":        "westeurope",
		"tags":            map[string]string{"env": "prod"},
	})

	want := map[string]any{
		keyStorageAccount: testSAShort,
		keyContainer:      testContShort,
		keyKey:            testKeyShort,
		keyResourceGroup:  testRG,
	}

	assert.Equal(t, want, got)
}
