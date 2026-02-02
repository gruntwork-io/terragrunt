package filter

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// ClassificationStatus indicates whether a component is definitely included, a candidate, or excluded.
type ClassificationStatus int

const (
	// StatusDiscovered indicates the component is definitely included in results.
	StatusDiscovered ClassificationStatus = iota
	// StatusCandidate indicates the component might be included (needs further evaluation).
	StatusCandidate
	// StatusExcluded indicates the component is definitely excluded from results.
	StatusExcluded
)

// String returns a string representation of the ClassificationStatus.
func (cs ClassificationStatus) String() string {
	switch cs {
	case StatusDiscovered:
		return "discovered"
	case StatusCandidate:
		return "candidate"
	case StatusExcluded:
		return "excluded"
	default:
		return "unknown"
	}
}

// CandidacyReason explains why a component is classified as a candidate.
type CandidacyReason int

const (
	// CandidacyReasonNone indicates no candidacy reason (component is discovered or excluded).
	CandidacyReasonNone CandidacyReason = iota
	// CandidacyReasonGraphTarget indicates the component matches a graph expression target
	// and needs the graph phase to determine if it should be included.
	CandidacyReasonGraphTarget
	// CandidacyReasonRequiresParse indicates the component needs parsing to evaluate
	// attribute filters (e.g., reading=config/*).
	CandidacyReasonRequiresParse
	// CandidacyReasonPotentialDependent indicates the component is a potential dependent
	// when dependent filters (e.g., ...vpc) exist. These components need to be parsed
	// to build the dependency graph and determine if they are dependents of the target.
	CandidacyReasonPotentialDependent
)

// String returns a string representation of the CandidacyReason.
func (cr CandidacyReason) String() string {
	switch cr {
	case CandidacyReasonNone:
		return "none"
	case CandidacyReasonGraphTarget:
		return "graph-target"
	case CandidacyReasonRequiresParse:
		return "requires-parse"
	case CandidacyReasonPotentialDependent:
		return "potential-dependent"
	default:
		return "unknown"
	}
}

// ClassificationContext provides context for component classification.
type ClassificationContext struct {
	// ParseDataAvailable indicates whether parsed data is available for classification.
	ParseDataAvailable bool
}

// GraphExpressionInfo contains information about a graph expression for the classifier.
type GraphExpressionInfo struct {
	// Target is the target expression within the graph expression.
	Target Expression
	// FullExpression is the complete graph expression.
	FullExpression *GraphExpression
	// Index is the position of this expression in the original filter list.
	Index int
	// IncludeDependencies indicates if dependencies should be traversed.
	IncludeDependencies bool
	// IncludeDependents indicates if dependents should be traversed.
	IncludeDependents bool
	// ExcludeTarget indicates if the target itself should be excluded from results (^ prefix).
	ExcludeTarget bool
	// IsNegated indicates if this graph expression is within a negation (e.g., !...db).
	IsNegated bool
	// DependencyDepth is the maximum depth for dependency traversal.
	DependencyDepth int
	// DependentDepth is the maximum depth for dependent traversal.
	DependentDepth int
}

// Classifier analyzes filter expressions to efficiently classify components
// as discovered, candidate, or excluded without full evaluation.
type Classifier struct {
	logger             log.Logger
	filesystemExprs    []Expression
	parseExprs         []Expression
	graphExprs         []*GraphExpressionInfo
	gitExprs           []*GitExpression
	negatedExprs       []Expression
	hasPositiveFilters bool
}

// NewClassifier creates a new Classifier.
func NewClassifier(l log.Logger) *Classifier {
	return &Classifier{
		logger: l,
	}
}

// Analyze categorizes all filter expressions for efficient component classification.
// It separates filters into filesystem-evaluable, parse-required, and graph expressions.
func (c *Classifier) Analyze(filters Filters) error {
	c.filesystemExprs = nil
	c.parseExprs = nil
	c.graphExprs = nil
	c.gitExprs = nil
	c.negatedExprs = nil
	c.hasPositiveFilters = false

	for i, f := range filters {
		expr := f.Expression()
		if expr == nil {
			continue
		}

		c.analyzeExpression(expr, i)
	}

	return nil
}

