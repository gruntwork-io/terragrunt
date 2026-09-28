package azurerm

import (
	"context"
	"strings"

	"golang.org/x/sync/singleflight"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/cache"
)

// stateClientCache holds resolved keys and coalesces concurrent account lookups.
type stateClientCache struct {
	configs *cache.Cache[*azurehelper.AzureConfig]
	flight  singleflight.Group
}

// stateClientCacheKey identifies the per-run shared-key cache in a context.
type stateClientCacheKey struct{}

// stateClientCacheName labels the cache in telemetry.
const stateClientCacheName = "azurermStateClientCache"

// WithStateClientCache scopes shared-key reuse to one run.
func WithStateClientCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, stateClientCacheKey{}, &stateClientCache{
		configs: cache.NewCache[*azurehelper.AzureConfig](stateClientCacheName),
	})
}

// stateClientCacheFromContext returns the required run-scoped cache.
func stateClientCacheFromContext(ctx context.Context) (*stateClientCache, error) {
	instance, ok := ctx.Value(stateClientCacheKey{}).(*stateClientCache)
	if !ok || instance == nil {
		return nil, StateClientCacheRequiredError{}
	}

	return instance, nil
}

// sharedKeyCacheKey identifies both the storage account and the identity that
// resolved its key, so one identity never reuses a key fetched by another.
func sharedKeyCacheKey(cfg *azurehelper.AzureConfig) string {
	return strings.Join([]string{
		cfg.CloudConfig.ActiveDirectoryAuthorityHost,
		cfg.SubscriptionID,
		cfg.ResourceGroup,
		cfg.AccountName,
		cfg.TenantID,
		cfg.ClientID,
		cfg.MSIResourceID,
		cfg.CredentialFingerprint,
		string(cfg.Method),
	}, "|")
}
