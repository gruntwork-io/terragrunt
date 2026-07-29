package azurerm_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend"
	"github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtendedAzurermConfig_ParsesFields(t *testing.T) {
	t.Parallel()

	ext, err := fullConfig().ExtendedAzurermConfig()
	require.NoError(t, err)

	rs := ext.RemoteStateConfigAzurerm
	assert.Equal(t, "tfstate1234", rs.StorageAccountName)
	assert.Equal(t, "tfstate", rs.ContainerName)
	assert.Equal(t, "prod/terraform.tfstate", rs.Key)
	assert.Equal(t, "rg-state", rs.ResourceGroupName)
	assert.True(t, rs.UseAzureADAuth)

	assert.Equal(t, "eastus", ext.Location)
	assert.Equal(t, "Standard", ext.AccountTier)
	assert.Equal(t, "LRS", ext.AccountReplicationType)
	assert.True(t, ext.EnableSoftDelete)
	assert.Equal(t, 14, ext.SoftDeleteRetentionDays)
	assert.Equal(t, map[string]string{"team": "platform"}, ext.Tags)
}

func TestExtendedAzurermConfig_Validation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		mutate    func(azurerm.Config)
		name      string
		wantError bool
	}{
		{name: "valid", mutate: func(azurerm.Config) {}, wantError: false},
		{name: "missing storage_account_name", mutate: func(c azurerm.Config) { delete(c, "storage_account_name") }, wantError: true},
		{name: "missing container_name", mutate: func(c azurerm.Config) { delete(c, "container_name") }, wantError: true},
		{name: "missing key", mutate: func(c azurerm.Config) { delete(c, "key") }, wantError: true},
		{
			name: "missing resource_group is fine when skipping account creation",
			mutate: func(c azurerm.Config) {
				delete(c, "resource_group_name")
				c["skip_storage_account_creation"] = true
			},
			wantError: false,
		},
		// resource_group_name is not required at validation time; it is enforced
		// at the ARM call site, so a data-plane (SAS/access-key) config without it
		// still parses cleanly.
		{name: "missing resource_group is allowed at validation", mutate: func(c azurerm.Config) { delete(c, "resource_group_name") }, wantError: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := fullConfig()
			tc.mutate(cfg)

			_, err := cfg.ExtendedAzurermConfig()
			if tc.wantError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestGetTFInitArgs_StripsTerragruntOnlyKeys(t *testing.T) {
	t.Parallel()

	args := fullConfig().GetTFInitArgs()

	// Backend keys are forwarded.
	assert.Equal(t, "tfstate1234", args["storage_account_name"])
	assert.Equal(t, "tfstate", args["container_name"])
	assert.Equal(t, "prod/terraform.tfstate", args["key"])
	assert.Equal(t, "rg-state", args["resource_group_name"])

	// Terragrunt-only bootstrap keys are stripped (the azurerm backend rejects them).
	for _, k := range []string{
		"location", "account_tier", "account_replication_type", "account_kind",
		"access_tier", "tags", "skip_resource_group_creation", "skip_storage_account_creation",
		"skip_container_creation", "skip_versioning", "enable_soft_delete",
		"soft_delete_retention_days", "allow_blob_public_access",
	} {
		_, ok := args[k]
		assert.Falsef(t, ok, "terragrunt-only key %q must not be forwarded to tofu init", k)
	}
}

func TestGetAzureSessionConfig_Mapping(t *testing.T) {
	t.Parallel()

	ext, err := fullConfig().ExtendedAzurermConfig()
	require.NoError(t, err)

	sess := ext.GetAzureSessionConfig()
	assert.Equal(t, "tfstate1234", sess.StorageAccountName)
	assert.Equal(t, "rg-state", sess.ResourceGroupName)
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", sess.SubscriptionID)
	assert.True(t, sess.UseAzureADAuth)
}

func TestGetTFInitArgs_EmptyConfig(t *testing.T) {
	t.Parallel()

	args := azurerm.Config(backend.Config{}).GetTFInitArgs()
	assert.Empty(t, args)
}

func TestGetTFInitArgs_NormalizesSnapshotBool(t *testing.T) {
	t.Parallel()

	cfg := fullConfig()
	cfg["snapshot"] = "true"

	args := cfg.GetTFInitArgs()

	v, ok := args["snapshot"].(bool)
	require.True(t, ok, "snapshot must be coerced to a bool before tofu init")
	assert.True(t, v)
}

func TestRemoteStateConfigCacheKey(t *testing.T) {
	t.Parallel()

	keyFor := func(environment string) string {
		raw := fullConfig()
		if environment != "" {
			raw["environment"] = environment
		}

		ext, err := raw.ExtendedAzurermConfig()
		require.NoError(t, err)

		return ext.RemoteStateConfigAzurerm.CacheKey()
	}

	// The account and container must both appear, so distinct containers in one
	// account never collide.
	assert.Contains(t, keyFor(""), "tfstate1234")
	assert.Contains(t, keyFor(""), "tfstate")

	// Aliases of one cloud share a key.
	assert.Equal(t, keyFor(""), keyFor("public"))
	assert.Equal(t, keyFor("public"), keyFor("AzurePublicCloud"))

	// Different sovereign clouds must NOT share a key, or the second unit would
	// skip its initialization checks against a different account entirely.
	assert.NotEqual(t, keyFor("public"), keyFor("usgovernment"))
	assert.NotEqual(t, keyFor("public"), keyFor("china"))
}
func fullConfig() azurerm.Config {
	return azurerm.Config{
		"storage_account_name": "tfstate1234",
		"container_name":       "tfstate",
		"key":                  "prod/terraform.tfstate",
		"resource_group_name":  "rg-state",
		"subscription_id":      "00000000-0000-0000-0000-000000000000",
		"use_azuread_auth":     true,
		// Terragrunt-only bootstrap keys (must NOT be forwarded to tofu):
		"location":                   "eastus",
		"account_tier":               "Standard",
		"account_replication_type":   "LRS",
		"skip_versioning":            false,
		"enable_soft_delete":         true,
		"soft_delete_retention_days": 14,
		"tags":                       map[string]string{"team": "platform"},
	}
}

// TestParseExtendedAzurermConfig_TrimsWhitespace pins that config values are
// normalized at parse time. azurehelper trims the values it resolves, so an
// untrimmed value here would validate and then fail downstream on a mismatch
// against the trimmed value the Azure clients are bound to.
func TestParseExtendedAzurermConfig_TrimsWhitespace(t *testing.T) {
	t.Parallel()

	cfg := azurerm.Config{
		"storage_account_name": "  tfstate1234\n",
		"container_name":       " tfstate ",
		"key":                  " unit/terraform.tfstate ",
		"resource_group_name":  "\trg\t",
		"location":             " eastus ",
	}

	ext, err := cfg.ParseExtendedAzurermConfig()
	require.NoError(t, err)

	rs := ext.RemoteStateConfigAzurerm
	assert.Equal(t, "tfstate1234", rs.StorageAccountName)
	assert.Equal(t, "tfstate", rs.ContainerName)
	assert.Equal(t, "unit/terraform.tfstate", rs.Key)
	assert.Equal(t, "rg", rs.ResourceGroupName)
	assert.Equal(t, "eastus", ext.Location)
}

// TestValidate_RejectsWhitespaceOnlyRequiredKeys pins that a whitespace-only
// required value is a validation error, not a downstream panic.
func TestValidate_RejectsWhitespaceOnlyRequiredKeys(t *testing.T) {
	t.Parallel()

	cfg := azurerm.Config{
		"storage_account_name": "   ",
		"container_name":       "tfstate",
		"key":                  "unit/terraform.tfstate",
	}

	ext, err := cfg.ParseExtendedAzurermConfig()
	require.NoError(t, err)
	require.Error(t, ext.Validate(), "a whitespace-only storage_account_name must be rejected")
}
