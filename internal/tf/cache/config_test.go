package cache_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
)

// TestWithPortSetsListenAddress pins that the configured port reaches the address the server binds to, and that the zero port stays a no-op, since zero means "let the OS choose a free port" and must never undo a port the user asked for.
func TestWithPortSetsListenAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		wantAddr string
		opts     []cache.Option
	}{
		{
			name:     "explicit port",
			opts:     []cache.Option{cache.WithHostname("127.0.0.1"), cache.WithPort(8123)},
			wantAddr: "127.0.0.1:8123",
		},
		{
			name:     "unset port lets the OS choose",
			opts:     []cache.Option{cache.WithHostname("127.0.0.1")},
			wantAddr: "127.0.0.1:0",
		},
		{
			name: "zero port applied after an explicit port",
			opts: []cache.Option{
				cache.WithHostname("127.0.0.1"),
				cache.WithPort(8123),
				cache.WithPort(0),
			},
			wantAddr: "127.0.0.1:8123",
		},
		{
			name: "zero port applied before an explicit port",
			opts: []cache.Option{
				cache.WithHostname("127.0.0.1"),
				cache.WithPort(0),
				cache.WithPort(8123),
			},
			wantAddr: "127.0.0.1:8123",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.wantAddr, cache.NewConfig(tc.opts...).Addr())
		})
	}
}

// TestWithHostnameSetsListenAddress pins that an empty hostname is a no-op, since an empty host would bind the cache server to every interface and expose a per-run token to the rest of the network.
func TestWithHostnameSetsListenAddress(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		wantAddr string
		opts     []cache.Option
	}{
		{
			name:     "unset hostname keeps the default",
			wantAddr: "localhost:0",
		},
		{
			name:     "explicit hostname",
			opts:     []cache.Option{cache.WithHostname("127.0.0.1")},
			wantAddr: "127.0.0.1:0",
		},
		{
			name:     "empty hostname keeps the default",
			opts:     []cache.Option{cache.WithHostname("")},
			wantAddr: "localhost:0",
		},
		{
			name:     "empty hostname applied after an explicit hostname",
			opts:     []cache.Option{cache.WithHostname("127.0.0.1"), cache.WithHostname("")},
			wantAddr: "127.0.0.1:0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.wantAddr, cache.NewConfig(tc.opts...).Addr())
		})
	}
}
