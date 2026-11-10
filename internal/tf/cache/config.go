package cache

import (
	"net"
	"strconv"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/internal/tf/cache/services"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	defaultHostname        = "localhost"
	defaultShutdownTimeout = time.Second * 30
)

// Option sets one setting of a [Config].
type Option func(Config) Config

// WithHostname sets the host the server listens on. An empty hostname keeps `localhost`.
func WithHostname(hostname string) Option {
	return func(cfg Config) Config {
		if hostname != "" {
			cfg.hostname = hostname
		}

		return cfg
	}
}

// WithPort sets the port the server listens on. Zero keeps the current port. With no port set,
// the OS picks a free one.
func WithPort(port int) Option {
	return func(cfg Config) Config {
		if port != 0 {
			cfg.port = port
		}

		return cfg
	}
}

// WithToken sets the bearer token every request to the provider and module endpoints must carry.
func WithToken(token string) Option {
	return func(cfg Config) Config {
		cfg.token = token
		return cfg
	}
}

// WithProviderService sets the service that downloads and caches providers.
func WithProviderService(service *services.ProviderService) Option {
	return func(cfg Config) Config {
		cfg.providerService = service
		return cfg
	}
}

// WithProviderHandlers sets the handlers that list versions and fetch platforms for cached provider requests.
func WithProviderHandlers(handlers ...handlers.ProviderHandler) Option {
	return func(cfg Config) Config {
		cfg.providerHandlers = handlers
		return cfg
	}
}

// WithProxyProviderHandler sets the handler that answers provider requests the server proxies without caching.
func WithProxyProviderHandler(handler *handlers.ProxyProviderHandler) Option {
	return func(cfg Config) Config {
		cfg.proxyProviderHandler = handler
		return cfg
	}
}

// WithProxyModuleHandler sets the handler for the module registry endpoints. A nil handler leaves
// those endpoints unregistered.
func WithProxyModuleHandler(handler *handlers.ProxyModuleHandler) Option {
	return func(cfg Config) Config {
		cfg.proxyModuleHandler = handler
		return cfg
	}
}

// WithCacheProviderHTTPStatusCode sets the status the server answers a platform request with
// after it starts caching that provider.
func WithCacheProviderHTTPStatusCode(statusCode int) Option {
	return func(cfg Config) Config {
		cfg.cacheProviderHTTPStatusCode = statusCode
		return cfg
	}
}

// WithLogger sets the logger for requests and server events.
func WithLogger(logger log.Logger) Option {
	return func(cfg Config) Config {
		cfg.logger = logger
		return cfg
	}
}

// Config is the configuration of the provider cache [Server], built from [Option] values.
type Config struct {
	logger                      log.Logger
	providerService             *services.ProviderService
	proxyProviderHandler        *handlers.ProxyProviderHandler
	proxyModuleHandler          *handlers.ProxyModuleHandler
	hostname                    string
	token                       string
	providerHandlers            handlers.ProviderHandlers
	port                        int
	shutdownTimeout             time.Duration
	cacheProviderHTTPStatusCode int
}

// NewConfig returns a Config with opts applied over the default `localhost` hostname.
func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		hostname:        defaultHostname,
		shutdownTimeout: defaultShutdownTimeout,
		logger:          log.Default(),
	}

	return cfg.WithOptions(opts...)
}

// WithOptions applies opts to cfg in order and returns cfg.
func (cfg *Config) WithOptions(opts ...Option) *Config {
	for _, opt := range opts {
		*cfg = opt(*cfg)
	}

	return cfg
}

// Addr returns the `host:port` address the server listens on.
func (cfg *Config) Addr() string {
	return net.JoinHostPort(cfg.hostname, strconv.Itoa(cfg.port))
}
