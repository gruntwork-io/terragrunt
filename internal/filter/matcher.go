package filter

import (
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/component"
)

// MatchComponent checks if a single component matches an expression.
// This is the shared core used by both Classifier and Evaluate.
func MatchComponent(c component.Component, expr Expression) (bool, error) {
	switch node := expr.(type) {
	case *PathExpression:
		return matchPath(c, node)

	case *AttributeExpression:
		return matchAttribute(c, node)

	case *PrefixExpression:
		if node.Operator != "!" {
			return false, nil
		}

		return MatchComponent(c, node.Right)

	case *InfixExpression:
		if node.Operator != "|" {
			return false, nil
		}

		leftMatch, err := MatchComponent(c, node.Left)
		if err != nil {
			return false, err
		}

		if !leftMatch {
			return false, nil
		}

		return MatchComponent(c, node.Right)

	case *GraphExpression:
		return MatchComponent(c, node.Target)

	case *GitExpression:
		return matchGit(c, node), nil

	default:
		return false, nil
	}
}

// matchPath checks if a component matches a path expression.
func matchPath(c component.Component, expr *PathExpression) (bool, error) {
	g, err := expr.CompileGlob()
	if err != nil {
		return false, err
	}

	componentPath := c.Path()

	// If the pattern is absolute, match against absolute path
	if filepath.IsAbs(expr.Value) {
		return g.Match(filepath.ToSlash(componentPath)), nil
	}

	// Try to get relative path from discovery context
	discoveryCtx := c.DiscoveryContext()
	if discoveryCtx != nil && discoveryCtx.WorkingDir != "" {
		relPath, err := filepath.Rel(discoveryCtx.WorkingDir, componentPath)
		if err == nil {
			return g.Match(filepath.ToSlash(relPath)), nil
		}
	}

	// Fall back to matching the path as-is
	return g.Match(filepath.ToSlash(componentPath)), nil
}

// matchAttribute checks if a component matches an attribute expression.
// This handles attributes that can be evaluated without parsing (name, type, external).
// For attributes requiring parsing (reading, source), this returns false.
func matchAttribute(c component.Component, expr *AttributeExpression) (bool, error) {
	switch expr.Key {
	case AttributeName:
		g, err := expr.CompileGlob()
		if err != nil {
			return false, err
		}

		return g.Match(filepath.Base(c.Path())), nil

	case AttributeType:
		switch expr.Value {
		case AttributeTypeValueUnit:
			_, ok := c.(*component.Unit)
			return ok, nil
		case AttributeTypeValueStack:
			_, ok := c.(*component.Stack)
			return ok, nil
		}

		return false, nil

	case AttributeExternal:
		switch expr.Value {
		case AttributeExternalValueTrue:
			return c.External(), nil
		case AttributeExternalValueFalse:
			return !c.External(), nil
		}

		return false, nil

	case AttributeReading:
		// Reading attribute requires parsing, can't evaluate without parsed data
		return false, nil

	case AttributeSource:
		// Source attribute requires parsing, can't evaluate without parsed data
		return false, nil

	default:
		return false, nil
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
