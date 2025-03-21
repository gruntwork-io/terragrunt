package cache

import (
	"net"
	"strconv"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/tf/cache/handlers"
	"github.com/gruntwork-io/terragrunt/tf/cache/services"
)

const (
	defaultHostname        = "localhost"
	defaultShutdownTimeout = time.Second * 30
)

type Option func(Config) Config

func WithHostname(hostname string) Option {
	return func(cfg Config) Config {
		if hostname != "" {
			cfg.hostname = hostname
		}

		return cfg
	}
}

func WithPort(port int) Option {
	return func(cfg Config) Config {
		if port != 0 {
			cfg.port = port
		}

		return cfg
	}
}

func WithToken(token string) Option {
	return func(cfg Config) Config {
		cfg.token = token
		return cfg
	}
}

func WithProviderService(service *services.ProviderService) Option {
	return func(cfg Config) Config {
		cfg.providerService = service
		return cfg
	}
}

func WithProviderHandlers(handlers ...handlers.ProviderHandler) Option {
	return func(cfg Config) Config {
		cfg.providerHandlers = handlers
		return cfg
	}
}

func WithProxyProviderHandler(handler *handlers.ProxyProviderHandler) Option {
	return func(cfg Config) Config {
		cfg.proxyProviderHandler = handler
		return cfg
	}
}

func WithCacheProviderHTTPStatusCode(statusCode int) Option {
	return func(cfg Config) Config {
		cfg.cacheProviderHTTPStatusCode = statusCode
		return cfg
	}
}

func WithLogger(logger log.Logger) Option {
	return func(cfg Config) Config {
		cfg.logger = logger
		return cfg
	}
}

type Config struct {
	logger                      log.Logger
	providerService             *services.ProviderService
	proxyProviderHandler        *handlers.ProxyProviderHandler
	hostname                    string
	token                       string
	providerHandlers            handlers.ProviderHandlers
	port                        int
	shutdownTimeout             time.Duration
	cacheProviderHTTPStatusCode int
}

func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		hostname:        defaultHostname,
		shutdownTimeout: defaultShutdownTimeout,
		logger:          log.Default(),
	}

	return cfg.WithOptions(opts...)
}

func (cfg *Config) WithOptions(opts ...Option) *Config {
	for _, opt := range opts {
		*cfg = opt(*cfg)
	}

	return cfg
}

func (cfg *Config) Addr() string {
	return net.JoinHostPort(cfg.hostname, strconv.Itoa(cfg.port))
}
