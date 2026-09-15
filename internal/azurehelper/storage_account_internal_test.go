//go:build azure

package azurehelper

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchAccountID(t *testing.T) {
	t.Parallel()

	accounts := []*armstorage.Account{
		nil,
		{},
		{Name: new("other"), ID: new("/subscriptions/sub/resourceGroups/other/providers/Microsoft.Storage/storageAccounts/other")},
		{Name: new("target")},
		{Name: new("target"), ID: new("/subscriptions/sub/resourceGroups/target/providers/Microsoft.Storage/storageAccounts/target")},
	}

	id, found := matchAccountID(accounts, "target")
	require.True(t, found)
	require.Equal(t, "/subscriptions/sub/resourceGroups/target/providers/Microsoft.Storage/storageAccounts/target", id)

	id, found = matchAccountID(accounts, "missing")
	require.False(t, found)
	require.Empty(t, id)
}

func TestWithDefaults_MinimumTLSVersion(t *testing.T) {
	t.Parallel()

	// An unset value falls back to the package default (TLS1_2).
	in := &StorageAccountConfig{}
	in.withDefaults()
	assert.Equal(t, defaultMinimumTLSVersion, in.MinimumTLSVersion)
	assert.Equal(t, "TLS1_2", in.MinimumTLSVersion)

	// An explicit value is preserved.
	in = &StorageAccountConfig{MinimumTLSVersion: "TLS1_3"}
	in.withDefaults()
	assert.Equal(t, "TLS1_3", in.MinimumTLSVersion)
}

func TestMinimumTLSVersionValue(t *testing.T) {
	t.Parallel()

	// An empty value yields the package default; TLS1_2 and TLS1_3 are the only
	// accepted values. The deprecated TLS1_0/TLS1_1 and any other string are
	// user errors.
	for _, tc := range []struct {
		in   string
		want armstorage.MinimumTLSVersion
	}{
		{in: "", want: armstorage.MinimumTLSVersionTLS12},
		{in: "TLS1_2", want: armstorage.MinimumTLSVersionTLS12},
		{in: "TLS1_3", want: armstorage.MinimumTLSVersionTLS13},
	} {
		got, err := minimumTLSVersionValue(tc.in)
		require.NoError(t, err, "input %q", tc.in)
		require.NotNil(t, got)
		assert.Equal(t, tc.want, *got, "input %q", tc.in)
	}

	for _, bad := range []string{"TLS1_0", "TLS1_1", "tls1_2", "garbage"} {
		_, err := minimumTLSVersionValue(bad)

		var unknown *UnknownMinimumTLSVersionError
		require.ErrorAs(t, err, &unknown, "input %q must be rejected", bad)
		assert.Equal(t, bad, unknown.Version)
	}
}
