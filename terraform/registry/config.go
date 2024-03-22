package registry

import (
	"net"
	"strconv"
	"time"
)

const (
	defaultHostname        = "localhost"
	defaultPort            = 5758
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
		cfg.port = port
		return cfg
	}
}

func WithToken(token string) Option {
	return func(cfg Config) Config {
		cfg.token = token
		return cfg
	}
}

type Config struct {
	hostname        string
	port            int
	token           string
	shutdownTimeout time.Duration
}

func NewConfig(opts ...Option) *Config {
	cfg := &Config{
		hostname:        defaultHostname,
		port:            defaultPort,
		shutdownTimeout: defaultShutdownTimeout,
	}

	return cfg.WithOptions(opts...)
}

func (config *Config) WithOptions(opts ...Option) *Config {
	for _, opt := range opts {
		*config = opt(*config)
	}

	return config
}

func (cfg *Config) Addr() string {
	return net.JoinHostPort(cfg.hostname, strconv.Itoa(cfg.port))
}
