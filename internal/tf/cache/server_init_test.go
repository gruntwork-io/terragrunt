package cache_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
)

// serveTimeout bounds how long the test waits for a server that should never
// have started serving in the first place.
const serveTimeout = 10 * time.Second

// TestServerRunInitializesServicesBeforeServing pins the order the server
// starts in. A service builds the directories it writes to when it
// initializes, and the listener is already accepting connections by the time
// Run is called, so requests waiting in the backlog are answered the moment
// Serve begins. A service initialized alongside serving can therefore hand a
// request paths it has not built yet, which is how provider archives ended up
// written relative to the working directory.
//
// A service that cannot initialize makes that order observable: the failure
// has to stop the server before it serves, rather than surface once it stops.
func TestServerRunInitializesServicesBeforeServing(t *testing.T) {
	t.Parallel()

	l := logger.CreateLogger()

	// No cache directory, so the provider service cannot initialize.
	server := cache.NewServer(
		cache.WithHostname("127.0.0.1"),
		cache.WithLogger(l),
		cache.WithProviderService(services.NewProviderService("", t.TempDir(), nil, l, venv.OSVenv())),
	)

	ln, err := server.Listen(t.Context())
	require.NoError(t, err)

	done := make(chan error, 1)

	go func() { done <- server.Run(t.Context(), ln) }()

	select {
	case err := <-done:
		require.ErrorIs(t, err, services.ErrCacheDirNotSpecified)

		// Closing the listener the test opened is what proves the server never
		// took it over: a run that reached Serve closes it while shutting
		// down, leaving nothing here to close.
		require.NoError(
			t,
			ln.Close(),
			"the server served on the listener before reporting the failure",
		)
	case <-time.After(serveTimeout):
		t.Fatal("the server neither served nor reported the initialization failure")
	}
}
