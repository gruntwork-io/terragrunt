package azurehelper

import (
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
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
