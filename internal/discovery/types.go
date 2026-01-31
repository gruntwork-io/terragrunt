package discovery

import (
	"context"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/report"
	"github.com/gruntwork-io/terragrunt/internal/worktrees"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/gruntwork-io/terragrunt/pkg/options"
)

// Type aliases for filter package types used throughout discovery.
// These provide backward compatibility and shorter type names within the discovery package.
type (
	// ClassificationStatus is an alias for filter.ClassificationStatus.
	ClassificationStatus = filter.ClassificationStatus
	// CandidacyReason is an alias for filter.CandidacyReason.
	CandidacyReason = filter.CandidacyReason
	// GraphExpressionInfo is an alias for filter.GraphExpressionInfo.
	GraphExpressionInfo = filter.GraphExpressionInfo
)

// Status constants are aliases for filter package constants.
const (
	StatusDiscovered = filter.StatusDiscovered
	StatusCandidate  = filter.StatusCandidate
	StatusExcluded   = filter.StatusExcluded
)

// CandidacyReason constants are aliases for filter package constants.
const (
	CandidacyReasonNone               = filter.CandidacyReasonNone
	CandidacyReasonGraphTarget        = filter.CandidacyReasonGraphTarget
	CandidacyReasonRequiresParse      = filter.CandidacyReasonRequiresParse
	CandidacyReasonPotentialDependent = filter.CandidacyReasonPotentialDependent
)

// PhaseKind identifies the type of discovery phase.
type PhaseKind int

const (
	// PhaseFilesystem walks directories to find terragrunt configurations.
	PhaseFilesystem PhaseKind = iota
	// PhaseWorktree discovers components in Git worktrees (concurrent with Filesystem).
	PhaseWorktree
	// PhaseParse parses HCL configurations for filter evaluation.
	PhaseParse
	// PhaseGraph traverses dependency/dependent relationships.
	PhaseGraph
	// PhaseRelationship builds dependency graph for orphan components.
	PhaseRelationship
	// PhaseFinal applies final filter evaluation and cycle checking.
	PhaseFinal
)

// String returns a string representation of the PhaseKind.
func (pk PhaseKind) String() string {
	switch pk {
	case PhaseFilesystem:
		return "filesystem"
	case PhaseWorktree:
		return "worktree"
	case PhaseParse:
		return "parse"
	case PhaseGraph:
		return "graph"
	case PhaseRelationship:
		return "relationship"
	case PhaseFinal:
		return "final"
	default:
		return "unknown"
	}
}

// DiscoveryResult represents a discovered or candidate component with metadata.
type DiscoveryResult struct {
	// Component is the discovered Terragrunt component.
	Component component.Component
	// Status indicates whether this is a definite discovery, candidate, or excluded.
	Status ClassificationStatus
	// Reason explains why the component is a candidate (only meaningful when Status == StatusCandidate).
	Reason CandidacyReason
	// Phase indicates which phase produced this result.
	Phase PhaseKind
	// GraphExpressionIndex is the index of the graph expression that matched (for candidates).
	// This is used during the graph phase to determine how to traverse.
	GraphExpressionIndex int
}

// PhaseInput provides input data to a discovery phase.
type PhaseInput struct {
	Logger     log.Logger
	Opts       *options.TerragruntOptions
	Classifier *filter.Classifier
	Discovery  *Discovery
	Components component.Components
	Candidates []DiscoveryResult
}

// PhaseOutput provides output channels from a discovery phase.
type PhaseOutput struct {
	// Discovered receives components definitively included in results.
	Discovered <-chan DiscoveryResult
	// Candidates receives components that might be included pending further evaluation.
	Candidates <-chan DiscoveryResult
	// Done is closed when the phase completes.
	Done <-chan struct{}
	// Errors receives any errors that occur during the phase.
	Errors <-chan error
}

// Phase defines the interface for a discovery phase.
type Phase interface {
	// Name returns the human-readable name of the phase.
	Name() string
	// Kind returns the PhaseKind identifier.
	Kind() PhaseKind
	// Run executes the phase with the given input and returns output channels.
	Run(ctx context.Context, input *PhaseInput) PhaseOutput
}

// Discovery is the main configuration for discovery.
type Discovery struct {
	// discoveryContext is the context in which the discovery is happening.
	discoveryContext *component.DiscoveryContext

	// worktrees is the worktrees created for Git-based filters.
	worktrees *worktrees.Worktrees

	// report is used for recording excluded external dependencies during discovery.
	report *report.Report

	// workingDir is the directory to search for Terragrunt configurations.
	workingDir string

	// gitRoot is the git repository root, used as boundary for dependent discovery.
	gitRoot string

	// graphTarget is the target path for graph filtering (prune to target + dependents).
	graphTarget string

	// configFilenames is the list of config filenames to discover. If nil, defaults are used.
	configFilenames []string

	// parserOptions are custom HCL parser options to use when parsing during discovery.
	parserOptions []hclparse.Option

	// filters contains filter queries for component selection.
	filters filter.Filters

	// classifier categorizes filter expressions for efficient evaluation.
	classifier *filter.Classifier

	// gitExpressions contains Git filter expressions that require worktree discovery.
	gitExpressions filter.GitExpressions

	// maxDependencyDepth is the maximum depth of the dependency tree to discover.
	maxDependencyDepth int

	// numWorkers determines the number of concurrent workers for discovery operations.
	numWorkers int

	// noHidden determines whether to detect configurations in hidden directories.
	noHidden bool

	// requiresParse is true when the discovery requires parsing Terragrunt configurations.
	requiresParse bool

	// parseExclude determines whether to parse exclude configurations.
	parseExclude bool

	// readFiles determines whether to parse for reading files.
	readFiles bool

	// suppressParseErrors determines whether to suppress errors when parsing Terragrunt configurations.
	suppressParseErrors bool

	// breakCycles determines whether to break cycles in the dependency graph if any exist.
	breakCycles bool

	// excludeByDefault determines whether to exclude configurations by default (triggered by include flags).
	excludeByDefault bool

	// discoverRelationships determines whether to run relationship discovery.
	discoverRelationships bool
}
