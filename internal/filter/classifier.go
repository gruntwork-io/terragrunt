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
		// Git expressions are handled by the worktree phase, but we need to track them
		// so components discovered in worktrees can be matched during classification.
		c.gitExprs = append(c.gitExprs, node)
		c.hasPositiveFilters = true

	case *PrefixExpression:
		if node.Operator == "!" {
			c.negatedExprs = append(c.negatedExprs, node.Right)
			// Also track if the negated expression requires parsing.
			// For example, "!reading=shared.hcl" still needs parsing to evaluate the reading attribute.
			if _, requiresParse := node.Right.RequiresParse(); requiresParse {
				c.parseExprs = append(c.parseExprs, node.Right)
			}
		} else {
			// Unknown prefix operator, analyze inner expression
			c.analyzeExpression(node.Right, filterIndex)
		}

	case *InfixExpression:
		// For infix expressions (intersection with |), analyze both sides
		c.analyzeExpression(node.Left, filterIndex)
		c.analyzeExpression(node.Right, filterIndex)
	}
}

// Classify determines whether a component should be discovered, is a candidate,
// or should be excluded based on the analyzed filters.
//
// Classification algorithm:
//  1. Check if component ONLY matches negated filters -> EXCLUDED
//  2. Check if component matches any positive filesystem filter -> DISCOVERED
//  3. Check if component matches any graph expression target -> CANDIDATE (GraphTarget)
//  4. Check if parse expressions exist and component not yet classified -> CANDIDATE (RequiresParse)
//  5. Check if dependent filters exist (component might be a dependent) -> CANDIDATE (PotentialDependent)
//  6. If positive filters exist but no match -> EXCLUDED (exclude-by-default)
//  7. If no positive filters exist -> DISCOVERED (include-by-default)
func (c *Classifier) Classify(comp component.Component, ctx ClassificationContext) (ClassificationStatus, CandidacyReason, int) {
	hasNegativeMatch := c.matchesAnyNegated(comp)
	hasPositiveMatch := c.matchesAnyPositive(comp, ctx)

	if hasNegativeMatch && !hasPositiveMatch {
		return StatusExcluded, CandidacyReasonNone, -1
	}

	matchesFilesystem := c.matchesFilesystemExpression(comp)
	matchesGit := c.matchesGitExpression(comp)

	// If there are parse-required expressions and parsing hasn't happened yet,
	// components matching filesystem/git expressions should be candidates, not discovered.
	// This is necessary for intersection filters like "./apps/** | reading=shared.hcl"
	// where we need parsing to verify the second part of the filter.
	if len(c.parseExprs) > 0 && !ctx.ParseDataAvailable {
		if matchesFilesystem || matchesGit {
			return StatusCandidate, CandidacyReasonRequiresParse, -1
		}

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

	if c.hasPositiveFilters {
		return StatusExcluded, CandidacyReasonNone, -1
	}

	return StatusDiscovered, CandidacyReasonNone, -1
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

// matchesGraphExpressionTarget checks if the component matches any graph expression target.
// Returns the index of the matching graph expression, or -1 if no match.
func (c *Classifier) matchesGraphExpressionTarget(comp component.Component) int {
	return slices.IndexFunc(c.graphExprs, func(info *GraphExpressionInfo) bool {
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
	for _, expr := range c.graphExprs {
		if expr.IncludeDependents {
			return true
		}
	}

	return false
}

// ParseExpressions returns the expressions that require parsing.
func (c *Classifier) ParseExpressions() []Expression {
	return c.parseExprs
}

// NegatedExpressions returns the negated expressions.
func (c *Classifier) NegatedExpressions() []Expression {
	return c.negatedExprs
}
