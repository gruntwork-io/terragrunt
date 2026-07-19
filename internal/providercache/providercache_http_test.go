//go:build http

// Opt-in variant of TestProviderCache that talks to the real
// registry.terraform.io and releases.hashicorp.com instead of the in-memory
// registry. Run with `go test -tags http` when real-network coverage is
// wanted; the default build stays fully offline.

package providercache_test

import (
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/vhttp"
)

func TestHTTPProviderCache(t *testing.T) {
	t.Parallel()

	testProviderCache(t, vhttp.NewOSClient())
}
