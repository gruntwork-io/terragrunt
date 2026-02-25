package handlers_test

import (
	"context"
	"errors"
	"net"
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
		{err: syscall.ECONNREFUSED, desc: "connection refused", expected: true},
		{err: syscall.ECONNRESET, desc: "connection reset by peer", expected: true},
		{err: syscall.ECONNABORTED, desc: "connection aborted", expected: true},
		{err: syscall.ENETUNREACH, desc: "network is unreachable", expected: true},
		{err: errors.New("get \"https://registry.terraform.io/.well-known/terraform.json\": dial tcp: lookup registry.terraform.io on 185.12.64.1:53: dial udp 185.12.64.1:53: connect: network is unreachable"), desc: "network is unreachable", expected: true},
		{err: errors.New("get \"https://registry.terraform.io/.well-known/terraform.json\": read tcp 10.10.230.10:58328->10.245.10.15:443: read: connection reset by peer"), desc: "network is unreachable", expected: true},
		{err: &net.DNSError{Err: "no such host", Name: "registry.terraform.io", IsNotFound: true}, desc: "DNS not found", expected: true},
		{err: &net.DNSError{Err: "server misbehaving", Name: "blocked-registry.invalid", IsTemporary: true}, desc: "DNS temporary failure", expected: true},
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
