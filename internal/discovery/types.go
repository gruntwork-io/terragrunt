package discovery

import (
	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
)

// DiscoveryResult represents a discovered or candidate component with metadata.
type DiscoveryResult struct {
	// Component is the discovered Terragrunt component.
	Component component.Component
	// Status indicates whether this component is ready for filter evaluation, a candidate
	// for further processing, or excluded.
	Status filter.ClassificationStatus
	// Reason explains why the component is a candidate (only meaningful when Status == filter.StatusCandidate).
	Reason filter.CandidacyReason
	// GraphExpressionIndex is the index of the graph expression that matched (for candidates).
	// The graph traversal uses it to determine how to traverse.
	GraphExpressionIndex int
}

// Discovery is the main configuration for discovery.
type Discovery struct {
	// discoveryContext is the context in which the discovery is happening.
	discoveryContext *component.DiscoveryContext

	// worktrees is the worktrees created for Git-based filters.
	worktrees *worktrees.Worktrees

	// workingDir is the logical working directory for filter evaluation and display paths.
	workingDir string

	// walkRoot overrides the filesystem walk root when set. The pipeline
	// walks walkRoot instead of workingDir, while workingDir stays the
	// logical base for relative path filters and display paths. Empty means
	// the walk starts at workingDir.
	walkRoot string

	// resolvedWorkingDir is workingDir with symlinks resolved, which is how
	// boundaries and dependency paths name it. Discover fills it in before the
	// pipeline runs.
	resolvedWorkingDir string

	// paths memoizes symlink resolution for one Discover call. Discover fills
	// it in before the pipeline runs.
	paths *vfs.PathResolver

	// gitRoot is the detected git repository root: the default ceiling for the
	// upstream dependent walk. It is detected lazily and only when needed, so
	// it stays empty when a discoveryBoundary is set (see
	// [Discovery.dependentWalkBoundary]).
	gitRoot string

	// discoveryBoundary is the user-supplied --discovery-boundary enclosure (resolved
	// to an absolute path). When set, it caps the dependent walk in place of
	// gitRoot and prunes dependencies that resolve outside it. Empty unless
	// --discovery-boundary is set.
	discoveryBoundary string

	// discoveryBoundaryInput is --discovery-boundary as given, which Git targets resolve against their worktree root.
	discoveryBoundaryInput string

	// worktreeGitRoot is the Git root that absolute boundaries are mirrored from into worktrees.
	worktreeGitRoot string

	// graphTarget is the target path for graph filtering (prune to target + dependents).
	graphTarget string

	// configFilenames is the list of config filenames to discover. If nil, defaults are used.
	configFilenames []string

	// filters contains filter queries for component selection.
	filters filter.Filters

	// classifier categorizes filter expressions for efficient evaluation.
	classifier *filter.Classifier

	// gitExpressions contains Git filter expressions that require worktree discovery.
	gitExpressions filter.GitExpressions

	// parseReasons gates parsing in the pipeline: non-empty (or classifier
	// parse-required filters at activation) turns it on. Surfaced on telemetry.
	parseReasons []parseReason

	// inputsReads collects the file reads found in inputs attributes while trackReads is on,
	// for the tip Discover gives.
	inputsReads inputsReads

	// maxDependencyDepth is the maximum depth of the dependency tree to discover.
	maxDependencyDepth int

	// numWorkers determines the number of concurrent workers for discovery operations.
	numWorkers int

	// noHidden determines whether to detect configurations in hidden directories.
	noHidden bool

	// parseExclude determines whether to parse exclude configurations.
	parseExclude bool

	// parseIncludes determines whether to parse for include configurations.
	parseIncludes bool

	// readFiles determines whether to parse for reading files.
	readFiles bool

	// trackReads determines whether parsing records the files each component
	// reads. Recording them costs a walk of every local module source, so it
	// stays off until something asks to see them.
	trackReads bool

	// parseStackConfigs determines whether to parse discovered stack config files.
	parseStackConfigs bool

	// suppressParseErrors determines whether to suppress errors when parsing Terragrunt configurations.
	suppressParseErrors bool

	// breakCycles determines whether to break cycles in the dependency graph if any exist.
	breakCycles bool

	// excludeByDefault determines whether to exclude configurations by default (triggered by include flags).
	excludeByDefault bool

	// discoverRelationships determines whether to run relationship discovery.
	discoverRelationships bool
}
