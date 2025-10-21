package find

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/mgutz/ansi"
)

// Run runs the find command.
func Run(ctx context.Context, l log.Logger, opts *Options) error {
	d, err := discovery.NewForCommand(discovery.DiscoveryCommandOptions{
		WorkingDir:       opts.WorkingDir,
		QueueConstructAs: opts.QueueConstructAs,
		Hidden:           opts.Hidden,
		Dependencies:     opts.Dependencies || opts.External || opts.Mode == ModeDAG,
		External:         opts.External,
		Exclude:          opts.Exclude,
		Include:          opts.Include,
		Reading:          opts.Reading,
		FilterQueries:    opts.FilterQueries,
		Experiments:      opts.Experiments,
	})
	if err != nil {
		return errors.New(err)
	}

	var (
		components  component.Components
		discoverErr error
	)

	telemetryErr := telemetry.TelemeterFromContext(ctx).Collect(ctx, "find_discover", map[string]any{
		"working_dir":  opts.WorkingDir,
		"hidden":       opts.Hidden,
		"dependencies": opts.Dependencies,
		"external":     opts.External,
		"mode":         opts.Mode,
		"exclude":      opts.Exclude,
	}, func(ctx context.Context) error {
		components, discoverErr = d.Discover(ctx, l, opts.TerragruntOptions)
		return discoverErr
	})
	if telemetryErr != nil {
		l.Debugf("Errors encountered while discovering components:\n%s", telemetryErr)
	}

	switch opts.Mode {
	case ModeNormal:
		components = components.Sort()
	case ModeDAG:
		err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "find_mode_dag", map[string]any{
			"working_dir":  opts.WorkingDir,
			"config_count": len(components),
		}, func(ctx context.Context) error {
			q, queueErr := queue.NewQueue(components)
			if queueErr != nil {
				return queueErr
			}

			components = q.Components()

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

	var foundComponents FoundComponents

	err = telemetry.TelemeterFromContext(ctx).Collect(ctx, "find_discovered_to_found", map[string]any{
		"working_dir":  opts.WorkingDir,
		"config_count": len(components),
	}, func(ctx context.Context) error {
		var convErr error

		foundComponents, convErr = discoveredToFound(components, opts)

		return convErr
	})
	if err != nil {
		return errors.New(err)
	}

	switch opts.Format {
	case FormatText:
		return outputText(l, opts, foundComponents)
	case FormatJSON:
		return outputJSON(opts, foundComponents)
	default:
		// This should never happen, because of validation in the command.
		// If it happens, we want to throw so we can fix the validation.
		return errors.New("invalid format: " + opts.Format)
	}
}

type FoundComponents []*FoundComponent

type FoundComponent struct {
	Type component.Kind `json:"type"`
	Path string         `json:"path"`

	Exclude *config.ExcludeConfig `json:"exclude,omitempty"`
	Include map[string]string     `json:"include,omitempty"`

	Dependencies []string `json:"dependencies,omitempty"`
	Reading      []string `json:"reading,omitempty"`
}

func discoveredToFound(components component.Components, opts *Options) (FoundComponents, error) {
	foundComponents := make(FoundComponents, 0, len(components))
	errs := []error{}

	for _, c := range components {
		if c.External && !opts.External {
			continue
		}

		if opts.QueueConstructAs != "" {
			if c.Parsed != nil && c.Parsed.Exclude != nil {
				if c.Parsed.Exclude.IsActionListed(opts.QueueConstructAs) {
					continue
				}
			}
		}

		relPath, err := filepath.Rel(opts.WorkingDir, c.Path)
		if err != nil {
			errs = append(errs, errors.New(err))

			continue
		}

		foundComponent := &FoundComponent{
			Type: c.Kind,
			Path: relPath,
		}

		if opts.Exclude && c.Parsed != nil && c.Parsed.Exclude != nil {
			foundComponent.Exclude = c.Parsed.Exclude.Clone()
		}

		if opts.Include && c.Parsed != nil && c.Parsed.ProcessedIncludes != nil {
			foundComponent.Include = make(map[string]string, len(c.Parsed.ProcessedIncludes))
			for _, v := range c.Parsed.ProcessedIncludes {
				foundComponent.Include[v.Name], err = util.GetPathRelativeTo(v.Path, opts.RootWorkingDir)
				if err != nil {
					errs = append(errs, errors.New(err))
				}
			}
		}

		if opts.Reading && len(c.Reading) > 0 {
			foundComponent.Reading = make([]string, len(c.Reading))

			for i, reading := range c.Reading {
				relReadingPath, err := filepath.Rel(opts.WorkingDir, reading)
				if err != nil {
					errs = append(errs, errors.New(err))
				}

				foundComponent.Reading[i] = relReadingPath
			}
		}

		if opts.Dependencies && len(c.Dependencies()) > 0 {
			foundComponent.Dependencies = make([]string, len(c.Dependencies()))

			for i, dep := range c.Dependencies() {
				relDepPath, err := filepath.Rel(opts.WorkingDir, dep.Path)
				if err != nil {
					errs = append(errs, errors.New(err))

					continue
				}

				foundComponent.Dependencies[i] = relDepPath
			}
		}

		if opts.Reading && len(c.Reading) > 0 {
			foundComponent.Reading = make([]string, len(c.Reading))

			for i, reading := range c.Reading {
				relReadingPath, err := filepath.Rel(opts.WorkingDir, reading)
				if err != nil {
					errs = append(errs, errors.New(err))
				}

				foundComponent.Reading[i] = relReadingPath
			}
		}

		foundComponents = append(foundComponents, foundComponent)
	}

	return foundComponents, errors.Join(errs...)
}

// outputJSON outputs the discovered components in JSON format.
func outputJSON(opts *Options, components FoundComponents) error {
	jsonBytes, err := json.MarshalIndent(components, "", "  ")
	if err != nil {
		return errors.New(err)
	}

	_, err = opts.Writer.Write(append(jsonBytes, []byte("\n")...))
	if err != nil {
		return errors.New(err)
	}

	return nil
}

// Colorizer is a colorizer for the discovered components.
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

func (c *Colorizer) Colorize(foundComponent *FoundComponent) string {
	path := foundComponent.Path

	// Get the directory and base name using filepath
	dir, base := filepath.Split(path)

	if dir == "" {
		// No directory part, color the whole path
		switch foundComponent.Type {
		case component.Unit:
			return c.unitColorizer(path)
		case component.Stack:
			return c.stackColorizer(path)
		default:
			return path
		}
	}

	// Color the components differently
	coloredPath := c.pathColorizer(dir)

	switch foundComponent.Type {
	case component.Unit:
		return coloredPath + c.unitColorizer(base)
	case component.Stack:
		return coloredPath + c.stackColorizer(base)
	default:
		return path
	}
}

// outputText outputs the discovered components in text format.
func outputText(l log.Logger, opts *Options, components FoundComponents) error {
	colorizer := NewColorizer(shouldColor(l))

	for _, c := range components {
		_, err := opts.Writer.Write([]byte(colorizer.Colorize(c) + "\n"))
		if err != nil {
			return errors.New(err)
		}
	}

	return nil
}

// shouldColor returns true if the output should be colored.
func shouldColor(l log.Logger) bool {
	return !l.Formatter().DisabledColors() && !stdout.IsRedirected()
}
