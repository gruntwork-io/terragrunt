package azurehelper

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"

	"github.com/gruntwork-io/terragrunt/internal/errors"
)

// BlobClient is a thin wrapper around azblob.Client that records the storage
// account name and the AzureConfig that produced it. Construct with
// NewBlobClient.
type BlobClient struct {
	client      *azblob.Client
	config      *AzureConfig
	accountName string
}

// NewBlobClient builds an *azblob.Client from an AzureConfig and returns it
// wrapped in BlobClient. SAS-token configs use the no-credential constructor;
// access-key configs use a shared key credential; all other methods use the
// AzureConfig.Credential as a token credential.
//
// endpointSuffix selects the blob endpoint host (e.g. "core.windows.net" for
// Azure public cloud, "core.usgovcloudapi.net" for US Government). When empty,
// the suffix is derived from cfg.CloudConfig.
func NewBlobClient(_ context.Context, cfg *AzureConfig, endpointSuffix string) (*BlobClient, error) {
	if cfg == nil {
		return nil, errors.Errorf("azure config is required")
	}

	if cfg.AccountName == "" {
		return nil, errors.Errorf("storage account name is required")
	}

	suffix := endpointSuffix
	if suffix == "" {
		suffix = endpointSuffixForCloud(cfg)
	}

	url := fmt.Sprintf("https://%s.blob.%s", cfg.AccountName, suffix)
	clientOpts := &azblob.ClientOptions{ClientOptions: cfg.ClientOptions}

	var (
		client *azblob.Client
		err    error
	)

	switch cfg.Method {
	case AuthMethodSasToken:
		sas := strings.TrimPrefix(cfg.SasToken, "?")

		client, err = azblob.NewClientWithNoCredential(url+"?"+sas, clientOpts)
	case AuthMethodAccessKey:
		var cred *azblob.SharedKeyCredential

		cred, err = azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccessKey)
		if err != nil {
			return nil, errors.Errorf("creating shared key credential: %w", err)
		}

		client, err = azblob.NewClientWithSharedKeyCredential(url, cred, clientOpts)
	case AuthMethodServicePrincipal, AuthMethodOIDC, AuthMethodMSI, AuthMethodAzureAD, AuthMethodDefault:
		if cfg.Credential == nil {
			return nil, errors.Errorf("azure config has no credential for method %q", cfg.Method)
		}

		client, err = azblob.NewClient(url, cfg.Credential, clientOpts)
	default:
		return nil, errors.Errorf("unsupported azure auth method %q", cfg.Method)
	}

	if err != nil {
		return nil, errors.Errorf("creating blob client: %w", err)
	}

	return &BlobClient{
		client:      client,
		accountName: cfg.AccountName,
		config:      cfg,
	}, nil
}

// AccountName returns the storage account name backing the client.
func (c *BlobClient) AccountName() string { return c.accountName }

// AzClient returns the underlying azblob.Client. Provided so callers needing
// SDK-specific operations (block staging, batch APIs, etc.) can reach in
// without us having to wrap every method.
func (c *BlobClient) AzClient() *azblob.Client { return c.client }

// ContainerExists reports whether the named container exists in the account.
// Returns (false, nil) for ContainerNotFound / ResourceNotFound; other errors
// are returned wrapped.
func (c *BlobClient) ContainerExists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, errors.Errorf("container name is required")
	}

	_, err := c.client.ServiceClient().NewContainerClient(name).GetProperties(ctx, nil)
	if err == nil {
		return true, nil
	}

	if IsNotFound(err) {
		return false, nil
	}

	return false, WrapError(err, "checking container existence")
}

