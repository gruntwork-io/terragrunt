// Blob and container data-plane operations.

package azurehelper

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	azblobblob "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	azblobblockblob "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	azblobcontainer "github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"

	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// BlobClient wraps azblob.Client with the data-plane surface the remote-state
// backend needs (containers, blobs, listing, copy). It carries no mutable
// state and is safe for concurrent use. Construct with NewBlobClient.
type BlobClient struct {
	// Client is the underlying Azure SDK client, exposed for SDK-specific
	// operations this wrapper does not cover.
	Client *azblob.Client
	// AccountName is the storage account this client is bound to.
	AccountName string
}

// NewBlobClient builds an *azblob.Client from an AzureConfig and returns it
// wrapped in BlobClient. SAS-token configs use the no-credential constructor;
// access-key configs use a shared key credential; all other methods use the
// AzureConfig.Credential as a token credential. The blob endpoint host
// (e.g. "core.windows.net" / "core.usgovcloudapi.net") is derived from
// cfg.CloudConfig.
func NewBlobClient(cfg *AzureConfig) (*BlobClient, error) {
	if cfg == nil {
		return nil, ErrAzureConfigRequired
	}

	if cfg.AccountName == "" {
		return nil, ErrStorageAccountRequired
	}

	suffix, err := endpointSuffixForCloud(cfg)
	if err != nil {
		return nil, err
	}

	client, err := newAzblobClient(cfg, suffix)
	if err != nil {
		return nil, err
	}

	return &BlobClient{
		Client:      client,
		AccountName: cfg.AccountName,
	}, nil
}

// ContainerExists reports whether the named container exists in the account.
// Returns (false, nil) for ContainerNotFound / ResourceNotFound; other errors
// are returned wrapped.
func (c *BlobClient) ContainerExists(ctx context.Context, name string) (bool, error) {
	if name == "" {
		return false, ErrContainerNameRequired
	}

	_, err := c.Client.ServiceClient().NewContainerClient(name).GetProperties(ctx, nil)
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

	_, err := c.Client.CreateContainer(ctx, name, nil)
	if err == nil {
		return nil
	}

	if isErrorCode(err, "ContainerAlreadyExists") {
		return nil
	}

	return fmt.Errorf("creating container %s: %w", name, err)
}

// EnsureContainer checks existence first, then creates only if missing.
// Idempotent.
func (c *BlobClient) EnsureContainer(ctx context.Context, name string) error {
	exists, err := c.ContainerExists(ctx, name)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return c.CreateContainer(ctx, name)
}

// EnsureContainerDeleted deletes the named container. Idempotent: returns
// nil if the container is already gone (ContainerNotFound).
func (c *BlobClient) EnsureContainerDeleted(ctx context.Context, name string) error {
	if name == "" {
		return ErrContainerNameRequired
	}

	_, err := c.Client.ServiceClient().NewContainerClient(name).Delete(ctx, nil)
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

	resp, err := c.Client.DownloadStream(ctx, container, key, nil)
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

	blockBlob := c.Client.ServiceClient().NewContainerClient(container).NewBlockBlobClient(key)
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

	blockBlob := c.Client.ServiceClient().NewContainerClient(container).NewBlockBlobClient(key)
	if _, err := blockBlob.UploadStream(ctx, reader, nil); err != nil {
		return fmt.Errorf("uploading blob %s/%s: %w", container, key, err)
	}

	return nil
}

// EnsureBlobDeleted deletes the named blob. Idempotent: returns nil if the
// blob is already gone (BlobNotFound).
func (c *BlobClient) EnsureBlobDeleted(ctx context.Context, container, key string) error {
	if container == "" || key == "" {
		return ErrBlobKeyRequired
	}

	_, err := c.Client.DeleteBlob(ctx, container, key, nil)
	if err == nil || isErrorCode(err, "BlobNotFound") {
		return nil
	}

	return fmt.Errorf("deleting blob %s/%s: %w", container, key, err)
}

