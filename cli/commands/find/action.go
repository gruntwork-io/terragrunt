package find

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/mgutz/ansi"
)

// Run runs the find command.
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

// outputJSON outputs the discovered configurations in JSON format.
func outputJSON(configs discovery.DiscoveredConfigs) error {
	jsonBytes, err := json.MarshalIndent(configs, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(jsonBytes))

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

func (c *Colorizer) colorize(config *discovery.DiscoveredConfig) string {
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
func outputText(opts *Options, configs discovery.DiscoveredConfigs) error {
	if opts.TerragruntOptions.Logger.Formatter().DisabledColors() || isStdoutRedirected() {
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

// isStdoutRedirected returns true if the stdout is redirected.
func isStdoutRedirected() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) == 0
}
