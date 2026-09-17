package discovery

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// addParseReason is idempotent so repeated option calls don't duplicate.
func (d *Discovery) addParseReason(r parseReason) {
	if !slices.Contains(d.parseReasons, r) {
		d.parseReasons = append(d.parseReasons, r)
	}
}

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

	if d.filters.HasPositiveFilter() {
		d.excludeByDefault = true
	}

	if _, ok := d.filters.RequiresParse(); ok {
		d.addParseReason(parseReasonFiltersRequireParse)
	}

	if d.filters.RequiresReading() {
		d = d.WithTrackReads()
	}

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

// ParsesConfigs reports whether the options set so far commit discovery to
// parsing Terragrunt configurations. The classifier can still call for a parse
// later, from a filter expression that exists only once a Git diff has been
// expanded.
func (d *Discovery) ParsesConfigs() bool {
	return len(d.parseReasons) > 0
}

// WithRequiresParse enables parsing of Terragrunt configurations.
func (d *Discovery) WithRequiresParse() *Discovery {
	d.addParseReason(parseReasonExplicit)
	return d
}

// WithParseExclude enables parsing of exclude configurations.
func (d *Discovery) WithParseExclude() *Discovery {
	d.parseExclude = true
	d.addParseReason(parseReasonParseExclude)

	return d
}

// WithParseIncludes enables parsing for include configurations.
func (d *Discovery) WithParseIncludes() *Discovery {
	d.parseIncludes = true
	d.addParseReason(parseReasonParseIncludes)

	return d
}

// WithParseStackConfigs enables parsing of discovered stack config files, populating
// each stack component's config. Parsing is best-effort: a stack whose config fails
// to parse is left without one.
func (d *Discovery) WithParseStackConfigs() *Discovery {
	d.parseStackConfigs = true
	return d
}

// WithReadFiles parses every discovered component so that all of them report the
// files they read, rather than only those a filter forces through the parser.
func (d *Discovery) WithReadFiles() *Discovery {
	d.readFiles = true
	d.addParseReason(parseReasonReadFiles)

	return d.WithTrackReads()
}

// WithTrackReads records, for each component parsing visits, the files it read.
// Discovery derives this from the filters it is given; callers that consume
// [component.Component.Reading] without a reading filter to ask for it, such as
// the browse TUI, set it themselves.
func (d *Discovery) WithTrackReads() *Discovery {
	d.trackReads = true

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

// WithGitRoot sets the git repository root used as the default ceiling for the
// upstream dependent walk, bypassing automatic detection.
func (d *Discovery) WithGitRoot(gitRoot string) *Discovery {
	d.gitRoot = gitRoot
	return d
}

// WithDiscoveryBoundary sets the directory that encloses graph discovery for
// filters, in place of the automatically detected git repository root:
// dependencies and dependents resolving outside it are not discovered. The
// path may be relative, in which case it is resolved against the discovery
// working directory.
func (d *Discovery) WithDiscoveryBoundary(boundary string) *Discovery {
	d.discoveryBoundary = boundary
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
