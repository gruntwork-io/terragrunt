// Package cache provides a private OpenTofu/Terraform provider cache server.
package cache

import (
	"context"
	"net"
	"net/http"

	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/terraform/cache/controllers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/middleware"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"golang.org/x/sync/errgroup"
)

// Server is a private Terraform cache for provider caching.
type Server struct {
	*router.Router
	*Config

	services           []services.Service
	ProviderController *controllers.ProviderController
}

// NewServer returns a new Server instance.
func NewServer(opts ...Option) *Server {
	cfg := NewConfig(opts...)

	authMiddleware := middleware.KeyAuth(cfg.token)

	downloaderController := &controllers.DownloaderController{
		ProviderHandlers: cfg.providerHandlers,
	}

	providerController := &controllers.ProviderController{
		AuthMiddleware:       authMiddleware,
		DownloaderController: downloaderController,
		ProviderHandlers:     cfg.providerHandlers,
	}

	discoveryController := &controllers.DiscoveryController{
		Endpointers: []controllers.Endpointer{providerController},
	}

	rootRouter := router.New()
	rootRouter.Use(middleware.Logger(cfg.logger))
	rootRouter.Use(middleware.Recover(cfg.logger))
	rootRouter.Register(discoveryController, downloaderController)

	v1Group := rootRouter.Group("v1")
	v1Group.Register(providerController)

	return &Server{
		Router:             rootRouter,
		Config:             cfg,
		services:           cfg.services,
		ProviderController: providerController,
	}
}

// DiscoveryURL looks for the first handler that can handle the given `registryName`,
// which is determined by the include and exclude settings in the `.terraformrc` CLI config file.
// If the handler is found, tries to discover its API endpoints otherwise return the default registry URLs.
func (server *Server) DiscoveryURL(ctx context.Context, registryName string) (*handlers.RegistryURLs, error) {
	return server.providerHandlers.DiscoveryURL(ctx, registryName)
}

// Listen starts listening to the given configuration address. It also automatically chooses a free port if not explicitly specified.
func (server *Server) Listen() (net.Listener, error) {
	ln, err := net.Listen("tcp", server.Addr())
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
		service := service

		errGroup.Go(func() error {
			return service.Run(ctx)
		})
	}

	errGroup.Go(func() error {
		<-ctx.Done()
		server.logger.Infof("Shutting down Terragrunt Cache server...")

		ctx, cancel := context.WithTimeout(ctx, server.shutdownTimeout)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			return errors.New(err)
		}

		return nil
	})

	if err := server.Server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return errors.Errorf("error starting terragrunt cache server: %w", err)
	}

	defer server.logger.Infof("Terragrunt Cache server stopped")

	return errGroup.Wait()
}