// CreateContainer creates the container. If it already exists, this returns
// nil (no-op). Other errors are returned wrapped.
func (c *BlobClient) CreateContainer(ctx context.Context, name string) error {
	if name == "" {
		return errors.Errorf("container name is required")
	}

	_, err := c.client.CreateContainer(ctx, name, nil)
	if err == nil {
		return nil
	}

	var respErr *azcore.ResponseError

	if errors.As(err, &respErr) && strings.EqualFold(respErr.ErrorCode, "ContainerAlreadyExists") {
		return nil
	}

	return WrapError(err, "creating container "+name)
}

// CreateContainerIfNecessary is a convenience for the common "ensure exists"
// pattern: checks existence first, then creates only if missing.
func (c *BlobClient) CreateContainerIfNecessary(ctx context.Context, name string) error {
	exists, err := c.ContainerExists(ctx, name)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return c.CreateContainer(ctx, name)
}

// DeleteContainer deletes the named container. Missing containers return nil.
func (c *BlobClient) DeleteContainer(ctx context.Context, name string) error {
	if name == "" {
		return errors.Errorf("container name is required")
	}

	_, err := c.client.ServiceClient().NewContainerClient(name).Delete(ctx, nil)
	if err == nil || IsNotFound(err) {
		return nil
	}

	return WrapError(err, "deleting container "+name)
}

// GetBlob downloads a blob and returns its body as an io.ReadCloser. Caller
// must Close the returned reader.
func (c *BlobClient) GetBlob(ctx context.Context, container, key string) (io.ReadCloser, error) {
	if container == "" || key == "" {
		return nil, errors.Errorf("container name and blob key are required")
	}

	resp, err := c.client.DownloadStream(ctx, container, key, nil)
	if err != nil {
		return nil, WrapError(err, "downloading blob "+container+"/"+key)
	}

	return resp.Body, nil
}

// PutBlob uploads data to a block blob, overwriting any existing blob.
func (c *BlobClient) PutBlob(ctx context.Context, container, key string, data []byte) error {
	if container == "" || key == "" {
		return errors.Errorf("container name and blob key are required")
	}

	blockBlob := c.client.ServiceClient().NewContainerClient(container).NewBlockBlobClient(key)
	if _, err := blockBlob.UploadBuffer(ctx, data, nil); err != nil {
		return WrapError(err, "uploading blob "+container+"/"+key)
	}

	return nil
}

// PutBlobFromReader uploads a blob by streaming from reader, avoiding loading
// the full payload into memory.
func (c *BlobClient) PutBlobFromReader(ctx context.Context, container, key string, reader io.Reader) error {
	if container == "" || key == "" {
		return errors.Errorf("container name and blob key are required")
	}

	blockBlob := c.client.ServiceClient().NewContainerClient(container).NewBlockBlobClient(key)
	if _, err := blockBlob.UploadStream(ctx, reader, nil); err != nil {
		return WrapError(err, "uploading blob "+container+"/"+key)
	}

	return nil
}

// DeleteBlob deletes the named blob. Missing blobs return nil.
func (c *BlobClient) DeleteBlob(ctx context.Context, container, key string) error {
	if container == "" || key == "" {
		return errors.Errorf("container name and blob key are required")
	}

	_, err := c.client.DeleteBlob(ctx, container, key, nil)
	if err == nil || IsNotFound(err) {
		return nil
	}

	return WrapError(err, "deleting blob "+container+"/"+key)
}

// endpointSuffixForCloud returns the blob endpoint host suffix for the cloud
// configured on cfg. Defaults to the public-cloud suffix.
func endpointSuffixForCloud(cfg *AzureConfig) string {
	// azcore exposes the storage audience but not the endpoint host, so we
	// derive the suffix from the AAD authority host, which is unique per
	// sovereign cloud.
	switch {
	case strings.Contains(cfg.CloudConfig.ActiveDirectoryAuthorityHost, "microsoftonline.us"):
		return "core.usgovcloudapi.net"
	case strings.Contains(cfg.CloudConfig.ActiveDirectoryAuthorityHost, "chinacloudapi.cn"):
		return "core.chinacloudapi.cn"
	default:
		return "core.windows.net"
	}
}
