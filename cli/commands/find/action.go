package find

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/mgutz/ansi"
)

func Run(ctx context.Context, opts *Options) error {
	configs, err := discovery.DiscoverConfigs(opts.TerragruntOptions)
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

type JSONOutput struct {
	Units  []string `json:"units,omitempty"`
	Stacks []string `json:"stacks,omitempty"`
}

func outputJSON(configs discovery.DiscoveredConfigs) error {
	output := JSONOutput{
		Units:  configs.Filter(discovery.ConfigTypeUnit).Paths(),
		Stacks: configs.Filter(discovery.ConfigTypeStack).Paths(),
	}

	jsonBytes, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(jsonBytes))
	return nil
}

type Colorizer struct {
	unitColorizer  func(string) string
	stackColorizer func(string) string
}

func NewColorizer() *Colorizer {
	return &Colorizer{
		unitColorizer:  ansi.ColorFunc("blue+bh"),
		stackColorizer: ansi.ColorFunc("green+bh"),
	}
}

func (c *Colorizer) colorize(config *discovery.DiscoveredConfig) string {
	switch config.ConfigType() {
	case discovery.ConfigTypeUnit:
		return c.unitColorizer(config.Path())
	case discovery.ConfigTypeStack:
		return c.stackColorizer(config.Path())
	default:
		return config.Path()
	}
}

func outputText(opts *Options, configs discovery.DiscoveredConfigs) error {
	if opts.TerragruntOptions.Logger.Formatter().DisabledColors() {
		for _, config := range configs {
			fmt.Println(config.Path())
		}

		return nil
	}

	colorizer := NewColorizer()

	for _, config := range configs {
		fmt.Println(colorizer.colorize(config))
	}

	return nil
}
