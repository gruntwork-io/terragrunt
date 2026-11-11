package config

import (
	"context"

	azurermbackend "github.com/gruntwork-io/terragrunt/internal/remotestate/backend/azurerm"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/internal/runner/run"
	"github.com/gruntwork-io/terragrunt/internal/util"
)

type configKey byte

const (
	// HclCacheContextKey is the context key of the cache of parsed HCL files.
	HclCacheContextKey configKey = iota
	// TerragruntConfigCacheContextKey is the context key of the cache of partially parsed configs.
	TerragruntConfigCacheContextKey configKey = iota
	// RunCmdCacheContextKey is the context key of the cache of `run_cmd` results.
	RunCmdCacheContextKey configKey = iota
	// DependencyOutputCacheContextKey is the context key of the cache of each dependency's `enabled` value and
	// inputs, keyed by the dependency's config path.
	DependencyOutputCacheContextKey configKey = iota
	// JSONOutputCacheContextKey is the context key of the cache of dependency outputs as JSON, keyed by the
	// dependency's config path.
	JSONOutputCacheContextKey configKey = iota
	// OutputLocksContextKey is the context key of the per-unit locks held while fetching dependency outputs.
	OutputLocksContextKey configKey = iota
	// SopsCacheContextKey is the context key of the cache of `sops_decrypt_file` results, keyed by file path.
	SopsCacheContextKey configKey = iota
	// SopsLocksContextKey is the context key of the per-file locks held while a file is decrypted.
	SopsLocksContextKey configKey = iota
	// AutoIncludeSuffixCacheContextKey is the context key of the cache of autoinclude suffixes added to partial
	// parse cache keys.
	AutoIncludeSuffixCacheContextKey configKey = iota
	// ParentFileProbeCacheContextKey is the context key of the cache of `find_in_parent_folders` file existence
	// checks.
	ParentFileProbeCacheContextKey configKey = iota
	// ReadTerragruntConfigCacheContextKey is the context key of the cache of `read_terragrunt_config` results.
	ReadTerragruntConfigCacheContextKey configKey = iota
	// RunCmdLocksContextKey is the context key of the per-command locks that make concurrent identical `run_cmd`
	// calls wait for the first.
	RunCmdLocksContextKey configKey = iota

	hclCacheName               = "hclCache"
	configCacheName            = "configCache"
	runCmdCacheName            = "runCmdCache"
	dependencyOutputCacheName  = "dependencyOutputCache"
	jsonOutputCacheName        = "jsonOutputCache"
	sopsCacheName              = "sopsCache"
	autoIncludeSuffixCacheName = "autoIncludeSuffixCache"
	parentFileProbeCacheName   = "parentFileProbeCache"
	readTerragruntConfigName   = "readTerragruntConfigCache"
)

// WithCaches installs every cache Terragrunt reads while parsing and running a command.
func WithCaches(ctx context.Context) context.Context {
	return run.WithRunVersionCache(withConfigValues(ctx))
}

// withConfigValues adds to context default values for configuration.
func withConfigValues(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, HclCacheContextKey, cache.NewCache[*CachedHCLFile](hclCacheName))
	ctx = context.WithValue(
		ctx,
		TerragruntConfigCacheContextKey,
		cache.NewCache[*TerragruntConfig](configCacheName),
	)
	ctx = context.WithValue(
		ctx,
		RunCmdCacheContextKey,
		cache.NewCache[*RunCmdCacheEntry](runCmdCacheName),
	)
	ctx = context.WithValue(ctx, RunCmdLocksContextKey, util.NewKeyLocks())
	ctx = azurermbackend.WithStateClientCache(ctx)
	ctx = context.WithValue(
		ctx,
		DependencyOutputCacheContextKey,
		cache.NewCache[*dependencyOutputCache](dependencyOutputCacheName),
	)
	ctx = context.WithValue(
		ctx,
		JSONOutputCacheContextKey,
		cache.NewCache[[]byte](jsonOutputCacheName),
	)
	ctx = context.WithValue(ctx, OutputLocksContextKey, util.NewKeyLocks())
	ctx = context.WithValue(ctx, SopsCacheContextKey, cache.NewCache[string](sopsCacheName))
	ctx = context.WithValue(ctx, SopsLocksContextKey, util.NewKeyLocks())
	ctx = context.WithValue(
		ctx,
		AutoIncludeSuffixCacheContextKey,
		cache.NewCache[string](autoIncludeSuffixCacheName),
	)
	ctx = context.WithValue(
		ctx,
		ParentFileProbeCacheContextKey,
		cache.NewCache[bool](parentFileProbeCacheName),
	)
	ctx = context.WithValue(
		ctx,
		ReadTerragruntConfigCacheContextKey,
		cache.NewCache[*readTerragruntConfigResult](readTerragruntConfigName),
	)

	return ctx
}
