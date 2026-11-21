// Package discovery finds the Terragrunt components (units and stacks) in a directory tree and selects the ones the
// filters ask for.
//
// # Overview
//
// Discovery sorts what it finds into two sets:
//   - discovered: Components definitively included in results
//   - candidates: Components that might be included pending further evaluation
//
// This split enables lazy evaluation. Components are only parsed or graph-traversed when necessary for filter
// evaluation.
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
// # Flow
//
// [Discovery.Discover] runs these steps in order:
//
//  1. Pipeline
//     - A walk of the working directory classifies each component via [filter.Classifier]
//     - For Git filters [ref...ref], a second finder discovers components in temporary worktrees beside the walk
//     and detects added/removed/modified components by comparing their files
//     - Workers parse the candidates with [filter.CandidacyReasonRequiresParse] while the walk is still finding
//     components, and classify them again from the parsed attributes (reading, source)
//     - Workers also read the dependencies each component declares, as paths, when a later step needs them
//     - The walk and the worktree discovery keep what they find, and one collector receives what the workers report
//
//  2. Graph traversal (if a filter traverses the graph)
//     - Traverses dependencies (target|N) and/or dependents (...target) based on
//     [filter.GraphExpressionInfo] configuration
//     - Supports depth limits and target exclusion (^target) for flexible graph queries
//
//  3. Relationships (optional, see [Discovery.WithRelationships])
//     - Links every discovered component to its dependencies for execution ordering
//     - Creates transient components for external dependencies (not in final results)
//
//  4. Final evaluation
//     - Applies the filters to the discovered components
//     - Cycle detection and removal if configured via [Discovery.WithBreakCycles]
//
// # Dependencies
//
// Steps 2 and 3 take dependencies from what the workers read, and run on one goroutine each. A unit that no worker
// read, such as one the graph traversal reached, has its dependencies read when a step needs them, together with
// the others at the same depth.
//
// A dependency that is not a discovered component becomes a transient unit. Each discovered unit then walks the
// transient units it reaches, reading their dependencies, until it has reached every other discovered unit or has
// nothing left to follow, so units joined only through transient units are related. The walks share their reads, so
// each transient unit is parsed at most once.
//
// A component that several graph traversals reach is traversed from again only by one with more depth left, so the
// results do not depend on the order of the traversals.
//
// When a filter selects dependents, the workers read the dependencies of every candidate a dependent traversal can
// reach, and the components are linked before the traversal. For a target with dependents outside the working
// directory, discovery walks up to each dependent boundary once, reads the dependencies of the units there together,
// and leaves the dependents to the traversal of the links. The depth of a filter such as 1...vpc counts
// dependencies only, however many directories lie between a dependent and the target. For a target found in a Git
// worktree, the search covers that worktree within the boundary there.
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
// The walk and the worktree discovery each own what they find until the pipeline has drained. The parse workers
// report over a channel to one collector, which alone holds what they parsed and read. Components are shared between
// goroutines, such as a unit the walk holds while a worker parses it, and each component guards its fields with its
// own lock. The graph traversal and the linking of dependencies each run on
// one goroutine after the pipeline has drained, and read the dependencies of the units at each depth concurrently.
package discovery
