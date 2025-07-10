package types

// StorageAccount represents an Azure storage account
type StorageAccount struct {
	Name              string
	ResourceGroupName string
	Location          string
	Properties        *StorageAccountProperties
}

// StorageAccountProperties represents Azure storage account properties
type StorageAccountProperties struct {
	AccessTier         AccessTier
	EnableVersioning   bool
	IsHnsEnabled       bool
	Kind               AccountKind
	PrimaryEndpoints   StorageEndpoints
	ProvisioningState  string
	SecondaryEndpoints StorageEndpoints
	StatusOfPrimary    string
	StatusOfSecondary  string
	SupportsHttpsOnly  bool
}

// StorageEndpoints represents the endpoints for a storage account
type StorageEndpoints struct {
	Blob  string
	Queue string
	Table string
	File  string
}
