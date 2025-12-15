package find

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/telemetry"
	"github.com/gruntwork-io/terragrunt/util"

	"github.com/gruntwork-io/terragrunt/config"
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/errors"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/os/stdout"
	"github.com/gruntwork-io/terragrunt/internal/queue"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/mgutz/ansi"
)

// Run runs the find command.
func Run(ctx context.Context, l log.Logger, opts *Options) error {
	d, err := discovery.NewForDiscoveryCommand(discovery.DiscoveryCommandOptions{
		WorkingDir:       opts.WorkingDir,
		QueueConstructAs: opts.QueueConstructAs,
		NoHidden:         !opts.Hidden,
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

	if opts.Experiments.Evaluate(experiment.FilterFlag) {
		// We do worktree generation here instead of in the discovery constructor
		// so that we can defer cleanup in the same context.
		filters, parseErr := filter.ParseFilterQueries(opts.FilterQueries)
		if parseErr != nil {
			return fmt.Errorf("failed to parse filters: %w", parseErr)
		}

		gitFilters := filters.UniqueGitFilters()

		worktrees, worktreeErr := worktrees.NewWorktrees(ctx, l, opts.WorkingDir, gitFilters)
		if worktreeErr != nil {
			return errors.Errorf("failed to create worktrees: %w", worktreeErr)
		}

		defer func() {
			cleanupErr := worktrees.Cleanup(ctx, l)
			if cleanupErr != nil {
				l.Errorf("failed to cleanup worktrees: %v", cleanupErr)
			}
		}()

		d = d.WithWorktrees(worktrees)
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
		if c.External() && !opts.External {
			continue
		}

		if opts.QueueConstructAs != "" {
			if unit, ok := c.(*component.Unit); ok {
				if cfg := unit.Config(); cfg != nil && cfg.Exclude != nil {
					if cfg.Exclude.IsActionListed(opts.QueueConstructAs) {
						continue
					}
				}
			}
		}

		var (
			relPath string
			err     error
		)

		if c.DiscoveryContext() != nil && c.DiscoveryContext().WorkingDir != "" {
			relPath, err = filepath.Rel(c.DiscoveryContext().WorkingDir, c.Path())
		} else {
			relPath, err = filepath.Rel(opts.WorkingDir, c.Path())
		}

		if err != nil {
			errs = append(errs, errors.New(err))

			continue
		}

		foundComponent := &FoundComponent{
			Type: c.Kind(),
			Path: relPath,
		}

		if opts.Exclude {
			if unit, ok := c.(*component.Unit); ok {
				if cfg := unit.Config(); cfg != nil && cfg.Exclude != nil {
					foundComponent.Exclude = cfg.Exclude.Clone()
				}
			}
		}

		if opts.Include {
			if unit, ok := c.(*component.Unit); ok {
				if cfg := unit.Config(); cfg != nil && cfg.ProcessedIncludes != nil {
					foundComponent.Include = make(map[string]string, len(cfg.ProcessedIncludes))
					for _, v := range cfg.ProcessedIncludes {
						foundComponent.Include[v.Name], err = util.GetPathRelativeTo(v.Path, opts.RootWorkingDir)
						if err != nil {
							errs = append(errs, errors.New(err))
						}
					}
				}
			}
		}

		if opts.Reading && len(c.Reading()) > 0 {
			foundComponent.Reading = make([]string, len(c.Reading()))

			for i, reading := range c.Reading() {
				var relReadingPath string

				if c.DiscoveryContext() != nil && c.DiscoveryContext().WorkingDir != "" {
					relReadingPath, err = filepath.Rel(c.DiscoveryContext().WorkingDir, reading)
				} else {
					relReadingPath, err = filepath.Rel(opts.WorkingDir, reading)
				}

				if err != nil {
					errs = append(errs, errors.New(err))

					continue
				}

				foundComponent.Reading[i] = relReadingPath
			}
		}

		if opts.Dependencies && len(c.Dependencies()) > 0 {
			foundComponent.Dependencies = make([]string, len(c.Dependencies()))

			for i, dep := range c.Dependencies() {
				var relDepPath string

				if dep.DiscoveryContext() != nil && dep.DiscoveryContext().WorkingDir != "" {
					relDepPath, err = filepath.Rel(dep.DiscoveryContext().WorkingDir, dep.Path())
				} else {
					relDepPath, err = filepath.Rel(opts.WorkingDir, dep.Path())
				}

				if err != nil {
					errs = append(errs, errors.New(err))

					continue
				}

				foundComponent.Dependencies[i] = relDepPath
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
		case component.UnitKind:
			return c.unitColorizer(path)
		case component.StackKind:
			return c.stackColorizer(path)
		default:
			return path
		}
	}

	// Color the components differently
	coloredPath := c.pathColorizer(dir)

	switch foundComponent.Type {
	case component.UnitKind:
		return coloredPath + c.unitColorizer(base)
	case component.StackKind:
		return coloredPath + c.stackColorizer(base)
	default:
		return path
	}
}

// outputText outputs the discovered components in text format.
func outputText(l log.Logger, opts *Options, components FoundComponents) error {
	var buf strings.Builder

	colorizer := NewColorizer(shouldColor(l))

	for _, c := range components {
		buf.WriteString(colorizer.Colorize(c) + "\n")
	}

	_, err := opts.Writer.Write([]byte(buf.String()))

	return errors.New(err)
}

// shouldColor returns true if the output should be colored.
func shouldColor(l log.Logger) bool {
	return !l.Formatter().DisabledColors() && !stdout.IsRedirected()
}
