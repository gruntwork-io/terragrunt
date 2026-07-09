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

// BlobClient is an account-scoped handle over azblob.Client. It constructs
// container-scoped clients (see Container); every container and blob operation
// lives on ContainerClient. It carries no mutable state and is safe for
// concurrent use. Construct with NewBlobClient.
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
		panic(ErrAzureConfigRequired)
	}

	if cfg.AccountName == "" {
		panic(ErrStorageAccountRequired)
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

// Container returns a ContainerClient scoped to name, so subsequent container
// and blob operations never repeat the container. An empty name is a caller
// invariant violation and panics.
func (c *BlobClient) Container(name string) *ContainerClient {
	if name == "" {
		panic(ErrContainerNameRequired)
	}

	return &ContainerClient{blob: c, container: name}
}

// ContainerClient is a BlobClient scoped to a single container. It codifies
// the "a blob lives in a container" invariant in the type system: container
// and blob operations take only a key, never a container argument. Obtain one
// via BlobClient.Container. It holds no mutable state and is safe for
// concurrent use.
type ContainerClient struct {
	blob      *BlobClient
	container string
}

// Name returns the container this client is bound to.
func (cc *ContainerClient) Name() string { return cc.container }

// Exists reports whether the container exists in the account. Returns
// (false, nil) for ContainerNotFound / ResourceNotFound; other errors are
// returned wrapped.
func (cc *ContainerClient) Exists(ctx context.Context) (bool, error) {
	_, err := cc.blob.Client.ServiceClient().NewContainerClient(cc.container).GetProperties(ctx, nil)
	if err == nil {
		return true, nil
	}

	if IsNotFound(err) || isErrorCode(err, "ContainerNotFound") {
		return false, nil
	}

	return false, fmt.Errorf("checking container existence: %w", err)
}

// Create creates the container. If it already exists, this returns nil
// (no-op). Other errors are returned wrapped.
func (cc *ContainerClient) Create(ctx context.Context) error {
	_, err := cc.blob.Client.CreateContainer(ctx, cc.container, nil)
	if err == nil {
		return nil
	}

	if isErrorCode(err, "ContainerAlreadyExists") {
		return nil
	}

	return fmt.Errorf("creating container %s: %w", cc.container, err)
}

// Ensure checks existence first, then creates only if missing. Idempotent.
func (cc *ContainerClient) Ensure(ctx context.Context) error {
	exists, err := cc.Exists(ctx)
	if err != nil {
		return err
	}

	if exists {
		return nil
	}

	return cc.Create(ctx)
}

// EnsureDeleted deletes the container. Idempotent: returns nil if the
// container is already gone (ContainerNotFound).
func (cc *ContainerClient) EnsureDeleted(ctx context.Context) error {
	_, err := cc.blob.Client.ServiceClient().NewContainerClient(cc.container).Delete(ctx, nil)
	if err == nil || isErrorCode(err, "ContainerNotFound") {
		return nil
	}

	return fmt.Errorf("deleting container %s: %w", cc.container, err)
}

// GetBlob downloads a blob and returns its body as an io.ReadCloser. Caller
// must Close the returned reader.
func (cc *ContainerClient) GetBlob(ctx context.Context, key string) (io.ReadCloser, error) {
	if key == "" {
		panic(ErrBlobKeyRequired)
	}

	resp, err := cc.blob.Client.DownloadStream(ctx, cc.container, key, nil)
	if err != nil {
		return nil, fmt.Errorf("downloading blob %s/%s: %w", cc.container, key, err)
	}

	return resp.Body, nil
}

// PutBlob uploads data to a block blob, overwriting any existing blob.
func (cc *ContainerClient) PutBlob(ctx context.Context, key string, data []byte) error {
	if key == "" {
		panic(ErrBlobKeyRequired)
	}

	blockBlob := cc.blob.Client.ServiceClient().NewContainerClient(cc.container).NewBlockBlobClient(key)
	if _, err := blockBlob.UploadBuffer(ctx, data, nil); err != nil {
		return fmt.Errorf("uploading blob %s/%s: %w", cc.container, key, err)
	}

	return nil
}

// PutBlobFromReader uploads a blob by streaming from reader, avoiding loading
// the full payload into memory.
func (cc *ContainerClient) PutBlobFromReader(ctx context.Context, key string, reader io.Reader) error {
	if key == "" {
		panic(ErrBlobKeyRequired)
	}

	blockBlob := cc.blob.Client.ServiceClient().NewContainerClient(cc.container).NewBlockBlobClient(key)
	if _, err := blockBlob.UploadStream(ctx, reader, nil); err != nil {
		return fmt.Errorf("uploading blob %s/%s: %w", cc.container, key, err)
	}

	return nil
}

// EnsureBlobDeleted deletes the named blob. Idempotent: returns nil if the
// blob is already gone (BlobNotFound).
func (cc *ContainerClient) EnsureBlobDeleted(ctx context.Context, key string) error {
	if key == "" {
		panic(ErrBlobKeyRequired)
	}

	_, err := cc.blob.Client.DeleteBlob(ctx, cc.container, key, nil)
	if err == nil || isErrorCode(err, "BlobNotFound") {
		return nil
	}

	return fmt.Errorf("deleting blob %s/%s: %w", cc.container, key, err)
}

// BlobExists reports whether the named blob exists in the container.
func (cc *ContainerClient) BlobExists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		panic(ErrBlobKeyRequired)
	}

	bc := cc.blob.Client.ServiceClient().NewContainerClient(cc.container).NewBlobClient(key)
	if _, err := bc.GetProperties(ctx, nil); err != nil {
		if IsNotFound(err) || isErrorCode(err, "BlobNotFound") {
			return false, nil
		}

		return false, fmt.Errorf("checking blob %s/%s: %w", cc.container, key, err)
	}

	return true, nil
}

