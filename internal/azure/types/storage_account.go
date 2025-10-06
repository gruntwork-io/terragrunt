package types

// StorageAccount represents an Azure storage account
type StorageAccount struct {
	Properties        *StorageAccountProperties
	Name              string
	ResourceGroupName string
	Location          string
}

// StorageAccountProperties represents Azure storage account properties
type StorageAccountProperties struct {
	PrimaryEndpoints   StorageEndpoints
	SecondaryEndpoints StorageEndpoints
	ProvisioningState  string
	StatusOfPrimary    string
	StatusOfSecondary  string
	Kind               AccountKind
	AccessTier         AccessTier
	SupportsHTTPSOnly  bool
	EnableVersioning   bool
	IsHnsEnabled       bool
}

// StorageEndpoints represents the endpoints for a storage account
type StorageEndpoints struct {
	Blob  string
	Queue string
	Table string
	File  string
}
