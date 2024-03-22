package registry

import (
	"context"
	"net/http"
	"net/url"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/registry/controllers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/router"
	"github.com/gruntwork-io/terragrunt/terraform/registry/services"
	"golang.org/x/sync/errgroup"
)

// Server is a private Terraform registry for provider caching.
type Server struct {
	http.Handler
	config *Config

	providerService    *services.ProviderService
	providerController *controllers.ProviderController
}

// NewServer returns a new Server instance.
func NewServer(providerService *services.ProviderService, opts ...Option) *Server {
	config := NewConfig(opts...)

	authorization := &handlers.Authorization{
		Token: config.token,
	}

	reverseProxy := &handlers.ReverseProxy{
		ServerURL: &url.URL{
			Scheme: "http",
			Host:   config.Addr(),
		},
	}

	downloaderController := &controllers.DownloaderController{
		ReverseProxy:    reverseProxy,
		ProviderService: providerService,
	}

	providerController := &controllers.ProviderController{
		Authorization:   authorization,
		ReverseProxy:    reverseProxy,
		ProviderService: providerService,
		Downloader:      downloaderController,
	}

	discoveryController := &controllers.DiscoveryController{
		Endpointers: []controllers.Endpointer{providerController},
	}

	rootRouter := router.New()
	rootRouter.Register(discoveryController, downloaderController)

	v1Group := rootRouter.Group("v1")
	v1Group.Register(providerController)

	return &Server{
		Handler:            rootRouter,
		config:             config,
		providerService:    providerService,
		providerController: providerController,
	}
}

// ProviderURL returns a full URL to the provider controller, e.g. http://localhost:5758/v1/providers
func (server *Server) ProviderURL() *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   server.config.Addr(),
		Path:   server.providerController.Path(),
	}
}

// Run starts the webserver and workers.
func (server *Server) Run(ctx context.Context) error {
	log.Infof("Start Private Registry")

	addr := server.config.Addr()
	srv := &http.Server{Addr: addr, Handler: server}

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return server.providerService.RunCacheWorker(ctx)
	})
	errGroup.Go(func() error {
		<-ctx.Done()
		log.Infof("Shutting down Private Registry")

		ctx, cancel := context.WithTimeout(ctx, server.config.shutdownTimeout)
		defer cancel()

		srv.SetKeepAlivesEnabled(false)
		if err := srv.Shutdown(ctx); err != nil {
			return errors.WithStackTrace(err)
		}
		return nil
	})

	log.Infof("Private Registry started, listening on %q", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return errors.Errorf("error starting Terrafrom Registry server: %w", err)
	}
	defer log.Infof("Private Registry stopped")

	return errGroup.Wait()
}
