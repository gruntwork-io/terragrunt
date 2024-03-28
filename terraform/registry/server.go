package registry

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/registry/controllers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/registry/router"
	"github.com/gruntwork-io/terragrunt/terraform/registry/services"
	"github.com/gruntwork-io/terragrunt/util"
	"golang.org/x/sync/errgroup"
)

const (
	filelockName                    = "terragrunt/server-port.lock"
	waitNextAttepmtToLockServerPort = time.Second
	maxAttemptsToLockServerPort     = 60 // equals 1 min
)

// Server is a private Terraform registry for provider caching.
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
	cacheDir, err := util.GetCacheDir()
	if err != nil {
		return err
	}

	filelockName := filepath.Join(cacheDir, filelockName)
	if err := os.MkdirAll(filepath.Dir(filelockName), os.ModePerm); err != nil {
		return errors.WithStackTrace(err)
	}

	filelock, err := util.AcquireFileLock(ctx, filelockName, maxAttemptsToLockServerPort, waitNextAttepmtToLockServerPort)
	if err != nil {
		return err
	}

	log.Trace("Server listen is locked")
	defer func() {
		_ = filelock.Unlock()
		log.Tracef("Server listen is released")
	}()

	// if the port is undefined, ask the kernel for a free open port that is ready to use
	addr, err := net.ResolveTCPAddr("tcp", server.config.Addr())
	if err != nil {
		return errors.WithStackTrace(err)
	}

	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return errors.WithStackTrace(err)
	}

	server.Addr = ln.Addr().String()
	server.listener = ln

	log.Infof("Private Registry started, listening on %s", server.Addr)
	return nil
}

// Run starts the webserver and workers.
func (server *Server) Run(ctx context.Context) error {
	log.Infof("Start Private Registry")

	errGroup, ctx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return server.Provider.RunCacheWorker(ctx)
	})
	errGroup.Go(func() error {
		<-ctx.Done()
		log.Infof("Shutting down Private Registry")

		ctx, cancel := context.WithTimeout(ctx, server.config.shutdownTimeout)
		defer cancel()

		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			return errors.WithStackTrace(err)
		}
		return nil
	})

	if err := server.Serve(server.listener); err != nil && err != http.ErrServerClosed {
		return errors.Errorf("error starting Terrafrom Registry server: %w", err)
	}
	defer log.Infof("Private Registry stopped")

	return errGroup.Wait()
}
