package find

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/mgutz/ansi"
)

// Run runs the find command.
func Run(ctx context.Context, l log.Logger, opts *Options) error {
	d := discovery.
		NewDiscovery(opts.WorkingDir).
		WithSuppressParseErrors()

	if opts.Hidden {
		d = d.WithHidden()
	}

	if opts.Dependencies || opts.External || opts.Mode == ModeDAG {
		d = d.WithDiscoverDependencies()
	}

	if opts.External {
		d = d.WithDiscoverExternalDependencies()
	}

	if opts.Exclude {
		d = d.WithParseExclude()
	}

	if opts.Include {
		d = d.WithParseInclude()
	}

	if opts.QueueConstructAs != "" {
		d = d.WithParseExclude()
		d = d.WithDiscoveryContext(&discovery.DiscoveryContext{
			Cmd: opts.QueueConstructAs,
		})
	}

	var cfgs discovery.DiscoveredConfigs

	var discoverErr error

	err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "find_discover", map[string]any{
		"working_dir":  opts.WorkingDir,
		"hidden":       opts.Hidden,
		"dependencies": opts.Dependencies,
		"external":     opts.External,
		"mode":         opts.Mode,
		"exclude":      opts.Exclude,
	}, func(ctx context.Context) error {
		cfgs, discoverErr = d.Discover(ctx, l, opts.TerragruntOptions)
		return discoverErr
	})

	if err != nil {
		l.Debugf("Errors encountered while discovering configurations:\n%s", err)
	}

	switch opts.Mode {
	case ModeNormal:
		cfgs = cfgs.Sort()
	case ModeDAG:
		err := telemetry.TelemeterFromContext(ctx).Collect(ctx, "find_mode_dag", map[string]any{
			"working_dir":  opts.WorkingDir,
			"config_count": len(cfgs),
		}, func(ctx context.Context) error {
			q, queueErr := queue.NewQueue(cfgs)
			if queueErr != nil {
				return queueErr
			}

			cfgs = q.Configs()

			return nil
		})
		if err != nil {
			return errors.New(err)
		}
	default:
		// This should never happen, because of validation in the command.
		// If it happens, we want to throw so we can fix the validation.
		return errors.New("invalid mode: " + opts.Mode)
	}

	var foundCfgs FoundConfigs

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "find_discovered_to_found", map[string]any{
		"working_dir":  opts.WorkingDir,
		"config_count": len(cfgs),
	}, func(ctx context.Context) error {
		var convErr error
		foundCfgs, convErr = discoveredToFound(cfgs, opts)

		return convErr
	})
	if err != nil {
		return errors.New(err)
	}

	switch opts.Format {
	case FormatText:
		return outputText(l, opts, foundCfgs)
	case FormatJSON:
		return outputJSON(opts, foundCfgs)
	default:
		// This should never happen, because of validation in the command.
		// If it happens, we want to throw so we can fix the validation.
		return errors.New("invalid format: " + opts.Format)
	}
}

type FoundConfigs []*FoundConfig

type FoundConfig struct {
	Type discovery.ConfigType `json:"type"`
	Path string               `json:"path"`

	Exclude *config.ExcludeConfig `json:"exclude,omitempty"`
	Include map[string]string     `json:"include,omitempty"`

	Dependencies []string `json:"dependencies,omitempty"`
}

func discoveredToFound(configs discovery.DiscoveredConfigs, opts *Options) (FoundConfigs, error) {
	foundCfgs := make(FoundConfigs, 0, len(configs))
	errs := []error{}

	for _, config := range configs {
		if config.External && !opts.External {
			continue
		}

		if opts.QueueConstructAs != "" {
			if config.Parsed != nil && config.Parsed.Exclude != nil {
				if config.Parsed.Exclude.IsActionListed(opts.QueueConstructAs) {
					continue
				}
			}
		}

		relPath, err := filepath.Rel(opts.WorkingDir, config.Path)
		if err != nil {
			errs = append(errs, errors.New(err))

			continue
		}

		foundCfg := &FoundConfig{
			Type: config.Type,
			Path: relPath,
		}

		if opts.Exclude && config.Parsed != nil && config.Parsed.Exclude != nil {
			foundCfg.Exclude = config.Parsed.Exclude.Clone()
		}

		if opts.Include && config.Parsed != nil && config.Parsed.ProcessedIncludes != nil {
			foundCfg.Include = make(map[string]string, len(config.Parsed.ProcessedIncludes))
			for _, v := range config.Parsed.ProcessedIncludes {
				foundCfg.Include[v.Name], err = util.GetPathRelativeTo(v.Path, opts.RootWorkingDir)
				if err != nil {
					errs = append(errs, errors.New(err))
				}
			}
		}

		if !opts.Dependencies || len(config.Dependencies) == 0 {
			foundCfgs = append(foundCfgs, foundCfg)

			continue
		}

		foundCfg.Dependencies = make([]string, len(config.Dependencies))

		for i, dep := range config.Dependencies {
			relDepPath, err := filepath.Rel(opts.WorkingDir, dep.Path)
			if err != nil {
				errs = append(errs, errors.New(err))

				continue
			}

			foundCfg.Dependencies[i] = relDepPath
		}

		foundCfgs = append(foundCfgs, foundCfg)
	}

	return foundCfgs, errors.Join(errs...)
}

// outputJSON outputs the discovered configurations in JSON format.
func outputJSON(opts *Options, configs FoundConfigs) error {
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
func NewColorizer(shouldColor bool) *Colorizer {
	if !shouldColor {
		return &Colorizer{
			unitColorizer:  func(s string) string { return s },
			stackColorizer: func(s string) string { return s },
			pathColorizer:  func(s string) string { return s },
		}
	}

	return &Colorizer{
		unitColorizer:  ansi.ColorFunc("blue+bh"),
		stackColorizer: ansi.ColorFunc("green+bh"),
		pathColorizer:  ansi.ColorFunc("white+d"),
	}
}

func (c *Colorizer) Colorize(config *FoundConfig) string {
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
func outputText(l log.Logger, opts *Options, configs FoundConfigs) error {
	colorizer := NewColorizer(shouldColor(l))

	for _, config := range configs {
		_, err := opts.Writer.Write([]byte(colorizer.Colorize(config) + "\n"))
		if err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// shouldColor returns true if the output should be colored.
func shouldColor(l log.Logger) bool {
	return !l.Formatter().DisabledColors() && !isStdoutRedirected()
}

// isStdoutRedirected returns true if the stdout is redirected.
func isStdoutRedirected() bool {
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return (stat.Mode() & os.ModeCharDevice) == 0
}
