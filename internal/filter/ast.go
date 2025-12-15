package filter

import (
	"path/filepath"
	"sync"

	"github.com/gobwas/glob"
)

// Expression is the interface that all AST nodes must implement.
type Expression interface {
	// expressionNode is a marker method to distinguish expression nodes.
	expressionNode()
	// String returns a string representation of the expression for debugging.
	String() string
	// RequiresDiscovery returns the first expression that requires discovery of Terragrunt components if any do.
	// Additionally, it returns a secondary value of true if any do.
	RequiresDiscovery() (Expression, bool)
	// RequiresParse returns the first expression that requires parsing Terragrunt HCL configurations if any do.
	// Additionally, it returns a secondary value of true if any do.
	RequiresParse() (Expression, bool)
	// IsRestrictedToStacks returns true if the expression is restricted to stacks.
	IsRestrictedToStacks() bool
}

// Expressions is a slice of expressions.
type Expressions []Expression

// PathExpression represents a path or glob filter (e.g., "./path/**/*" or "/absolute/path").
type PathExpression struct {
	compiledGlob glob.Glob
	compileErr   error
	Value        string
	compileOnce  sync.Once
}

// NewPathFilter creates a new PathFilter with lazy glob compilation.
func NewPathFilter(value string) *PathExpression {
	return &PathExpression{Value: value}
}

// CompileGlob returns the compiled glob pattern, compiling it on first call.
// Subsequent calls return the cached compiled glob and any error.
// Uses sync.Once for thread-safe lazy initialization.
func (p *PathExpression) CompileGlob() (glob.Glob, error) {
	p.compileOnce.Do(func() {
		pattern := p.Value
		pattern = filepath.Clean(filepath.ToSlash(pattern))
		p.compiledGlob, p.compileErr = glob.Compile(pattern, '/')
	})

	return p.compiledGlob, p.compileErr
}

func (p *PathExpression) expressionNode()                       {}
func (p *PathExpression) String() string                        { return p.Value }
func (p *PathExpression) RequiresDiscovery() (Expression, bool) { return p, false }
func (p *PathExpression) RequiresParse() (Expression, bool)     { return p, false }
func (p *PathExpression) IsRestrictedToStacks() bool            { return false }

// AttributeExpression represents a key-value attribute filter (e.g., "name=my-app").
type AttributeExpression struct {
	compiledGlob glob.Glob
	compileErr   error
	Key          string
	Value        string
	compileOnce  sync.Once
}

// NewAttributeExpression creates a new AttributeFilter with lazy glob compilation.
func NewAttributeExpression(key string, value string) *AttributeExpression {
	return &AttributeExpression{Key: key, Value: value}
}

// CompileGlob returns the compiled glob pattern for name and reading filters, compiling it on first call.
// Returns an error if called on unsupported attributes (e.g. type, external).
// Uses sync.Once for thread-safe lazy initialization.
func (a *AttributeExpression) CompileGlob() (glob.Glob, error) {
	// Only compile for attributes that support glob matching
	if !a.supportsGlob() {
		return nil, NewEvaluationError("attribute '" + a.Key + "' does not support glob patterns")
	}

	a.compileOnce.Do(func() {
		pattern := a.Value

		if a.Key == AttributeReading {
			pattern = filepath.Clean(filepath.ToSlash(pattern))
		}

		a.compiledGlob, a.compileErr = glob.Compile(pattern, '/')
	})

	return a.compiledGlob, a.compileErr
}

// supportsGlob returns true if the attribute filter supports glob patterns.
func (a *AttributeExpression) supportsGlob() bool {
	return a.Key == AttributeReading || a.Key == AttributeName || a.Key == AttributeSource
}

func (a *AttributeExpression) expressionNode()                       {}
func (a *AttributeExpression) String() string                        { return a.Key + "=" + a.Value }
func (a *AttributeExpression) RequiresDiscovery() (Expression, bool) { return a, true }
func (a *AttributeExpression) RequiresParse() (Expression, bool) {
	switch a.Key {
	// All of these attributes can be determined based on the component + configuration filepath.
	case AttributeName, AttributeType, AttributeExternal:
		return nil, false
	// We only know what a component reads if we parse it.
	case AttributeReading:
		return a, true
	// We default to true to be conservative in-case we forget to register
	// a new attribute here that does require parsing.
	default:
		return nil, true
	}
}
func (a *AttributeExpression) IsRestrictedToStacks() bool {
	return a.Key == "type" && a.Value == "stack"
}

// PrefixExpression represents a prefix operator expression (e.g., "!name=foo").
type PrefixExpression struct {
	Right    Expression
	Operator string
}

// NewPrefixExpression creates a new PrefixExpression.
func NewPrefixExpression(operator string, right Expression) *PrefixExpression {
	return &PrefixExpression{Operator: operator, Right: right}
}

