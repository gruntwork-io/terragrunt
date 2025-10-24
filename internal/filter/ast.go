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
}

// PathFilter represents a path or glob filter (e.g., "./path/**/*" or "/absolute/path").
type PathFilter struct {
	compiledGlob glob.Glob
	compileErr   error
	Value        string
	WorkingDir   string
	compileOnce  sync.Once
}

// NewPathFilter creates a new PathFilter with lazy glob compilation.
func NewPathFilter(value string, workingDir string) *PathFilter {
	return &PathFilter{Value: value, WorkingDir: workingDir}
}

// CompileGlob returns the compiled glob pattern, compiling it on first call.
// Subsequent calls return the cached compiled glob and any error.
// Uses sync.Once for thread-safe lazy initialization.
func (p *PathFilter) CompileGlob() (glob.Glob, error) {
	p.compileOnce.Do(func() {
		pattern := p.Value
		if !filepath.IsAbs(pattern) {
			pattern = filepath.Join(p.WorkingDir, pattern)
		}

		pattern = filepath.ToSlash(pattern)
		p.compiledGlob, p.compileErr = glob.Compile(pattern, '/')
	})

	return p.compiledGlob, p.compileErr
}

func (p *PathFilter) expressionNode() {}
func (p *PathFilter) String() string  { return p.Value }

// AttributeFilter represents a key-value attribute filter (e.g., "name=my-app").
type AttributeFilter struct {
	compiledGlob glob.Glob
	compileErr   error
	Key          string
	Value        string
	WorkingDir   string
	compileOnce  sync.Once
}

// CompileGlob returns the compiled glob pattern for name and reading filters, compiling it on first call.
// Returns an error if called on unsupported attributes (e.g. type, external).
// Uses sync.Once for thread-safe lazy initialization.
func (a *AttributeFilter) CompileGlob() (glob.Glob, error) {
	// Only compile for attributes that support glob matching
	if !a.supportsGlob() {
		return nil, NewEvaluationError("attribute '" + a.Key + "' does not support glob patterns")
	}

	a.compileOnce.Do(func() {
		pattern := a.Value

		if a.Key == AttributeReading {
			if !filepath.IsAbs(pattern) {
				pattern = filepath.Join(a.WorkingDir, pattern)
			}

			pattern = filepath.ToSlash(pattern)
		}

		a.compiledGlob, a.compileErr = glob.Compile(pattern, '/')
	})

	return a.compiledGlob, a.compileErr
}

// supportsGlob returns true if the attribute filter supports glob patterns.
func (a *AttributeFilter) supportsGlob() bool {
	return a.Key == AttributeReading || a.Key == AttributeName
}

func (a *AttributeFilter) expressionNode() {}
func (a *AttributeFilter) String() string  { return a.Key + "=" + a.Value }

// PrefixExpression represents a prefix operator expression (e.g., "!name=foo").
type PrefixExpression struct {
	Right    Expression
	Operator string
}

func (p *PrefixExpression) expressionNode() {}
func (p *PrefixExpression) String() string  { return p.Operator + p.Right.String() }

// InfixExpression represents an infix operator expression (e.g., "./apps/* | name=bar").
type InfixExpression struct {
	Left     Expression
	Right    Expression
	Operator string
}

func (i *InfixExpression) expressionNode() {}
func (i *InfixExpression) String() string {
	return i.Left.String() + " " + i.Operator + " " + i.Right.String()
}
