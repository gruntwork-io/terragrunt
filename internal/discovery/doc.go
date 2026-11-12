// Package discovery provides a channel-based phased discovery architecture for Terragrunt components.
//
// # Overview
//
// This package discovers Terragrunt components (units and stacks) across a directory tree
// using a multi-phase pipeline.
//
// Each phase communicates via two output channels:
//   - discovered: Components definitively included in results
//   - candidates: Components that might be included pending further evaluation
//
// This dual-channel approach enables lazy evaluation. Components are only parsed or
// graph-traversed when necessary for filter evaluation.
//
// # Constructors
//
// The package provides several constructors for different use cases:
//
//   - [NewDiscovery]: Creates a Discovery with sensible defaults including CPU-aware worker
//     count (scales with runtime.GOMAXPROCS, min 4, max 8) and pre-initialized [component.DiscoveryContext].
//     This is the recommended constructor for most use cases.
//
//   - [NewForDiscoveryCommand]: Creates a Discovery configured for discovery commands (find/list)
//     with parse error suppression and cycle breaking enabled.
//
//   - [NewFiltered]: Creates a Discovery that applies filters and a discovery boundary, for
//     commands that act on the components it finds, such as hcl validate and render --all.
//
//   - [NewForStackGenerate]: Creates a Discovery for stack generate commands.
//
// # Classification Rules
//
// The [filter.Classifier] analyzes all filter expressions upfront and classifies
// each component into one of three statuses:
//
//   - [filter.StatusReadyForFilter]: Passed classification, ready for final filter evaluation
//   - [filter.StatusCandidate]: Needs further evaluation (graph target, requires parsing, or potential dependent)
//   - [filter.StatusExcluded]: Only matches negated filters, or positive filters exist but none match
//
// When no positive filters exist, components are included by default. When positive
// filters exist, only matching components are included.
//
// # Phase Flow
//
// The discovery process executes in the following phases:
//
//  1. Filesystem + Worktree Discovery (concurrent)
//     - [PhaseFilesystem]: Walk directories recursively, classify components via [filter.Classifier]
//     - [PhaseWorktree]: For Git filters [ref...ref], discover components in temporary worktrees
//     and detect added/removed/modified components via SHA256 comparison
//
//  2. Parse Phase (if needed)
//     - [PhaseParse]: Parse HCL configs for candidates with [filter.CandidacyReasonRequiresParse]
//     - Re-classify based on parsed attributes (reading, source), promote to discovered or
//     transition to graph candidate
//
//  3. Graph Phase (if needed)
//     - Pre-graph: If dependent filters exist, parse all components and build bidirectional
//     dependency links for reverse traversal
//     - [PhaseGraph]: Traverse dependencies (target|N) and/or dependents (...target) based on
//     [filter.GraphExpressionInfo] configuration
//     - Supports depth limits and target exclusion (^target) for flexible graph queries
//
//  4. Relationship Phase (optional)
//     - [PhaseRelationship]: Build complete dependency graph for execution ordering
//     - Creates transient components for external dependencies (not in final results)
//
//  5. Final Phase
//     - [PhaseFinal]: Merge all discovered, deduplicate by path, apply final filter evaluation
//     - Cycle detection and removal if configured via [Discovery.WithBreakCycles]
//
// # Pipeline
//
// [EnvPipeline] replaces phases 1 and 2 with a pipeline that parses components while the walk is still finding
// them. The walk and the parse workers report to one collector, which hands the graph phase the same discovered
// components and candidates the two phases would. A discovery with Git filters keeps the phases.
//
// The pipeline also replaces phase 4. After the graph phase, it reads the dependencies of each discovered unit no
// worker read, and links the components on one goroutine. A dependency that is not a discovered component becomes a
// transient unit. Each discovered unit then walks the transient units it reaches, reading their dependencies, until
// it has reached every other discovered unit or has nothing left to follow, so units joined only through transient
// units are related. The walks share their reads, so each transient unit is parsed at most once.
//
// When a filter selects dependents, the workers also read the dependencies of every candidate a dependent traversal
// can reach, and the pre-graph step of phase 3 links them on one goroutine without parsing anything again.
//
// The dependency traversals of phase 3 also run on one goroutine from what the workers read, with the dependencies
// at each depth read together. Phase 3 stops at a component an earlier traversal reached, even one that had less
// depth left, so its results depend on which traversal gets there first. The pipeline continues from such a
// component when it has more depth left.
//
// # Filter Expressions
//
// The package supports several filter expression types:
//
//   - Path expressions: ./foo, ./foo/**, ./**/vpc (glob patterns)
//   - Attribute expressions: name=vpc, type=unit, external=true, reading=config/*, source=*
//   - Graph expressions: vpc (target), vpc|2 (dependencies), ...vpc (dependents), ^vpc|... (exclude target)
//   - Git expressions: [main...develop] (changes between refs)
//   - Negated expressions: !./internal (exclusion)
//
// # Example Usage
//
//	d := NewDiscovery(workingDir).
//		WithFilters(filters).
//		WithRelationships().
//		WithMaxDependencyDepth(10)
//
//	components, err := d.Discover(ctx, logger, v, s, opts)
//
// # Thread Safety
//
// All phase communication uses channels with no shared mutable state between phases.
// [component.ThreadSafeComponents] provides concurrent component access during graph traversal.
// A custom stringSet (RWMutex-based) tracks seen components during traversal.
// [errgroup] with configurable worker limits (default 4, max 8) handles concurrent operations.
package discovery