// CopyBlob copies srcKey in this container to dstKey in dst by streaming the
// blob through this authenticated client, so it works for private containers
// under any auth method. dst must belong to the same storage account. The
// destination write carries an If-None-Match condition, so an existing
// destination blob is never overwritten; a DestinationBlobExistsError is
// returned instead.
func (cc *ContainerClient) CopyBlob(ctx context.Context, srcKey string, dst *ContainerClient, dstKey string) error {
	assertCopyBlobArgs(srcKey, dst, dstKey)

	body, err := cc.GetBlob(ctx, srcKey)
	if err != nil {
		return err
	}

	defer func() { _ = body.Close() }()

	blockBlob := cc.blob.Client.ServiceClient().NewContainerClient(dst.container).NewBlockBlobClient(dstKey)
	opts := &azblobblockblob.UploadStreamOptions{
		AccessConditions: &azblobblob.AccessConditions{
			ModifiedAccessConditions: &azblobblob.ModifiedAccessConditions{
				IfNoneMatch: to.Ptr(azcore.ETagAny),
			},
		},
	}

	if _, err := blockBlob.UploadStream(ctx, body, opts); err != nil {
		if isErrorCode(err, "BlobAlreadyExists") || isErrorCode(err, "TargetConditionNotMet") || isErrorCode(err, "ConditionNotMet") {
			return &DestinationBlobExistsError{Container: dst.container, Key: dstKey}
		}

		return fmt.Errorf("uploading blob %s/%s: %w", dst.container, dstKey, err)
	}

	return nil
}

// MoveBlobIfNecessary moves srcKey in this container to dstKey in dst by
// copying then deleting the source, mirroring the move semantics of the S3
// and GCS backends: it is a no-op when the source is absent, refuses to
// overwrite an existing destination, copies, then deletes the source. dst
// must belong to the same storage account.
func (cc *ContainerClient) MoveBlobIfNecessary(ctx context.Context, srcKey string, dst *ContainerClient, dstKey string) error {
	srcExists, err := cc.BlobExists(ctx, srcKey)
	if err != nil {
		return err
	}

	if !srcExists {
		return nil
	}

	if err := cc.CopyBlob(ctx, srcKey, dst, dstKey); err != nil {
		return err
	}

	return cc.EnsureBlobDeleted(ctx, srcKey)
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

// ListBlobs returns the keys of all blobs in the container, optionally filtered
// by a WithPrefix option. Pages through ListBlobsFlat results; the full set
// is materialised in memory, so callers should expect O(N) memory in the
// number of blobs.
func (cc *ContainerClient) ListBlobs(ctx context.Context, l log.Logger, opts ...ListBlobsOption) ([]string, error) {
	o := &listBlobsOptions{}
	for _, opt := range opts {
		opt(o)
	}

	c := cc.blob.Client.ServiceClient().NewContainerClient(cc.container)

	flatOpts := &azblobcontainer.ListBlobsFlatOptions{}
	if o.prefix != "" {
		flatOpts.Prefix = &o.prefix
	}

	pager := c.NewListBlobsFlatPager(flatOpts)

	var out []string

	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing blobs in %s: %w", cc.container, err)
		}

		// The SDK always populates Segment; log so an API contract change is visible.
		if page.Segment == nil {
			l.Debugf("azurehelper: list blobs page for %s has no segment; skipping", cc.container)

			continue
		}

		for _, item := range page.Segment.BlobItems {
			if item == nil || item.Name == nil {
				l.Debugf("azurehelper: list blobs item without a name in %s; skipping", cc.container)

				continue
			}

			out = append(out, *item.Name)
		}
	}

	return out, nil
}

// assertCopyBlobArgs panics with a MissingCopyBlobArgsError naming every empty
// CopyBlob argument. The keys come from already-validated config, so an empty
// value is a caller bug rather than a user error.
func assertCopyBlobArgs(srcKey string, dst *ContainerClient, dstKey string) {
	var missing []string

	if srcKey == "" {
		missing = append(missing, "source key")
	}

	if dst == nil {
		missing = append(missing, "destination container")
	}

	if dstKey == "" {
		missing = append(missing, "destination key")
	}

	if len(missing) > 0 {
		panic(&MissingCopyBlobArgsError{Missing: missing})
	}
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
			panic(&CredentialMissingError{Method: cfg.Method})
		}

		client, err := azblob.NewClient(serviceURL, cfg.Credential, clientOpts)
		if err != nil {
			return nil, fmt.Errorf(errCreatingBlobClient, err)
		}

		return client, nil
	default:
		panic(&UnsupportedAuthMethodError{Method: cfg.Method})
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