func (p *PrefixExpression) expressionNode() {}
func (p *PrefixExpression) String() string  { return p.Operator + p.Right.String() }
func (p *PrefixExpression) RequiresDiscovery() (Expression, bool) {
	return p.Right.RequiresDiscovery()
}
func (p *PrefixExpression) RequiresParse() (Expression, bool) {
	return p.Right.RequiresParse()
}
func (p *PrefixExpression) IsRestrictedToStacks() bool {
	switch p.Operator {
	case "!":
		switch a := p.Right.(type) {
		case *AttributeExpression:
			switch a.Key {
			case "type":
				return a.Value != "stack"
			default:
				return false
			}
		default:
			return false
		}
	default:
		return false
	}
}

// InfixExpression represents an infix operator expression (e.g., "./apps/* | name=bar").
type InfixExpression struct {
	Left     Expression
	Right    Expression
	Operator string
}

// NewInfixExpression creates a new InfixExpression.
func NewInfixExpression(left Expression, operator string, right Expression) *InfixExpression {
	return &InfixExpression{Left: left, Operator: operator, Right: right}
}

func (i *InfixExpression) expressionNode() {}
func (i *InfixExpression) String() string {
	return i.Left.String() + " " + i.Operator + " " + i.Right.String()
}
func (i *InfixExpression) RequiresDiscovery() (Expression, bool) {
	if _, ok := i.Left.RequiresDiscovery(); ok {
		return i, true
	}

	if _, ok := i.Right.RequiresDiscovery(); ok {
		return i, true
	}

	return nil, false
}
func (i *InfixExpression) RequiresParse() (Expression, bool) {
	if _, ok := i.Left.RequiresParse(); ok {
		return i, true
	}

	if _, ok := i.Right.RequiresParse(); ok {
		return i, true
	}

	return nil, false
}
func (i *InfixExpression) IsRestrictedToStacks() bool {
	switch i.Operator {
	case "|":
		return i.Left.IsRestrictedToStacks() || i.Right.IsRestrictedToStacks()
	default:
		return false
	}
}

// GraphExpression represents a graph traversal expression (e.g., "...foo", "foo...", "...foo...", "^foo").
type GraphExpression struct {
	Target              Expression
	IncludeDependents   bool
	IncludeDependencies bool
	ExcludeTarget       bool
}

// NewGraphExpression creates a new GraphExpression.
func NewGraphExpression(
	target Expression,
	includeDependents bool,
	includeDependencies bool,
	excludeTarget bool,
) *GraphExpression {
	return &GraphExpression{
		Target:              target,
		IncludeDependents:   includeDependents,
		IncludeDependencies: includeDependencies,
		ExcludeTarget:       excludeTarget,
	}
}

func (g *GraphExpression) expressionNode() {}
func (g *GraphExpression) String() string {
	result := ""
	if g.IncludeDependents {
		result += "..."
	}

	if g.ExcludeTarget {
		result += "^"
	}

	result += g.Target.String()
	if g.IncludeDependencies {
		result += "..."
	}

	return result
}
func (g *GraphExpression) RequiresDiscovery() (Expression, bool) {
	// Graph expressions require dependency discovery to traverse the graph
	return g, true
}
func (g *GraphExpression) RequiresParse() (Expression, bool) {
	// Graph expressions require parsing to traverse the graph.
	return g, true
}
func (g *GraphExpression) IsRestrictedToStacks() bool { return false }

// GitExpression represents a Git-based filter expression (e.g., "[main...HEAD]" or "[main]").
// It filters components based on changes between Git references.
type GitExpression struct {
	FromRef string
	ToRef   string
}

func NewGitExpression(fromRef, toRef string) *GitExpression {
	return &GitExpression{FromRef: fromRef, ToRef: toRef}
}

func (g *GitExpression) expressionNode() {}
func (g *GitExpression) String() string {
	return "[" + g.FromRef + "..." + g.ToRef + "]"
}
func (g *GitExpression) RequiresDiscovery() (Expression, bool) {
	// Git filters require discovery to check which components changed between references
	return g, true
}
func (g *GitExpression) RequiresParse() (Expression, bool) {
	// Git filters don't require parsing - they compare file paths, not HCL content
	return nil, false
}
func (g *GitExpression) IsRestrictedToStacks() bool { return false }

// GitExpressions is a slice of Git expressions.
type GitExpressions []*GitExpression

// UniqueGitRefs returns all unique Git references in a slice of expressions.
func (e GitExpressions) UniqueGitRefs() []string {
	refSet := make(map[string]struct{}, len(e))

	for _, expr := range e {
		refs := collectGitReferences(expr)
		for _, ref := range refs {
			refSet[ref] = struct{}{}
		}
	}

	result := make([]string, 0, len(refSet))
	for ref := range refSet {
		result = append(result, ref)
	}

	return result
}
