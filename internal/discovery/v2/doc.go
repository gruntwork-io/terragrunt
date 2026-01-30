// Package v2 provides a channel-based phased discovery architecture for Terragrunt components.
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
// # Classification Rules
//
// The [CandidacyClassifier] analyzes all filter expressions upfront and classifies
// each component into one of three statuses:
//
//   - [StatusDiscovered]: Matches a positive filter (path, attribute, or git expression)
//   - [StatusCandidate]: Needs further evaluation (graph target, requires parsing, or potential dependent)
//   - [StatusExcluded]: Only matches negated filters, or positive filters exist but none match
//
// When no positive filters exist, components are included by default. When positive
// filters exist, only matching components are included.
//
// # Phase Flow
//
// The discovery process executes in the following phases:
//
//  1. Filesystem + Worktree Discovery (concurrent)
//     - [PhaseFilesystem]: Walk directories recursively, classify components via [CandidacyClassifier]
//     - [PhaseWorktree]: For Git filters [ref...ref], discover components in temporary worktrees
//     and detect added/removed/modified components via SHA256 comparison
//
//  2. Parse Phase (if needed)
//     - [PhaseParse]: Parse HCL configs for candidates with [CandidacyReasonRequiresParse]
//     - Re-classify based on parsed attributes (reading, source), promote to discovered or
//     transition to graph candidate
//
//  3. Graph Phase (if needed)
//     - Pre-graph: If dependent filters exist, parse all components and build bidirectional
//     dependency links for reverse traversal
//     - [PhaseGraph]: Traverse dependencies (target|N) and/or dependents (...target) based on
//     [GraphExpressionInfo] configuration
//     - Supports depth limits and target exclusion (^target) for flexible graph queries
//
//  4. Relationship Phase (optional)
//     - [PhaseRelationship]: Build complete dependency graph for execution ordering
//     - Creates transient components for external dependencies (not in final results)
//
//  5. Final Phase
//     - [PhaseFinal]: Merge all discovered, deduplicate by path, apply final filter evaluation
//     - Cycle detection and removal if configured via WithBreakCycles
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
//	d := v2.New(workingDir).
//		WithFilters(filters).
//		WithDiscoveryContext(discoveryContext).
//		WithRelationships().
//		WithMaxDependencyDepth(10)
//
//	components, err := d.Discover(ctx, logger, opts)
//
// # Thread Safety
//
// All phase communication uses channels with no shared mutable state between phases.
// [component.ThreadSafeComponents] provides concurrent component access during graph traversal.
// A custom stringSet (RWMutex-based) tracks seen components during traversal.
// [errgroup] with configurable worker limits (default 4, max 8) handles concurrent operations.
package v2
