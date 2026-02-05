//go:build sops

package config //nolint:testpackage // bridge file exports unexported symbols for config_test

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/cache"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// Test helpers bridging unexported symbols for package config_test.

func GetSopsDecryptFn() func(string, string) ([]byte, error) {
	return sopsDecryptFn
}

func SetSopsDecryptFn(fn func(string, string) ([]byte, error)) {
	sopsDecryptFn = fn
}

func ResetSopsCache() {
	sopsCache = cache.NewCache[string](sopsCacheName)
}

func SopsDecryptFile(ctx context.Context, pctx *ParsingContext, l log.Logger, params []string) (string, error) {
	return sopsDecryptFile(ctx, pctx, l, params)
}
