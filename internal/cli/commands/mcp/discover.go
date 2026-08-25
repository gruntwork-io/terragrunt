package mcp

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/discovery"
	"github.com/gruntwork-io/terragrunt/internal/tf"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type discoverInput struct {
	WorkingDir   string   `json:"working_dir,omitempty"  jsonschema:"Directory to discover under, relative to the server root. Defaults to the server root."`
	As           string   `json:"as,omitempty"           jsonschema:"The command exclude blocks are evaluated against, such as plan (the default), apply, or destroy."`
	Filter       []string `json:"filter,omitempty"       jsonschema:"Terragrunt filter queries (e.g. './stage/**', 'name=vpc') to narrow discovery."`
	Dependencies bool     `json:"dependencies,omitempty" jsonschema:"Parse configurations and include dependency edges and exclude-block results (slower)."`
	Hidden       bool     `json:"hidden,omitempty"       jsonschema:"Include units in hidden directories."`
}

// discoverParseAs maps the tool-call as argument onto the command exclude
// blocks are evaluated against, defaulting to a plan. An exclude block lists
// bare action words, so a value carrying arguments could never match one and
// is refused rather than quietly excluding nothing.
func discoverParseAs(name string) (string, error) {
	if name == "" {
		return tf.CommandNamePlan, nil
	}

	if strings.ContainsFunc(name, unicode.IsSpace) {
		return "", fmt.Errorf(
			"unsupported command %q: expected a single command such as %q or %q, without arguments",
			name, tf.CommandNamePlan, tf.CommandNameApply,
		)
	}

	return name, nil
}

// discoverEdges selects whether a discovered component comes back with the
// components it depends on.
type discoverEdges int

const (
	discoverOmitEdges discoverEdges = iota
	discoverIncludeEdges
)

// discoverEdgesFor maps the tool-call argument onto whether dependency edges
// come back.
func discoverEdgesFor(dependencies bool) discoverEdges {
	if dependencies {
		return discoverIncludeEdges
	}

	return discoverOmitEdges
}

type discoveredComponent struct {
	Path         string   `json:"path"`
	Dependencies []string `json:"dependencies,omitempty"`
	Excluded     bool     `json:"excluded,omitempty"`
}

type discoverOutput struct {
	WorkingDir string                `json:"working_dir"`
	Units      []discoveredComponent `json:"units"`
	Stacks     []discoveredComponent `json:"stacks,omitempty"`
	Degraded   []string              `json:"degraded,omitempty"`
	UnitCount  int                   `json:"unit_count"`
}

func registerDiscover(srv *mcp.Server, l log.Logger, d *serverDeps, rootVenv *venv.Venv) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "discover",
		Description: "List every Terragrunt unit and stack under a directory. Use this first to map an " +
			"unfamiliar Terragrunt tree, or to resolve a filter expression into concrete unit paths. Set " +
			"dependencies=true only when you need the dependency graph or exclude-block results; it parses " +
			"every configuration and is slower. 'excluded' reflects the exclude blocks for the command named " +
			"by 'as', a plan by default, and is only populated when dependencies=true. Results may carry a " +
			"'degraded' list when the server is running without --allow=exec.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:   true,
			IdempotentHint: true,
			OpenWorldHint:  new(d.reachesNetwork()),
		},
	}, parseToolHandler(l, d, "discover", rootVenv, runDiscover))
}

func runDiscover(
	ctx context.Context,
	l log.Logger,
	d *serverDeps,
	rootVenv *venv.Venv,
	input discoverInput,
) (discoverOutput, error) {
	as, err := discoverParseAs(input.As)
	if err != nil {
		return discoverOutput{}, err
	}

	dir, err := resolveWorkingDir(rootVenv.FS, d.launchDir, input.WorkingDir)
	if err != nil {
		return discoverOutput{}, err
	}

	opts, env, err := buildDirOptions(l, d, rootVenv, dir, input.Filter)
	if err != nil {
		return discoverOutput{}, err
	}

	// Exclude-block evaluation matches actions against TerraformCommand, where
	// an empty command never matches.
	opts.TerraformCommand = as

	cv := d.callVenv(rootVenv, env, io.Discard)

	ctx = freshCallContext(ctx)

	disc, err := discovery.NewForDiscoveryCommand(l, cv.FS, &discovery.DiscoveryCommandOptions{
		WorkingDir:        dir,
		DiscoveryBoundary: opts.DiscoveryBoundary,
		Filters:           opts.Filters,
		NoHidden:          !input.Hidden,
		WithRequiresParse: input.Dependencies,
		WithRelationships: input.Dependencies,
	})
	if err != nil {
		return discoverOutput{}, err
	}

	w, cleanup, err := setupGitFilterWorktrees(ctx, l, d, cv, opts)
	if err != nil {
		return discoverOutput{}, err
	}

	if w != nil {
		defer cleanup()

		disc = disc.WithWorktrees(w)
	}

	components, degraded := discoverDegraded(ctx, l, cv, opts, disc, "discover")

	units, stacks := discoverComponents(l, dir, components, discoverEdgesFor(input.Dependencies))

	return discoverOutput{
		WorkingDir: dir,
		Units:      units,
		Stacks:     stacks,
		Degraded:   append(degraded, d.rec.notes()...),
		UnitCount:  len(units),
	}, nil
}

// discoverComponents maps the discovered components onto their output form,
// splitting the units from the stacks. Paths come back relative to workingDir,
// which is the directory the call was bounded to rather than the server root.
func discoverComponents(
	l log.Logger,
	workingDir string,
	components component.Components,
	edges discoverEdges,
) (units, stacks []discoveredComponent) {
	units = make([]discoveredComponent, 0, len(components))

	for _, c := range components {
		dc := discoveredComponent{
			Path: discovery.RelPathForComponent(l, c, workingDir, c.Path(), "component"),
		}

		if edges == discoverIncludeEdges {
			for _, dep := range c.Dependencies() {
				dc.Dependencies = append(
					dc.Dependencies,
					discovery.RelPathForComponent(l, dep, workingDir, dep.Path(), "dependency"),
				)
			}
		}

		switch c := c.(type) {
		case *component.Unit:
			dc.Excluded = c.Excluded()

			units = append(units, dc)
		case *component.Stack:
			stacks = append(stacks, dc)
		}
	}

	return units, stacks
}
