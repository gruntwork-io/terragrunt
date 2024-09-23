package cache

import (
	"net"
	"strconv"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/terraform/cache/handlers"
	"github.com/gruntwork-io/terragrunt/terraform/cache/services"
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

func WithServices(services ...services.Service) Option {
	return func(cfg Config) Config {
		cfg.services = services
		return cfg
	}
}

func WithProviderHandlers(handlers ...handlers.ProviderHandler) Option {
	return func(cfg Config) Config {
		cfg.providerHandlers = handlers
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
	hostname        string
	port            int
	token           string
	shutdownTimeout time.Duration

	services         []services.Service
	providerHandlers handlers.ProviderHandlers

	logger log.Logger
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
