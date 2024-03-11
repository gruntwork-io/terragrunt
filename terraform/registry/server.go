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

type Server struct {
	*services.ProviderService
	providerController *controllers.ProviderController

	config  *Config
	handler http.Handler
}

// NewServer returns a new Server instance.
func NewServer(config *Config) *Server {
	providerService := services.NewProviderService()

	authorization := &handlers.Authorization{
		Token: config.Token,
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
		ProviderService:    providerService,
		providerController: providerController,
		handler:            rootRouter,
		config:             config,
	}
}

func (server *Server) ProviderURLPrefix() *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   server.config.Addr(),
		Path:   server.providerController.Prefix(),
	}
}

func (server *Server) Run(ctx context.Context) error {
	log.Infof("Start Private Registry")

	addr := server.config.Addr()
	srv := &http.Server{Addr: addr, Handler: server.handler}

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return server.ProviderService.RunCacheWorker(ctx)
	})
	errGroup.Go(func() error {
		<-ctx.Done()
		log.Infof("Shutting down Private Registry")

		ctx, cancel := context.WithTimeout(ctx, server.config.ShutdownTimeout)
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
