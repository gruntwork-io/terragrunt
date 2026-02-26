package handlers_test

import (
	"context"
	"errors"
	"net"
	"net/url"
	"syscall"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsOfflineError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		err      error
		desc     string
		expected bool
	}{
		// *url.Error wrapping various transport failures — all caught by the single *url.Error check.
		{err: &url.Error{Op: "Get", URL: "https://registry.terraform.io/.well-known/terraform.json", Err: syscall.ECONNREFUSED}, desc: "connection refused", expected: true},
		{err: &url.Error{Op: "Get", URL: "https://registry.terraform.io/.well-known/terraform.json", Err: syscall.ECONNRESET}, desc: "connection reset", expected: true},
		{err: &url.Error{Op: "Get", URL: "https://registry.terraform.io/.well-known/terraform.json", Err: syscall.ENETUNREACH}, desc: "network unreachable", expected: true},
		{err: &url.Error{Op: "Get", URL: "https://registry.terraform.io/.well-known/terraform.json", Err: &net.DNSError{Err: "no such host", Name: "registry.terraform.io", IsNotFound: true}}, desc: "DNS not found", expected: true},
		{err: &url.Error{Op: "Get", URL: "https://registry.terraform.io/.well-known/terraform.json", Err: &net.DNSError{Err: "server misbehaving", Name: "blocked-registry.invalid"}}, desc: "DNS temporary failure", expected: true},
		{err: &url.Error{Op: "Get", URL: "https://registry.terraform.io/.well-known/terraform.json", Err: errors.New("tls: failed to verify certificate")}, desc: "TLS error", expected: true},
		// Non-transport errors — should NOT be treated as offline.
		{err: errors.New("random error"), desc: "a random error that should not be offline", expected: false},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			result := handlers.IsOfflineError(tc.err)
			assert.Equal(t, tc.expected, result, "Expected result for %v is %v", tc.desc, tc.expected)
		})
	}
}

// TestProviderHandlers_DiscoveryURL_WithNetworkMirrorForBlockedRegistry reproduces the bug from issue #5613.
// When a network_mirror is configured for a registry that is unreachable (e.g., blocked by DNS),
// DiscoveryURL should return DefaultRegistryURLs without attempting to contact the registry.
// The ".invalid" TLD is guaranteed to never resolve per RFC 2606.
func TestProviderHandlers_DiscoveryURL_WithNetworkMirrorForBlockedRegistry(t *testing.T) {
	t.Parallel()

	cfg := &cliconfig.Config{
		ProviderInstallation: &cliconfig.ProviderInstallation{
			Methods: cliconfig.ProviderInstallationMethods{
				cliconfig.NewProviderInstallationNetworkMirror(
					"https://mirror.example.com/providers/",
					[]string{"blocked-registry.invalid/*/*"},
					nil,
				),
			},
		},
	}

	providerHandlers, err := handlers.NewProviderHandlers(cfg, log.New(), nil)
	require.NoError(t, err)

	urls, err := providerHandlers.DiscoveryURL(context.Background(), "blocked-registry.invalid")
	require.NoError(t, err)
	assert.Equal(t, handlers.DefaultRegistryURLs, urls)
}
