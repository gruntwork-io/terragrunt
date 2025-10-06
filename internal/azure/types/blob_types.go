package types

// GetObjectInput represents input parameters for getting a blob
type GetObjectInput struct {
	StorageAccountName string
	ContainerName      string
	BlobName           string
}

// GetObjectOutput represents output parameters from getting a blob
type GetObjectOutput struct {
	Properties map[string]string
	Content    []byte
}