// BlobExists reports whether the named blob exists in container.
func (c *BlobClient) BlobExists(ctx context.Context, container, key string) (bool, error) {
	if container == "" || key == "" {
		return false, ErrBlobKeyRequired
	}

	bc := c.Client.ServiceClient().NewContainerClient(container).NewBlobClient(key)
	if _, err := bc.GetProperties(ctx, nil); err != nil {
		if IsNotFound(err) || isErrorCode(err, "BlobNotFound") {
			return false, nil
		}

		return false, fmt.Errorf("checking blob %s/%s: %w", container, key, err)
	}

	return true, nil
}

// MoveBlobIfNecessary moves srcKey in srcContainer to dstKey in dstContainer
// by copying then deleting the source, mirroring the move semantics of the S3
// and GCS backends: it is a no-op when the source is absent, refuses to
// overwrite an existing destination, copies, then deletes the source. The
// destination write is conditional (If-None-Match), so a concurrent writer
// cannot be overwritten. Both blobs must live in the storage account this
// client is bound to.
func (c *BlobClient) MoveBlobIfNecessary(ctx context.Context, srcContainer, srcKey, dstContainer, dstKey string) error {
	srcExists, err := c.BlobExists(ctx, srcContainer, srcKey)
	if err != nil {
		return err
	}

	if !srcExists {
		return nil
	}

	if err := c.CopyBlob(ctx, srcContainer, srcKey, dstContainer, dstKey); err != nil {
		return err
	}

	return c.EnsureBlobDeleted(ctx, srcContainer, srcKey)
}

// ListBlobsOption configures ListBlobs.
type ListBlobsOption func(*listBlobsOptions)

type listBlobsOptions struct {
	prefix string
}

// WithPrefix restricts ListBlobs to blob names beginning with prefix.
func WithPrefix(prefix string) ListBlobsOption {
	return func(o *listBlobsOptions) { o.prefix = prefix }
}

// ListBlobs returns the keys of all blobs in container, optionally filtered
// by a WithPrefix option. Pages through ListBlobsFlat results; the full set
// is materialised in memory, so callers should expect O(N) memory in the
// number of blobs.
func (c *BlobClient) ListBlobs(ctx context.Context, l log.Logger, container string, opts ...ListBlobsOption) ([]string, error) {
	if container == "" {
		return nil, ErrContainerNameRequired
	}

	o := &listBlobsOptions{}
	for _, opt := range opts {
		opt(o)
	}

	cc := c.Client.ServiceClient().NewContainerClient(container)

	flatOpts := &azblobcontainer.ListBlobsFlatOptions{}
	if o.prefix != "" {
		flatOpts.Prefix = &o.prefix
	}

	pager := cc.NewListBlobsFlatPager(flatOpts)

	var out []string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing blobs in %s: %w", container, err)
		}

		// The SDK always populates Segment; log so an API contract change is visible.
		if page.Segment == nil {
			l.Debugf("azurehelper: list blobs page for %s has no segment; skipping", container)

			continue
		}

		for _, item := range page.Segment.BlobItems {
			if item == nil || item.Name == nil {
				l.Debugf("azurehelper: list blobs item without a name in %s; skipping", container)

				continue
			}

			out = append(out, *item.Name)
		}
	}

	return out, nil
}

// CopyBlob copies srcKey from srcContainer to dstKey in dstContainer by
// streaming the blob through this authenticated client, so it works for
// private containers under any auth method. The destination write carries an
// If-None-Match condition, so an existing destination blob is never
// overwritten; a DestinationBlobExistsError is returned instead. Both blobs
// must live in the storage account this client is bound to.
func (c *BlobClient) CopyBlob(ctx context.Context, srcContainer, srcKey, dstContainer, dstKey string) error {
	if err := validateCopyBlobArgs(srcContainer, srcKey, dstContainer, dstKey); err != nil {
		return err
	}

	body, err := c.GetBlob(ctx, srcContainer, srcKey)
	if err != nil {
		return err
	}

	defer func() { _ = body.Close() }()

	blockBlob := c.Client.ServiceClient().NewContainerClient(dstContainer).NewBlockBlobClient(dstKey)
	opts := &azblobblockblob.UploadStreamOptions{
		AccessConditions: &azblobblob.AccessConditions{
			ModifiedAccessConditions: &azblobblob.ModifiedAccessConditions{
				IfNoneMatch: to.Ptr(azcore.ETagAny),
			},
		},
	}

	if _, err := blockBlob.UploadStream(ctx, body, opts); err != nil {
		if isErrorCode(err, "BlobAlreadyExists") || isErrorCode(err, "TargetConditionNotMet") || isErrorCode(err, "ConditionNotMet") {
			return &DestinationBlobExistsError{Container: dstContainer, Key: dstKey}
		}

		return fmt.Errorf("uploading blob %s/%s: %w", dstContainer, dstKey, err)
	}

	return nil
}