// Classify determines whether a component should be discovered, is a candidate,
// or should be excluded based on the analyzed filters.
//
// Returns the classification status, the reason for candidacy (if applicable),
// and the index of the matching graph expression (-1 if not a graph target match).
//
// Classification algorithm:
//  1. Check if component ONLY matches negated filters -> EXCLUDED
//  2. Check if parse expressions exist and parse data unavailable -> CANDIDATE (RequiresParse)
//  3. Check if component matches any positive filesystem filter -> DISCOVERED
//  4. Check if component matches any git expression -> DISCOVERED
//  5. Check if component matches any graph expression target -> CANDIDATE (GraphTarget, returns index)
//  6. Check if dependent filters exist and parse data unavailable -> CANDIDATE (PotentialDependent)
//  7. If negated expressions exist and component doesn't match any -> DISCOVERED (negation acts as inclusion)
//  8. If positive filters exist but no match -> EXCLUDED (exclude-by-default)
//  9. If no positive filters exist -> DISCOVERED (include-by-default)
func (c *Classifier) Classify(comp component.Component, ctx ClassificationContext) (ClassificationStatus, CandidacyReason, int) {
	hasNegativeMatch := c.matchesAnyNegated(comp)
	hasPositiveMatch := c.matchesAnyPositive(comp, ctx)

	// Before excluding due to negation, check if the component matches a negated graph expression target.
	// If so, we need to process it through the graph phase to discover dependencies/dependents
	// that should also be excluded. The final filter evaluation will handle the actual exclusion.
	if hasNegativeMatch && !hasPositiveMatch {
		if graphIdx := c.matchesNegatedGraphExpressionTarget(comp); graphIdx >= 0 {
			return StatusCandidate, CandidacyReasonGraphTarget, graphIdx
		}

		return StatusExcluded, CandidacyReasonNone, -1
	}

	matchesFilesystem := c.matchesFilesystemExpression(comp)
	matchesGit := c.matchesGitExpression(comp)

	if len(c.parseExprs) > 0 && !ctx.ParseDataAvailable {
		return StatusCandidate, CandidacyReasonRequiresParse, -1
	}

	if matchesFilesystem {
		return StatusDiscovered, CandidacyReasonNone, -1
	}

	if matchesGit {
		return StatusDiscovered, CandidacyReasonNone, -1
	}

	if graphIdx := c.matchesGraphExpressionTarget(comp); graphIdx >= 0 {
		return StatusCandidate, CandidacyReasonGraphTarget, graphIdx
	}

	if c.HasDependentFilters() && !ctx.ParseDataAvailable {
		return StatusCandidate, CandidacyReasonPotentialDependent, -1
	}

	if len(c.negatedExprs) > 0 && !hasNegativeMatch {
		return StatusDiscovered, CandidacyReasonNone, -1
	}

	if c.hasPositiveFilters {
		return StatusExcluded, CandidacyReasonNone, -1
	}

	return StatusDiscovered, CandidacyReasonNone, -1
}

// analyzeExpression recursively analyzes an expression and categorizes it.
func (c *Classifier) analyzeExpression(expr Expression, filterIndex int) {
	switch node := expr.(type) {
	case *PathExpression:
		c.filesystemExprs = append(c.filesystemExprs, node)
		c.hasPositiveFilters = true

	case *AttributeExpression:
		if _, requiresParse := node.RequiresParse(); requiresParse {
			c.parseExprs = append(c.parseExprs, node)
		} else {
			c.filesystemExprs = append(c.filesystemExprs, node)
		}

		c.hasPositiveFilters = true

	case *GraphExpression:
		info := &GraphExpressionInfo{
			Target:              node.Target,
			FullExpression:      node,
			Index:               filterIndex,
			IncludeDependencies: node.IncludeDependencies,
			IncludeDependents:   node.IncludeDependents,
			ExcludeTarget:       node.ExcludeTarget,
			DependencyDepth:     node.DependencyDepth,
			DependentDepth:      node.DependentDepth,
		}
		c.graphExprs = append(c.graphExprs, info)
		c.hasPositiveFilters = true

	case *GitExpression:
		c.gitExprs = append(c.gitExprs, node)
		c.hasPositiveFilters = true

	case *PrefixExpression:
		// Right now, the only prefix operator is "!".
		// If we encounter an unknown operator, just analyze the inner expression.
		if node.Operator != "!" {
			c.analyzeExpression(node.Right, filterIndex)
			break
		}

		c.negatedExprs = append(c.negatedExprs, node.Right)
		if _, requiresParse := node.Right.RequiresParse(); requiresParse {
			c.parseExprs = append(c.parseExprs, node.Right)
		}

		c.extractNegatedGraphExpressions(node.Right, filterIndex)

	case *InfixExpression:
		c.analyzeExpression(node.Left, filterIndex)
		c.analyzeExpression(node.Right, filterIndex)
	}
}

