package types

// Account configuration types
type (
	AccountKind     string
	AccountTier     string
	AccessTier      string
	ReplicationType string
)

// StorageAccountConfig represents configuration for a storage account
type StorageAccountConfig struct {
	Name                  string
	ResourceGroupName     string
	Location              string
	EnableVersioning      bool
	AllowBlobPublicAccess bool
	AccountKind           AccountKind
	AccountTier           AccountTier
	AccessTier            AccessTier
	ReplicationType       ReplicationType
	Tags                  map[string]string
}
