// Package cache provides a private OpenTofu/Terraform provider cache server.
package cache

import (
	"context"
	"net"
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/controllers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/middleware"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/router"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"golang.org/x/sync/errgroup"
)

// Server is a private Terraform cache for provider caching.
type Server struct {
	*router.Router
	*Config
	ProviderController *controllers.ProviderController
	ModuleController   *controllers.ModuleController
	services           []services.Service
}

// NewServer returns a new Server instance.
func NewServer(opts ...Option) *Server {
	cfg := NewConfig(opts...)

	authMiddleware := middleware.KeyAuth(cfg.token)

	downloaderController := &controllers.DownloaderController{
		ProxyProviderHandler: cfg.proxyProviderHandler,
		ProviderService:      cfg.providerService,
	}

	providerController := &controllers.ProviderController{
		AuthMiddleware:              authMiddleware,
		DownloaderController:        downloaderController,
		ProviderHandlers:            cfg.providerHandlers,
		ProxyProviderHandler:        cfg.proxyProviderHandler,
		ProviderService:             cfg.providerService,
		CacheProviderHTTPStatusCode: cfg.cacheProviderHTTPStatusCode,
		Logger:                      cfg.logger,
	}

	moduleController := &controllers.ModuleController{
		AuthMiddleware:     authMiddleware,
		ProxyModuleHandler: cfg.proxyModuleHandler,
		Logger:             cfg.logger,
	}

	endpointers := []controllers.Endpointer{providerController}
	if cfg.proxyModuleHandler != nil {
		endpointers = append(endpointers, moduleController)
	}

	discoveryController := &controllers.DiscoveryController{
		Endpointers: endpointers,
	}

	rootRouter := router.New()
	rootRouter.Use(middleware.Logger(cfg.logger))
	rootRouter.Use(middleware.Recover(cfg.logger))
	rootRouter.Register(discoveryController, downloaderController)

	v1Group := rootRouter.Group("v1")
	v1Group.Register(providerController)

	if cfg.proxyModuleHandler != nil {
		v1Group.Register(moduleController)
	}

	return &Server{
		Router:             rootRouter,
		Config:             cfg,
		services:           []services.Service{cfg.providerService},
		ProviderController: providerController,
		ModuleController:   moduleController,
	}
}

// DiscoveryURL looks for the first handler that can handle the given `registryName`,
// which is determined by the include and exclude settings in the `.terraformrc` CLI config file.
// If the handler is found, tries to discover its API endpoints otherwise return the default registry URLs.
func (server *Server) DiscoveryURL(ctx context.Context, registryName string) (*handlers.RegistryURLs, error) {
	return server.providerHandlers.DiscoveryURL(ctx, registryName)
}

// Listen starts listening to the given configuration address. It also automatically chooses a free port if not explicitly specified.
func (server *Server) Listen(ctx context.Context) (net.Listener, error) {
	lc := &net.ListenConfig{}

	ln, err := lc.Listen(ctx, "tcp", server.Addr())
	if err != nil {
		return nil, errors.New(err)
	}

	server.Server.Addr = ln.Addr().String()

	server.logger.Infof("Terragrunt Cache server is listening on %s", ln.Addr())

	return ln, nil
}

// Run starts the webserver and workers.
func (server *Server) Run(ctx context.Context, ln net.Listener) error {
	server.logger.Infof("Start Terragrunt Cache server")

	errGroup, ctx := errgroup.WithContext(ctx)

	for _, service := range server.services {
		errGroup.Go(func() error {
			return service.Run(ctx)
		})
	}

	errGroup.Go(func() error {
		<-ctx.Done()
		server.logger.Infof("Shutting down Terragrunt Cache server...")

		// The parent ctx is by definition already cancelled here; detach from
		// its cancellation (preserving any values) so http.Server.Shutdown gets
		// the full shutdownTimeout to drain in-flight requests.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), server.shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return errors.New(err)
		}

		return nil
	})

	if err := server.Server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return errors.Errorf("error starting terragrunt cache server: %w", err)
	}

	defer server.logger.Infof("Terragrunt Cache server stopped")

	if err := errGroup.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}
