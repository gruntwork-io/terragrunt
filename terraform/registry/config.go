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

type Config struct {
	Hostname        string
	Port            int
	Token           string
	ShutdownTimeout time.Duration
}

func NewConfig() *Config {
	return &Config{
		Hostname:        defaultHostname,
		Port:            defaultPort,
		ShutdownTimeout: defaultShutdownTimeout,
	}
}

func (cfg *Config) Addr() string {
	return net.JoinHostPort(cfg.Hostname, strconv.Itoa(cfg.Port))
}
