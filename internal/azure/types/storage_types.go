package types

// Account configuration types
type (
	AccountKind     string
	AccountTier     string
	AccessTier      string
	ReplicationType string
)

// StorageAccountConfig represents configuration for a storage account
//
//nolint:govet // fieldalignment: Field order mirrors user-facing configuration blocks.
type StorageAccountConfig struct {
	Name                  string
	ResourceGroupName     string
	Location              string
	AccountKind           AccountKind
	AccountTier           AccountTier
	AccessTier            AccessTier
	ReplicationType       ReplicationType
	EnableVersioning      bool
	AllowBlobPublicAccess bool
	Tags                  map[string]string
}
