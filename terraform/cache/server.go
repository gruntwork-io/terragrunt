package cache

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/controllers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/middleware"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"golang.org/x/sync/errgroup"
)

// Server is a private Terraform cache for provider caching.
type Server struct {
	*http.Server
	config *Config

	services           []services.Service
	providerController *controllers.ProviderController
}

// NewServer returns a new Server instance.
func NewServer(opts ...Option) *Server {
	cfg := NewConfig(opts...)

	authMiddleware := middleware.KeyAuth(cfg.authType, cfg.authToken)

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
	rootRouter.Use(middleware.Logger())
	rootRouter.Use(middleware.Recover())
	rootRouter.Register(discoveryController, downloaderController)

	v1Group := rootRouter.Group("v1")
	v1Group.Register(providerController)

	return &Server{
		Server:             &http.Server{Handler: rootRouter},
		config:             cfg,
		services:           cfg.services,
		providerController: providerController,
	}
}

// ProviderURL returns a full URL to the provider controller, e.g. http://localhost:5758/v1/providers
func (server *Server) ProviderURL() *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   server.Addr,
		Path:   server.providerController.URLPath(),
	}
}

// Listen starts listening to the given configuration address. It also automatically chooses a free port if not explicitly specified.
func (server *Server) Listen() (net.Listener, error) {
	ln, err := net.Listen("tcp", server.config.Addr())
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	server.Addr = ln.Addr().String()

	log.Infof("Terragrunt Cache server is listening on %s", ln.Addr())
	return ln, nil
}

// Run starts the webserver and workers.
func (server *Server) Run(ctx context.Context, ln net.Listener) error {
	log.Infof("Start Terragrunt Cache server")

	errGroup, ctx := errgroup.WithContext(ctx)
	for _, service := range server.services {
		errGroup.Go(func() error {
			return service.Run(ctx)
		})
	}
	errGroup.Go(func() error {
		<-ctx.Done()
		log.Infof("Shutting down Terragrunt Cache server...")

		ctx, cancel := context.WithTimeout(ctx, server.config.shutdownTimeout)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			return errors.WithStackTrace(err)
		}
		return nil
	})

	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return errors.Errorf("error starting terragrunt cache server: %w", err)
	}
	defer log.Infof("Terragrunt Cache server stopped")

	return errGroup.Wait()
}
