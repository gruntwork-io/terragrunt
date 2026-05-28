// Blob and container data-plane operations.
//
// BlobClient wraps azblob.Client and exposes the small data-plane surface
// the remote-state backend needs (containers, blobs, listing, copy). It
// also remembers an optional bound container so callers fetching state
// files by key do not have to repeat the container on every call.

package azurehelper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	azblobcontainer "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
)

// BlobClient is a thin wrapper around azblob.Client that records the storage
// account name and the AzureConfig that produced it. Construct with
// NewBlobClient.
//
// A BlobClient may optionally be bound to a default container via
// BindContainer. Methods like GetObject and ListBlobsBound use that bound
// container so callers (e.g. the dependency-fetch path that asks for a
// state file by key) do not need to repeat the container on every call.
//
// BlobClient is not safe for concurrent use when callers invoke
// BindContainer: the bound container is stored on the receiver. Construct
// one client per goroutine, or call BindContainer once during setup before
// fanning out.
type BlobClient struct {
	client         *azblob.Client
	config         *AzureConfig
	accountName    string
	endpointSuffix string
	container      string
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
		return nil, ErrAzureConfigRequired
	}

	if cfg.AccountName == "" {
		return nil, ErrStorageAccountRequired
	}

	suffix := endpointSuffix
	if suffix == "" {
		suffix = endpointSuffixForCloud(cfg)
	}

	serviceURL := (&url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.blob.%s", cfg.AccountName, suffix),
	}).String()
	clientOpts := &azblob.ClientOptions{ClientOptions: cfg.ClientOptions}

	var (
		client *azblob.Client
		err    error
	)

	switch cfg.Method {
	case AuthMethodSasToken:
		sasURL := (&url.URL{
			Scheme:   "https",
			Host:     fmt.Sprintf("%s.blob.%s", cfg.AccountName, suffix),
			RawQuery: strings.TrimPrefix(cfg.SasToken, "?"),
		}).String()

		client, err = azblob.NewClientWithNoCredential(sasURL, clientOpts)
	case AuthMethodAccessKey:
		var cred *azblob.SharedKeyCredential

		cred, err = azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccessKey)
		if err != nil {
			return nil, fmt.Errorf("creating shared key credential: %w", err)
		}

		client, err = azblob.NewClientWithSharedKeyCredential(serviceURL, cred, clientOpts)
	case AuthMethodServicePrincipal, AuthMethodOIDC, AuthMethodMSI, AuthMethodAzureAD:
		if cfg.Credential == nil {
			return nil, &CredentialMissingError{Method: cfg.Method}
		}

		client, err = azblob.NewClient(serviceURL, cfg.Credential, clientOpts)
	default:
		return nil, &UnsupportedAuthMethodError{Method: cfg.Method}
	}

	if err != nil {
		return nil, fmt.Errorf("creating blob client: %w", err)
	}

	return &BlobClient{
		client:         client,
		accountName:    cfg.AccountName,
		endpointSuffix: suffix,
		config:         cfg,
	}, nil
}

// AccountName returns the storage account name backing the client.
func (c *BlobClient) AccountName() string { return c.accountName }

// AzClient returns the underlying azblob.Client. Provided so callers needing
// SDK-specific operations (block staging, batch APIs, etc.) can reach in
// without us having to wrap every method.
func (c *BlobClient) AzClient() *azblob.Client { return c.client }

// BindContainer associates a default container with the client. The bound
// container is stored on the receiver, so callers must not invoke
// BindContainer concurrently from multiple goroutines on the same client.
// Returns the receiver to allow fluent chaining:
// NewBlobClient(...).BindContainer("state").
func (c *BlobClient) BindContainer(name string) *BlobClient {
	c.container = name
	return c
}

// Container returns the bound container name, or empty string if no
// container has been bound.
func (c *BlobClient) Container() string { return c.container }

// ContainerExists reports whether the named container exists in the account.
// Returns (false, nil) for ContainerNotFound / ResourceNotFound; other errors
// are returned wrapped.
func (c *BlobClient) ContainerExists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, ErrContainerNameRequired
	}

	_, err := c.client.ServiceClient().NewContainerClient(name).GetProperties(ctx, nil)
	if err == nil {
		return true, nil
	}

	if isErrorCode(err, "ContainerNotFound") {
		return false, nil
	}

	return false, fmt.Errorf("checking container existence: %w", err)
}

// CreateContainer creates the container. If it already exists, this returns
// nil (no-op). Other errors are returned wrapped.
func (c *BlobClient) CreateContainer(ctx context.Context, name string) error {
	if name == "" {
		return ErrContainerNameRequired
	}

	_, err := c.client.CreateContainer(ctx, name, nil)
	if err == nil {
		return nil
	}

	var respErr *azcore.ResponseError

	if errors.As(err, &respErr) && strings.EqualFold(respErr.ErrorCode, "ContainerAlreadyExists") {
		return nil
	}

	return fmt.Errorf("creating container %s: %w", name, err)
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
		return ErrContainerNameRequired
	}

	_, err := c.client.ServiceClient().NewContainerClient(name).Delete(ctx, nil)
	if err == nil || isErrorCode(err, "ContainerNotFound") {
		return nil
	}

	return fmt.Errorf("deleting container %s: %w", name, err)
}

