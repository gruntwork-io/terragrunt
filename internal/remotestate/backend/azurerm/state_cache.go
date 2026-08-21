package azurerm

import (
	"context"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/azurehelper"
	"github.com/gruntwork-io/terragrunt/internal/cache"
)

// stateClientCacheKey identifies the per-run shared-key cache in a context.
type stateClientCacheKey struct{}

// stateClientCacheName labels the cache in telemetry.
const stateClientCacheName = "azurermStateClientCache"

// WithStateClientCache scopes shared-key reuse to one run. Every dependency read
// otherwise repeats the ARM ListKeys call for the same storage account, which adds
// a round trip per dependency and invites throttling. Without this the readers
// still work, they just resolve a key each time.
func WithStateClientCache(ctx context.Context) context.Context {
	return context.WithValue(
		ctx,
		stateClientCacheKey{},
		cache.NewCache[*azurehelper.AzureConfig](stateClientCacheName),
	)
}

// stateClientCacheFromContext returns the run's cache, or nil when unscoped.
func stateClientCacheFromContext(ctx context.Context) *cache.Cache[*azurehelper.AzureConfig] {
	instance, _ := ctx.Value(stateClientCacheKey{}).(*cache.Cache[*azurehelper.AzureConfig])

	return instance
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
		string(cfg.Method),
	}, "|")
}
