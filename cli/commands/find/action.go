package find

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/mgutz/ansi"
)

func Run(ctx context.Context, opts *Options) error {
	d := discovery.NewDiscovery(opts.WorkingDir)

	configs, err := d.Discover()
	if err != nil {
		return err
	}

	switch opts.Format {
	case "text":
		return outputText(opts, configs)
	case "json":
		return outputJSON(configs)
	default:
		return errors.New("invalid format: " + opts.Format)
	}
}

func outputJSON(configs discovery.DiscoveredConfigs) error {
	jsonBytes, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(jsonBytes))

	return nil
}

type Colorizer struct {
	unitColorizer  func(string) string
	stackColorizer func(string) string
	pathColorizer  func(string) string
}

func NewColorizer() *Colorizer {
	return &Colorizer{
		unitColorizer:  ansi.ColorFunc("blue+bh"),
		stackColorizer: ansi.ColorFunc("green+bh"),
		pathColorizer:  ansi.ColorFunc("white+d"),
	}
}

func (c *Colorizer) colorize(config *discovery.DiscoveredConfig) string {
	path := config.Path

	// Get the directory and base name using filepath
	dir, base := filepath.Split(path)

	if dir == "" {
		// No directory part, color the whole path
		switch config.ConfigType() {
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

	switch config.ConfigType() {
	case discovery.ConfigTypeUnit:
		return coloredPath + c.unitColorizer(base)
	case discovery.ConfigTypeStack:
		return coloredPath + c.stackColorizer(base)
	default:
		return path
	}
}

func outputText(opts *Options, configs discovery.DiscoveredConfigs) error {
	if opts.TerragruntOptions.Logger.Formatter().DisabledColors() {
		for _, config := range configs {
			fmt.Println(config.Path)
		}

		return nil
	}

	colorizer := NewColorizer()

	for _, config := range configs {
		fmt.Println(colorizer.colorize(config))
	}

	return nil
}
