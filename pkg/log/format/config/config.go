package config

type Configs []*Config

func (cfg Configs) Find(name string) *Config {
	for _, cfg := range cfg {
		if cfg.name == name {
			return cfg
		}
	}

	return nil
}

func (cfg Configs) Names() []string {
	var names []string

	for _, cfg := range cfg {
		if cfg.name != "" {
			names = append(names, cfg.name)
		}
	}

	return names
}

type Config struct {
	name string
	opts Options
}

func (cfg *Config) Options() Options {
	return cfg.opts
}

func (cfg *Config) Name() string {
	return cfg.name
}

func New(name string, opts ...*Option) *Config {
	return &Config{
		name: name,
		opts: opts,
	}
}
