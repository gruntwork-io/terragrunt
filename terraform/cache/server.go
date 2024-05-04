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
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/sync/errgroup"
)

const (
	// The status returned when making a request to the caching provider.
	// It is needed to prevent further loading of providers by terraform, and at the same time make sure that the request was processed successfully.
	CacheProviderHTTPStatusCode = http.StatusLocked
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

	regsitryHanlder := handlers.NewRegistry(providerService, CacheProviderHTTPStatusCode)
	networkMirrorHandler := handlers.NewNetworkMirror(providerService, CacheProviderHTTPStatusCode)

	downloaderController := &controllers.DownloaderController{
		RegistryHandler: regsitryHanlder,
		ProviderService: providerService,
	}

	providerController := &controllers.ProviderController{
		Authorization:        authorization,
		RegistryHandler:      regsitryHanlder,
		NetworkMirrorHandler: networkMirrorHandler,
	}

	discoveryController := &controllers.DiscoveryController{
		Endpointers: []controllers.Endpointer{providerController},
	}

	rootRouter := router.New()
	rootRouter.Register(discoveryController, downloaderController)

	v1Group := rootRouter.Group("v1")
	v1Group.Register(providerController)
	rootRouter.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
		LogStatus:   true,
		LogURI:      true,
		LogError:    true,
		HandleError: true, // forwards error to the global error handler, so it can decide appropriate status code
		LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
			log := log.WithField("uri", v.URI).WithField("status", v.Status)
			if v.Error != nil {
				log.Errorf("Cache server was unable to process the received request, %s", v.Error.Error())
			} else {
				log.Tracef("Cache server received request")
			}
			return nil
		},
	}))
	rootRouter.Use(handlers.Recover())

	return &Server{
		Server:             &http.Server{Handler: rootRouter},
		config:             cfg,
		Provider:           providerService,
		providerController: providerController,
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

	log.Infof("Terragrunt Cache server is listening on %s", ln.Addr())
	return ln, nil
}

// Run starts the webserver and workers.
func (server *Server) Run(ctx context.Context, ln net.Listener) error {
	log.Infof("Start Terragrunt Cache server")

	server.Addr = ln.Addr().String()

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
