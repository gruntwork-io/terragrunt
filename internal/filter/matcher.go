package filter

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
)

// MatchComponent checks if a single component matches an expression.
// This is the shared core used by both Classifier and Evaluate.
func MatchComponent(c component.Component, expr Expression) bool {
	switch node := expr.(type) {
	case *PathExpression:
		return matchPath(c, node)

	case *AttributeExpression:
		return matchAttribute(c, node)

	case *PrefixExpression:
		if node.Operator != "!" {
			return false
		}

		return MatchComponent(c, node.Right)

	case *InfixExpression:
		if node.Operator != "|" {
			return false
		}

		if !MatchComponent(c, node.Left) {
			return false
		}

		return MatchComponent(c, node.Right)

	case *GraphExpression:
		return MatchComponent(c, node.Target)

	case *GitExpression:
		return matchGit(c, node)

	default:
		return false
	}
}

// matchPath checks if a component matches a path expression.
func matchPath(c component.Component, expr *PathExpression) bool {
	g := expr.Glob()

	componentPath := c.Path()

	// If the pattern is absolute, match against absolute path
	if filepath.IsAbs(expr.Value) {
		return g.Match(filepath.ToSlash(componentPath))
	}

	// Try to get relative path from discovery context
	discoveryCtx := c.DiscoveryContext()
	if discoveryCtx != nil && discoveryCtx.WorkingDir != "" {
		relPath, err := filepath.Rel(discoveryCtx.WorkingDir, componentPath)
		if err == nil {
			return g.Match(filepath.ToSlash(relPath))
		}
	}

	// Fall back to matching the path as-is
	return g.Match(filepath.ToSlash(componentPath))
}

// matchAttribute checks if a component matches an attribute expression.
// This handles attributes that can be evaluated without parsing (name, type, external).
// For attributes requiring parsing (reading, source), this returns false.
func matchAttribute(c component.Component, expr *AttributeExpression) bool {
	switch expr.Key {
	case AttributeName:
		return expr.Glob().Match(filepath.Base(c.Path()))

	case AttributeType:
		switch expr.Value {
		case AttributeTypeValueUnit:
			_, ok := c.(*component.Unit)
			return ok
		case AttributeTypeValueStack:
			_, ok := c.(*component.Stack)
			return ok
		}

		return false

	case AttributeExternal:
		switch expr.Value {
		case AttributeExternalValueTrue:
			return c.External()
		case AttributeExternalValueFalse:
			return !c.External()
		}

		return false

	case AttributeReading:
		// Reading attribute requires parsing, can't evaluate without parsed data
		return false

	case AttributeSource:
		// Source attribute requires parsing, can't evaluate without parsed data
		return false

	default:
		return false
	}
}

// matchGit checks if a component matches a git expression.
// Components discovered from worktrees have a Ref set in their discovery context.
func matchGit(c component.Component, expr *GitExpression) bool {
	discoveryCtx := c.DiscoveryContext()
	if discoveryCtx == nil || discoveryCtx.Ref == "" {
		return false
	}

	return discoveryCtx.Ref == expr.FromRef || discoveryCtx.Ref == expr.ToRef
}