// validateCopyBlobArgs returns a MissingCopyBlobArgsError naming every empty
// CopyBlob argument, or nil when all four are present.
func validateCopyBlobArgs(srcContainer, srcKey, dstContainer, dstKey string) error {
	var missing []string

	for _, arg := range []struct {
		name  string
		value string
	}{
		{name: "source container", value: srcContainer},
		{name: "source key", value: srcKey},
		{name: "destination container", value: dstContainer},
		{name: "destination key", value: dstKey},
	} {
		if arg.value == "" {
			missing = append(missing, arg.name)
		}
	}

	if len(missing) > 0 {
		return &MissingCopyBlobArgsError{Missing: missing}
	}

	return nil
}

// newAzblobClient dispatches azblob client construction by auth method,
// returning the constructed *azblob.Client or a descriptive error.
func newAzblobClient(cfg *AzureConfig, suffix string) (*azblob.Client, error) {
	const errCreatingBlobClient = "creating blob client: %w"

	host := fmt.Sprintf("%s.blob.%s", cfg.AccountName, suffix)
	serviceURL := (&url.URL{Scheme: "https", Host: host}).String()
	clientOpts := &azblob.ClientOptions{ClientOptions: cfg.ClientOptions}

	switch cfg.Method {
	case AuthMethodSasToken:
		sasURL := (&url.URL{
			Scheme:   "https",
			Host:     host,
			RawQuery: strings.TrimPrefix(cfg.SasToken, "?"),
		}).String()

		client, err := azblob.NewClientWithNoCredential(sasURL, clientOpts)
		if err != nil {
			return nil, fmt.Errorf(errCreatingBlobClient, err)
		}

		return client, nil
	case AuthMethodAccessKey:
		cred, err := azblob.NewSharedKeyCredential(cfg.AccountName, cfg.AccessKey)
		if err != nil {
			return nil, fmt.Errorf("creating shared key credential: %w", err)
		}

		client, err := azblob.NewClientWithSharedKeyCredential(serviceURL, cred, clientOpts)
		if err != nil {
			return nil, fmt.Errorf(errCreatingBlobClient, err)
		}

		return client, nil
	case AuthMethodServicePrincipal, AuthMethodOIDC, AuthMethodMSI, AuthMethodAzureAD:
		if cfg.Credential == nil {
			return nil, &CredentialMissingError{Method: cfg.Method}
		}

		client, err := azblob.NewClient(serviceURL, cfg.Credential, clientOpts)
		if err != nil {
			return nil, fmt.Errorf(errCreatingBlobClient, err)
		}

		return client, nil
	default:
		return nil, &UnsupportedAuthMethodError{Method: cfg.Method}
	}
}

// endpointSuffixForCloud returns the blob endpoint host suffix for the cloud
// configured on cfg. azcore exposes the storage audience but not the endpoint
// host, so the suffix is derived from the AAD authority host, which is unique
// per cloud; an unrecognized non-empty authority host fails loudly instead of
// silently falling back to the public cloud.
func endpointSuffixForCloud(cfg *AzureConfig) (string, error) {
	host := cfg.CloudConfig.ActiveDirectoryAuthorityHost

	switch {
	case strings.Contains(host, "microsoftonline.us"):
		return "core.usgovcloudapi.net", nil
	case strings.Contains(host, "chinacloudapi.cn"):
		return "core.chinacloudapi.cn", nil
	case host == "" || strings.Contains(host, "microsoftonline.com"):
		return "core.windows.net", nil
	default:
		return "", &UnknownAuthorityHostError{Host: host}
	}
}
