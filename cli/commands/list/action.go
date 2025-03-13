package list

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/mgutz/ansi"
)

// Run runs the list command.
func Run(ctx context.Context, opts *Options) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	if opts.Hidden {
		d = d.WithHidden()
	}

	if opts.Dependencies || opts.External || opts.Sort == SortDAG {
		d = d.WithDiscoverDependencies()
	}

	if opts.External {
		d = d.WithDiscoverExternalDependencies()
	}

	cfgs, err := d.Discover(ctx, opts.TerragruntOptions)
	if err != nil {
		return errors.New(err)
	}

	switch opts.Sort {
	case SortAlpha:
		cfgs = cfgs.Sort()
	case SortDAG:
		q, err := queue.NewQueue(cfgs)
		if err != nil {
			return errors.New(err)
		}

		cfgs = q.Entries()
	}

	listedCfgs, err := discoveredToListed(cfgs, opts)
	if err != nil {
		return errors.New(err)
	}

	switch opts.Format {
	case FormatText:
		return outputText(opts, listedCfgs)
	case FormatJSON:
		return outputJSON(opts, listedCfgs)
	default:
		// This should never happen, because of validation in the command.
		// If it happens, we want to throw so we can fix the validation.
		return errors.New("invalid format: " + opts.Format)
	}
}

type ListedConfigs []*ListedConfig

type ListedConfig struct {
	Type discovery.ConfigType `json:"type"`
	Path string               `json:"path"`

	Dependencies []string `json:"dependencies,omitempty"`
}

func discoveredToListed(configs discovery.DiscoveredConfigs, opts *Options) (ListedConfigs, error) {
	listedCfgs := make(ListedConfigs, 0, len(configs))
	errs := []error{}

	for _, config := range configs {
		if config.External && !opts.External {
			continue
		}

		relPath, err := filepath.Rel(opts.WorkingDir, config.Path)
		if err != nil {
			errs = append(errs, errors.New(err))

			continue
		}

		listedCfg := &ListedConfig{
			Type: config.Type,
			Path: relPath,
		}

		if !opts.Dependencies || len(config.Dependencies) == 0 {
			listedCfgs = append(listedCfgs, listedCfg)

			continue
		}

		listedCfg.Dependencies = make([]string, len(config.Dependencies))

		for i, dep := range config.Dependencies {
			relDepPath, err := filepath.Rel(opts.WorkingDir, dep.Path)
			if err != nil {
				errs = append(errs, errors.New(err))

				continue
			}

			listedCfg.Dependencies[i] = relDepPath
		}

		listedCfgs = append(listedCfgs, listedCfg)
	}

	return listedCfgs, errors.Join(errs...)
}

// outputJSON outputs the discovered configurations in JSON format.
func outputJSON(opts *Options, configs ListedConfigs) error {
	jsonBytes, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return errors.New(err)
	}

	_, err = opts.Writer.Write(append(jsonBytes, []byte("\n")...))
	if err != nil {
		return errors.New(err)
	}

	return nil
}

// Colorizer is a colorizer for the discovered configurations.
type Colorizer struct {
	unitColorizer  func(string) string
	stackColorizer func(string) string
	pathColorizer  func(string) string
}

// NewColorizer creates a new Colorizer.
func NewColorizer() *Colorizer {
	return &Colorizer{
		unitColorizer:  ansi.ColorFunc("blue+bh"),
		stackColorizer: ansi.ColorFunc("green+bh"),
		pathColorizer:  ansi.ColorFunc("white+d"),
	}
}

func (c *Colorizer) Colorize(config *ListedConfig) string {
	path := config.Path

	// Get the directory and base name using filepath
	dir, base := filepath.Split(path)

	if dir == "" {
		// No directory part, color the whole path
		switch config.Type {
		case discovery.ConfigTypeUnit:
			return c.unitColorizer(path)
		case discovery.ConfigTypeStack:
			return c.stackColorizer(path)
		default:
			return path
		}
	}

	// Color the components differently
	coloredPath := c.pathColorizer(dir)

	switch config.Type {
	case discovery.ConfigTypeUnit:
		return coloredPath + c.unitColorizer(base)
	case discovery.ConfigTypeStack:
		return coloredPath + c.stackColorizer(base)
	default:
		return path
	}
}

// outputText outputs the discovered configurations in text format.
func outputText(opts *Options, configs ListedConfigs) error {
	if opts.TerragruntOptions.Logger.Formatter().DisabledColors() || isStdoutRedirected() {
		for _, config := range configs {
			_, err := opts.Writer.Write([]byte(config.Path + "\n"))
			if err != nil {
				return errors.New(err)
			}
		}

		return nil
	}

	colorizer := NewColorizer()

	for _, config := range configs {
		_, err := opts.Writer.Write([]byte(colorizer.Colorize(config) + "\n"))
		if err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// isStdoutRedirected returns true if the stdout is redirected.
func isStdoutRedirected() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) == 0
}