// GetBlob downloads a blob and returns its body as an io.ReadCloser. Caller
// must Close the returned reader.
func (c *BlobClient) GetBlob(ctx context.Context, container, key string) (io.ReadCloser, error) {
	if container == "" || key == "" {
		return nil, ErrBlobKeyRequired
	}

	resp, err := c.client.DownloadStream(ctx, container, key, nil)
	if err != nil {
		return nil, fmt.Errorf("downloading blob %s/%s: %w", container, key, err)
	}

	return resp.Body, nil
}

// PutBlob uploads data to a block blob, overwriting any existing blob.
func (c *BlobClient) PutBlob(ctx context.Context, container, key string, data []byte) error {
	if container == "" || key == "" {
		return ErrBlobKeyRequired
	}

	blockBlob := c.client.ServiceClient().NewContainerClient(container).NewBlockBlobClient(key)
	if _, err := blockBlob.UploadBuffer(ctx, data, nil); err != nil {
		return fmt.Errorf("uploading blob %s/%s: %w", container, key, err)
	}

	return nil
}

// PutBlobFromReader uploads a blob by streaming from reader, avoiding loading
// the full payload into memory.
func (c *BlobClient) PutBlobFromReader(ctx context.Context, container, key string, reader io.Reader) error {
	if container == "" || key == "" {
		return ErrBlobKeyRequired
	}

	blockBlob := c.client.ServiceClient().NewContainerClient(container).NewBlockBlobClient(key)
	if _, err := blockBlob.UploadStream(ctx, reader, nil); err != nil {
		return fmt.Errorf("uploading blob %s/%s: %w", container, key, err)
	}

	return nil
}

// DeleteBlob deletes the named blob. Missing blobs return nil.
func (c *BlobClient) DeleteBlob(ctx context.Context, container, key string) error {
	if container == "" || key == "" {
		return ErrBlobKeyRequired
	}

	_, err := c.client.DeleteBlob(ctx, container, key, nil)
	if err == nil || isErrorCode(err, "BlobNotFound") {
		return nil
	}

	return fmt.Errorf("deleting blob %s/%s: %w", container, key, err)
}

// GetObject downloads a blob from the bound container by key. Convenience
// wrapper for callers that have already bound a container via BindContainer.
func (c *BlobClient) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if c.container == "" {
		return nil, ErrNoContainerBound
	}

	return c.GetBlob(ctx, c.container, key)
}

// ListBlobs returns the keys of all blobs in container whose names start
// with prefix. Pass an empty prefix to enumerate the entire container.
// Pages through ListBlobsFlat results; the full set is materialised in
// memory, so callers should expect O(N) memory in the number of blobs.
func (c *BlobClient) ListBlobs(ctx context.Context, container, prefix string) ([]string, error) {
	if container == "" {
		return nil, ErrContainerNameRequired
	}

	cc := c.client.ServiceClient().NewContainerClient(container)

	opts := &azblobcontainer.ListBlobsFlatOptions{}
	if prefix != "" {
		opts.Prefix = &prefix
	}

	pager := cc.NewListBlobsFlatPager(opts)

	var out []string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing blobs in %s: %w", container, err)
		}

		for _, item := range page.Segment.BlobItems {
			if item != nil && item.Name != nil {
				out = append(out, *item.Name)
			}
		}
	}

	return out, nil
}

// CopyBlob copies srcKey from srcContainer to dstKey in dstContainer using
// the server-side StartCopyFromURL API. Both blobs must live in the same
// storage account that this client is bound to.
//
// The copy is initiated synchronously (StartCopyFromURL returns once Azure
// accepts the request) but Azure may complete the copy asynchronously for
// large blobs; this method does not poll for completion. Callers needing
// to block on completion should poll the destination blob's CopyStatus.
func (c *BlobClient) CopyBlob(ctx context.Context, srcContainer, srcKey, dstContainer, dstKey string) error {
	if srcContainer == "" || srcKey == "" || dstContainer == "" || dstKey == "" {
		return ErrCopyBlobArgsRequired
	}

	srcURL := (&url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s.blob.%s", c.accountName, c.endpointSuffix),
		Path:   "/" + srcContainer + "/" + srcKey,
	}).String()

	dst := c.client.ServiceClient().NewContainerClient(dstContainer).NewBlobClient(dstKey)
	if _, err := dst.StartCopyFromURL(ctx, srcURL, nil); err != nil {
		return fmt.Errorf("copying blob %s/%s to %s/%s: %w", srcContainer, srcKey, dstContainer, dstKey, err)
	}

	return nil
}

// endpointSuffixForCloud returns the blob endpoint host suffix for the cloud
// configured on cfg. Defaults to the public-cloud suffix.
//
// azcore exposes the storage *audience* but not the endpoint *host*, so
// this derives the suffix from the AAD authority host, which is unique
// per sovereign cloud. NOTE: adding a new sovereign cloud (e.g. a future
// Azure region) requires extending this switch — otherwise new clouds
// silently fall through to the public-cloud suffix.
func endpointSuffixForCloud(cfg *AzureConfig) string {
	switch {
	case strings.Contains(cfg.CloudConfig.ActiveDirectoryAuthorityHost, "microsoftonline.us"):
		return "core.usgovcloudapi.net"
	case strings.Contains(cfg.CloudConfig.ActiveDirectoryAuthorityHost, "chinacloudapi.cn"):
		return "core.chinacloudapi.cn"
	default:
		return "core.windows.net"
	}
}
