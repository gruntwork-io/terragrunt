package discovery

import (
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// WithDiscoveryContext sets the discovery context.
func (d *Discovery) WithDiscoveryContext(ctx *component.DiscoveryContext) *Discovery {
	d.discoveryContext = ctx
	return d
}

// WithWorktrees sets the worktrees for Git-based filters.
func (d *Discovery) WithWorktrees(w *worktrees.Worktrees) *Discovery {
	d.worktrees = w
	return d
}

// WithConfigFilenames sets the config filenames to discover.
func (d *Discovery) WithConfigFilenames(filenames []string) *Discovery {
	d.configFilenames = filenames
	return d
}

// WithParserOptions sets custom HCL parser options.
func (d *Discovery) WithParserOptions(opts []hclparse.Option) *Discovery {
	d.parserOptions = opts
	return d
}

// WithFilters sets filter queries for component selection.
func (d *Discovery) WithFilters(filters filter.Filters) *Discovery {
	d.filters = filters

	// If there are any positive filters, exclude by default
	if d.filters.HasPositiveFilter() {
		d.excludeByDefault = true
	}

	// Check if filters require parsing
	if _, ok := d.filters.RequiresParse(); ok {
		d.requiresParse = true
	}

	// Collect Git expressions
	d.gitExpressions = d.filters.UniqueGitFilters()

	return d
}

// WithMaxDependencyDepth sets the maximum dependency depth.
func (d *Discovery) WithMaxDependencyDepth(depth int) *Discovery {
	d.maxDependencyDepth = depth
	return d
}

// WithNumWorkers sets the number of concurrent workers.
func (d *Discovery) WithNumWorkers(numWorkers int) *Discovery {
	if numWorkers > 0 && numWorkers <= maxDiscoveryWorkers {
		d.numWorkers = numWorkers
	}

	return d
}

// WithNoHidden excludes hidden directories from discovery.
func (d *Discovery) WithNoHidden() *Discovery {
	d.noHidden = true
	return d
}

// WithRequiresParse enables parsing of Terragrunt configurations.
func (d *Discovery) WithRequiresParse() *Discovery {
	d.requiresParse = true
	return d
}

// WithParseExclude enables parsing of exclude configurations.
func (d *Discovery) WithParseExclude() *Discovery {
	d.parseExclude = true
	d.requiresParse = true

	return d
}

// WithParseIncludes enables parsing for include configurations.
func (d *Discovery) WithParseIncludes() *Discovery {
	d.parseIncludes = true
	d.requiresParse = true

	return d
}

// WithReadFiles enables parsing for file reading information.
func (d *Discovery) WithReadFiles() *Discovery {
	d.readFiles = true
	d.requiresParse = true

	return d
}

// WithSuppressParseErrors suppresses errors during parsing.
func (d *Discovery) WithSuppressParseErrors() *Discovery {
	d.suppressParseErrors = true
	return d
}

// WithBreakCycles enables breaking cycles in the dependency graph.
func (d *Discovery) WithBreakCycles() *Discovery {
	d.breakCycles = true
	return d
}

// WithRelationships enables relationship discovery.
func (d *Discovery) WithRelationships() *Discovery {
	d.discoverRelationships = true
	return d
}

// WithGitRoot sets the git root directory for dependent discovery boundary.
func (d *Discovery) WithGitRoot(gitRoot string) *Discovery {
	d.gitRoot = gitRoot
	return d
}

// WithGraphTarget sets the graph target so discovery can prune to the target and its dependents.
func (d *Discovery) WithGraphTarget(target string) *Discovery {
	d.graphTarget = target
	return d
}

// WithOptions ingests runner options and applies any discovery-relevant settings.
// Currently, it extracts HCL parser options provided via common.ParseOptionsProvider
// and graph target options, and forwards them to discovery's configuration.
func (d *Discovery) WithOptions(opts ...any) *Discovery {
	var parserOptions []hclparse.Option

	for _, opt := range opts {
		if p, ok := opt.(interface{ GetParseOptions() []hclparse.Option }); ok {
			parserOptions = append(parserOptions, p.GetParseOptions()...)
		}

		if g, ok := opt.(interface{ GraphTarget() string }); ok {
			if target := g.GraphTarget(); target != "" {
				d = d.WithGraphTarget(target)
			}
		}
	}

	if len(parserOptions) > 0 {
		d = d.WithParserOptions(parserOptions)
	}

	return d
}