// extractNegatedGraphExpressions walks through a negated expression and extracts
// any graph expressions found within it. This ensures that filters like "!...db"
// or "!db..." trigger the graph discovery phase.
func (c *Classifier) extractNegatedGraphExpressions(expr Expression, filterIndex int) {
	WalkExpressions(expr, func(e Expression) bool {
		if graphExpr, ok := e.(*GraphExpression); ok {
			info := &GraphExpressionInfo{
				Target:              graphExpr.Target,
				FullExpression:      graphExpr,
				Index:               filterIndex,
				IncludeDependencies: graphExpr.IncludeDependencies,
				IncludeDependents:   graphExpr.IncludeDependents,
				ExcludeTarget:       graphExpr.ExcludeTarget,
				DependencyDepth:     graphExpr.DependencyDepth,
				DependentDepth:      graphExpr.DependentDepth,
				IsNegated:           true,
			}
			c.graphExprs = append(c.graphExprs, info)
		}

		return true
	})
}

// matchesAnyNegated checks if the component matches any negated expression.
func (c *Classifier) matchesAnyNegated(comp component.Component) bool {
	return slices.ContainsFunc(c.negatedExprs, func(expr Expression) bool {
		match, _ := MatchComponent(comp, expr)
		return match
	})
}

// matchesAnyPositive checks if the component matches any positive (non-negated) expression.
func (c *Classifier) matchesAnyPositive(comp component.Component, ctx ClassificationContext) bool {
	if c.matchesFilesystemExpression(comp) {
		return true
	}

	if c.matchesGraphExpressionTarget(comp) >= 0 {
		return true
	}

	if c.matchesGitExpression(comp) {
		return true
	}

	if !ctx.ParseDataAvailable || len(c.parseExprs) == 0 {
		return false
	}

	return slices.ContainsFunc(c.parseExprs, func(expr Expression) bool {
		match, _ := MatchComponent(comp, expr)
		return match
	})
}

// matchesGitExpression checks if a component matches any git expression.
// Components discovered from worktrees have a Ref set in their discovery context.
func (c *Classifier) matchesGitExpression(comp component.Component) bool {
	discoveryCtx := comp.DiscoveryContext()
	if discoveryCtx == nil || discoveryCtx.Ref == "" {
		return false
	}

	return slices.ContainsFunc(c.gitExprs, func(gitExpr *GitExpression) bool {
		return discoveryCtx.Ref == gitExpr.FromRef || discoveryCtx.Ref == gitExpr.ToRef
	})
}

// matchesFilesystemExpression checks if the component matches any filesystem-evaluable expression.
func (c *Classifier) matchesFilesystemExpression(comp component.Component) bool {
	return slices.ContainsFunc(c.filesystemExprs, func(expr Expression) bool {
		match, _ := MatchComponent(comp, expr)
		return match
	})
}

// matchesGraphExpressionTarget checks if the component matches any non-negated graph expression target.
// Returns the index of the matching graph expression, or -1 if no match.
// Negated graph expressions are handled separately by matchesNegatedGraphExpressionTarget.
func (c *Classifier) matchesGraphExpressionTarget(comp component.Component) int {
	return slices.IndexFunc(c.graphExprs, func(info *GraphExpressionInfo) bool {
		if info.IsNegated {
			return false
		}

		match, _ := MatchComponent(comp, info.Target)

		return match
	})
}

// matchesNegatedGraphExpressionTarget checks if the component matches any negated graph expression target.
// Returns the index of the matching graph expression, or -1 if no match.
// This is used to identify components that need graph traversal even when they would otherwise be excluded.
func (c *Classifier) matchesNegatedGraphExpressionTarget(comp component.Component) int {
	return slices.IndexFunc(c.graphExprs, func(info *GraphExpressionInfo) bool {
		if !info.IsNegated {
			return false
		}

		match, _ := MatchComponent(comp, info.Target)

		return match
	})
}

// GraphExpressions returns the analyzed graph expressions.
func (c *Classifier) GraphExpressions() []*GraphExpressionInfo {
	return c.graphExprs
}

// HasPositiveFilters returns whether any positive (non-negated) filters exist.
func (c *Classifier) HasPositiveFilters() bool {
	return c.hasPositiveFilters
}

// HasParseRequiredFilters returns whether any filters require HCL parsing.
func (c *Classifier) HasParseRequiredFilters() bool {
	return len(c.parseExprs) > 0
}

// HasGraphFilters returns whether any graph traversal filters exist.
func (c *Classifier) HasGraphFilters() bool {
	return len(c.graphExprs) > 0
}

// HasDependentFilters returns whether any graph expressions include dependent traversal.
// This is used to determine if pre-graph dependency building is needed to populate
// reverse links before dependent discovery can work.
func (c *Classifier) HasDependentFilters() bool {
	return slices.ContainsFunc(c.graphExprs, func(expr *GraphExpressionInfo) bool {
		return expr.IncludeDependents
	})
}

// ParseExpressions returns the expressions that require parsing.
func (c *Classifier) ParseExpressions() []Expression {
	return c.parseExprs
}

// NegatedExpressions returns the negated expressions.
func (c *Classifier) NegatedExpressions() []Expression {
	return c.negatedExprs
}
