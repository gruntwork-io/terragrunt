// Package azurehelper -- blob and container data-plane operations.
//
// BlobClient wraps azblob.Client and exposes the small data-plane surface
// the remote-state backend needs (containers, blobs, listing, copy). It
// also remembers an optional bound container so callers fetching state
// files by key (e.g. PR 3's dependency-fetch path) do not have to repeat
// the container on every call.
package azurehelper

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	azblobcontainer "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"

	"github.com/gruntwork-io/terragrunt/internal/errors"
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
	case AuthMethodServicePrincipal, AuthMethodOIDC, AuthMethodMSI, AuthMethodAzureAD:
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

// GetObject downloads a blob from the bound container by key. Convenience
// wrapper for callers that have already bound a container via
// BindContainer (e.g. PR 3's dependency-fetch path).
func (c *BlobClient) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if c.container == "" {
		return nil, errors.Errorf("BlobClient has no container bound; call BindContainer first or use GetBlob")
	}

	return c.GetBlob(ctx, c.container, key)
}

// ListBlobs returns the keys of all blobs in container whose names start
// with prefix. Pass an empty prefix to enumerate the entire container.
// Pages through ListBlobsFlat results; the full set is materialised in
// memory, so callers should expect O(N) memory in the number of blobs.
func (c *BlobClient) ListBlobs(ctx context.Context, container, prefix string) ([]string, error) {
	if container == "" {
		return nil, errors.Errorf("container name is required")
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
			return nil, WrapError(err, "listing blobs in "+container)
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
		return errors.Errorf("source and destination container/key are required")
	}

	// Build the source URL via net/url so blob names containing
	// characters that need percent-encoding (spaces, '#', '?',
	// non-ASCII, etc.) produce a valid URL. The blob path is the
	// virtual key including any '/' separators; net/url's URL.String
	// encodes each path segment without escaping the slashes.
	srcURL := (&url.URL{
		Scheme: "https",
		Host:   c.accountName + ".blob." + c.endpointSuffix,
		Path:   "/" + srcContainer + "/" + srcKey,
	}).String()

	dst := c.client.ServiceClient().NewContainerClient(dstContainer).NewBlobClient(dstKey)
	if _, err := dst.StartCopyFromURL(ctx, srcURL, nil); err != nil {
		return WrapError(err, fmt.Sprintf("copying blob %s/%s to %s/%s", srcContainer, srcKey, dstContainer, dstKey))
	}

	return nil
}

// WaitForCopy polls the destination blob until its copy operation reaches
// a terminal state (success, aborted or failed) or the context is
// cancelled. Returns nil on success, an error on aborted/failed, or the
// context error on timeout. Suitable for callers that need to ensure a
// CopyBlob has completed before, e.g., deleting the source blob.
func (c *BlobClient) WaitForCopy(ctx context.Context, dstContainer, dstKey string) error {
	const pollInterval = 200 * time.Millisecond

	dst := c.client.ServiceClient().NewContainerClient(dstContainer).NewBlobClient(dstKey)

	for {
		props, err := dst.GetProperties(ctx, nil)
		if err != nil {
			return WrapError(err, fmt.Sprintf("polling copy status of %s/%s", dstContainer, dstKey))
		}

		if props.CopyStatus == nil {
			return nil
		}

		switch *props.CopyStatus {
		case blob.CopyStatusTypeSuccess:
			return nil
		case blob.CopyStatusTypeAborted, blob.CopyStatusTypeFailed:
			desc := ""
			if props.CopyStatusDescription != nil {
				desc = *props.CopyStatusDescription
			}

			return errors.Errorf("copy of %s/%s ended in state %s: %s", dstContainer, dstKey, *props.CopyStatus, desc)
		case blob.CopyStatusTypePending:
			// keep polling
		}

		select {
		case <-ctx.Done():
			return errors.Errorf("context cancelled while waiting for copy of %s/%s: %w", dstContainer, dstKey, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
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
