package providercache_test

import (
	"slices"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/providercache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cliconfig"
	"github.com/stretchr/testify/assert"
)

// TestStripProxiedCredentials pins which entries survive, in both directions: dropping an
// unrouted host silently breaks its auth, and keeping a routed one puts a real token on disk.
func TestStripProxiedCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		creds        []cliconfig.ConfigCredentials
		proxiedHosts []string
		want         []cliconfig.ConfigCredentials
	}{
		{
			name:         "proxied host is dropped",
			creds:        []cliconfig.ConfigCredentials{{Name: "registry.opentofu.org", Token: "real-token"}},
			proxiedHosts: []string{"registry.opentofu.org"},
			want:         []cliconfig.ConfigCredentials{},
		},
		{
			name:         "unproxied host is kept",
			creds:        []cliconfig.ConfigCredentials{{Name: "private.example.com", Token: "real-token"}},
			proxiedHosts: []string{"registry.opentofu.org"},
			want:         []cliconfig.ConfigCredentials{{Name: "private.example.com", Token: "real-token"}},
		},
		{
			name: "only the proxied entry is dropped",
			creds: []cliconfig.ConfigCredentials{
				{Name: "registry.opentofu.org", Token: "registry-token"},
				{Name: "private.example.com", Token: "private-token"},
			},
			proxiedHosts: []string{"registry.opentofu.org"},
			want:         []cliconfig.ConfigCredentials{{Name: "private.example.com", Token: "private-token"}},
		},
		{
			name:         "host match ignores case",
			creds:        []cliconfig.ConfigCredentials{{Name: "Registry.OpenTofu.org", Token: "real-token"}},
			proxiedHosts: []string{"registry.opentofu.org"},
			want:         []cliconfig.ConfigCredentials{},
		},
		{
			name:         "no proxied hosts keeps every entry",
			creds:        []cliconfig.ConfigCredentials{{Name: "registry.opentofu.org", Token: "real-token"}},
			proxiedHosts: nil,
			want:         []cliconfig.ConfigCredentials{{Name: "registry.opentofu.org", Token: "real-token"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			unchanged := slices.Clone(tt.creds)

			got := providercache.StripProxiedCredentials(tt.creds, tt.proxiedHosts)

			assert.Equal(t, tt.want, got)
			assert.Equal(t, unchanged, tt.creds, "the caller's slice must not be mutated")
		})
	}
}
