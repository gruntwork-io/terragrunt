package cache

import (
	"context"
	"net"
	"net/http"
	"net/url"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/controllers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/router"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
	"golang.org/x/sync/errgroup"
)

// Server is a private Terraform cache for provider caching.
type Server struct {
	*http.Server
	config *Config

	Provider           *services.ProviderService
	providerController *controllers.ProviderController
	reverseProxy       *handlers.ReverseProxy
}

// NewServer returns a new Server instance.
func NewServer(opts ...Option) *Server {
	cfg := NewConfig(opts...)

	providerService := services.NewProviderService(cfg.providerCacheDir, cfg.userProviderDir)

	authorization := &handlers.Authorization{
		Token: cfg.token,
	}

	reverseProxy := &handlers.ReverseProxy{}

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
		Server:             &http.Server{Handler: rootRouter},
		config:             cfg,
		Provider:           providerService,
		providerController: providerController,
		reverseProxy:       reverseProxy,
	}
}

// ProviderURL returns a full URL to the provider controller, e.g. http://localhost:5758/v1/providers
func (server *Server) ProviderURL() *url.URL {
	return &url.URL{
		Scheme: "http",
		Host:   server.Addr,
		Path:   server.providerController.Path(),
	}
}

// Listen starts listening to the given configuration address. It also automatically chooses a free port if not explicitly specified.
func (server *Server) Listen() (net.Listener, error) {
	ln, err := net.Listen("tcp", server.config.Addr())
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}

	log.Infof("Terragrunt Cache server is listening on %s", server.Addr)

	server.Addr = ln.Addr().String()
	server.reverseProxy.ServerURL = &url.URL{
		Scheme: "http",
		Host:   ln.Addr().String(),
	}
	return ln, nil
}

// Run starts the webserver and workers.
func (server *Server) Run(ctx context.Context, ln net.Listener) error {
	log.Infof("Start Terragrunt Cache server")

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return server.Provider.RunCacheWorker(ctx)
	})
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
