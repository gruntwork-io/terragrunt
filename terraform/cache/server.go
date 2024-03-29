package cache

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

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
	config   *Config
	listener net.Listener

	Provider           *services.ProviderService
	providerController *controllers.ProviderController
}

// NewServer returns a new Server instance.
func NewServer(opts ...Option) *Server {
	cfg := NewConfig(opts...)

	providerService := services.NewProviderService(cfg.providerCacheDir, cfg.providerCompleteLock)

	// authorization := &handlers.Authorization{
	// 	Token: cfg.token,
	// }

	reverseProxy := &handlers.ReverseProxy{
		ServerURL: &url.URL{
			Scheme: "http",
			Host:   cfg.Addr(),
		},
	}

	downloaderController := &controllers.DownloaderController{
		ReverseProxy:    reverseProxy,
		ProviderService: providerService,
	}

	providerController := &controllers.ProviderController{
		//Authorization:   authorization,
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

func (server *Server) Listen(ctx context.Context) error {
	debugCtx, debugCancel := context.WithCancel(ctx)
	defer debugCancel()

	go func() {
		select {
		case <-debugCtx.Done():
		case <-time.After(time.Minute * 1):
			fmt.Println("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! failed to listen")
			os.Exit(1)
		}
	}()

	ln, err := net.Listen("tcp", server.config.Addr())
	if err != nil {
		return errors.WithStackTrace(err)
	}
	debugCancel()
	server.Addr = ln.Addr().String()
	server.listener = ln

	log.Infof("Private Cache started, listening on %s", server.Addr)
	return nil
}

// Run starts the webserver and workers.
func (server *Server) Run(ctx context.Context) error {
	log.Infof("Start Terragrunt Provider Cache")

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return server.Provider.RunCacheWorker(ctx)
	})
	errGroup.Go(func() error {
		<-ctx.Done()
		log.Infof("Shutting down Terragrunt Provider Cache")

		ctx, cancel := context.WithTimeout(ctx, server.config.shutdownTimeout)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			return errors.WithStackTrace(err)
		}
		return nil
	})

	if err := server.Serve(server.listener); err != nil && err != http.ErrServerClosed {
		return errors.Errorf("error starting Terrafrom Cache server: %w", err)
	}
	defer log.Infof("Terragrunt Provider Cache stopped")

	return errGroup.Wait()
}
