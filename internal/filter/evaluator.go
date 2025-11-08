package filter

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/component"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

const (
	AttributeName     = "name"
	AttributeType     = "type"
	AttributeExternal = "external"
	AttributeReading  = "reading"
	AttributeSource   = "source"

	AttributeTypeValueUnit  = string(component.UnitKind)
	AttributeTypeValueStack = string(component.StackKind)

	AttributeExternalValueTrue  = "true"
	AttributeExternalValueFalse = "false"

	// MaxTraversalDepth is the maximum depth to traverse the graph for both dependencies and dependents.
	MaxTraversalDepth = 1000000
)

// EvaluationContext provides additional context for filter evaluation, such as Git worktree directories.
type EvaluationContext struct {
	// GitWorktrees maps Git references to temporary worktree directory paths.
	// This is used by GitFilter expressions to access different Git references.
	GitWorktrees map[string]string
	// WorkingDir is the base working directory for resolving relative paths.
	WorkingDir string
}

// Evaluate evaluates an expression against a list of components and returns the filtered components.
// If logger is provided, it will be used for logging warnings during evaluation.
func Evaluate(l log.Logger, expr Expression, components component.Components) (component.Components, error) {
	return EvaluateWithContext(l, expr, components, nil)
}

// EvaluateWithContext evaluates an expression against a list of components with additional context.
// If logger is provided, it will be used for logging warnings during evaluation.
func EvaluateWithContext(l log.Logger, expr Expression, components component.Components, ctx *EvaluationContext) (component.Components, error) {
	if expr == nil {
		return nil, NewEvaluationError("expression is nil")
	}

	return evaluate(l, expr, components, ctx)
}

// evaluate is the internal recursive evaluation function.
func evaluate(l log.Logger, expr Expression, components component.Components, ctx *EvaluationContext) (component.Components, error) {
	switch node := expr.(type) {
	case *PathFilter:
		return evaluatePathFilter(node, components)
	case *AttributeFilter:
		return evaluateAttributeFilter(node, components)
	case *PrefixExpression:
		return evaluatePrefixExpression(l, node, components, ctx)
	case *InfixExpression:
		return evaluateInfixExpression(l, node, components, ctx)
	case *GraphExpression:
		return evaluateGraphExpression(l, node, components, ctx)
	case *GitFilter:
		return evaluateGitFilter(l, node, components, ctx)
	default:
		return nil, NewEvaluationError("unknown expression type")
	}
}

// evaluatePathFilter evaluates a path filter using glob matching.
func evaluatePathFilter(filter *PathFilter, components component.Components) (component.Components, error) {
	g, err := filter.CompileGlob()
	if err != nil {
		return nil, NewEvaluationErrorWithCause("failed to compile glob pattern: "+filter.Value, err)
	}

	var result component.Components

	for _, component := range components {
		normalizedPath := component.Path()
		if !filepath.IsAbs(normalizedPath) {
			normalizedPath = filepath.Join(filter.WorkingDir, normalizedPath)
		}

		normalizedPath = filepath.ToSlash(normalizedPath)

		if g.Match(normalizedPath) {
			result = append(result, component)
		}
	}

	return result, nil
}

// evaluateAttributeFilter evaluates an attribute filter.
func evaluateAttributeFilter(filter *AttributeFilter, components []component.Component) ([]component.Component, error) {
	var result []component.Component

	switch filter.Key {
	case AttributeName:
		g, err := filter.CompileGlob()
		if err != nil {
			return nil, NewEvaluationErrorWithCause("failed to compile glob pattern for name filter: "+filter.Value, err)
		}

		for _, c := range components {
			if g.Match(filepath.Base(c.Path())) {
				result = append(result, c)
			}
		}

	case AttributeType:
		switch filter.Value {
		case AttributeTypeValueUnit:
			for _, c := range components {
				if _, ok := c.(*component.Unit); ok {
					result = append(result, c)
				}
			}
		case AttributeTypeValueStack:
			for _, c := range components {
				if _, ok := c.(*component.Stack); ok {
					result = append(result, c)
				}
			}
		default:
			return nil, NewEvaluationError("invalid type value: " + filter.Value + " (expected 'unit' or 'stack')")
		}
	case AttributeExternal:
		switch filter.Value {
		case AttributeExternalValueTrue:
			for _, c := range components {
				if c.External() {
					result = append(result, c)
				}
			}
		case AttributeExternalValueFalse:
			for _, c := range components {
				if !c.External() {
					result = append(result, c)
				}
			}
		default:
			return nil, NewEvaluationError("invalid external value: " + filter.Value + " (expected 'true' or 'false')")
		}
	case AttributeReading:
		g, err := filter.CompileGlob()
		if err != nil {
			return nil, NewEvaluationErrorWithCause("failed to compile glob pattern for reading filter: "+filter.Value, err)
		}

		for _, c := range components {
			for _, readFile := range c.Reading() {
				normalizedPath := readFile
				if !filepath.IsAbs(normalizedPath) {
					normalizedPath = filepath.Join(filter.WorkingDir, normalizedPath)
				}

				normalizedPath = filepath.ToSlash(normalizedPath)

				if g.Match(normalizedPath) {
					result = append(result, c)
					break
				}
			}
		}
	case AttributeSource:
		g, err := filter.CompileGlob()
		if err != nil {
			return nil, NewEvaluationErrorWithCause("failed to compile glob pattern for source filter: "+filter.Value, err)
		}

		for _, c := range components {
			if slices.ContainsFunc(c.Sources(), g.Match) {
				result = append(result, c)
			}
		}
	default:
		return nil, NewEvaluationError("unknown attribute key: " + filter.Key)
	}

	return result, nil
}

// evaluatePrefixExpression evaluates a prefix expression (negation).
func evaluatePrefixExpression(l log.Logger, expr *PrefixExpression, components component.Components, ctx *EvaluationContext) (component.Components, error) {
	if expr.Operator != "!" {
		return nil, NewEvaluationError("unknown prefix operator: " + expr.Operator)
	}

	toExclude, err := evaluate(l, expr.Right, components, ctx)
	if err != nil {
		return nil, err
	}

	excludeSet := make(map[string]struct{}, len(toExclude))
	for _, c := range toExclude {
		excludeSet[c.Path()] = struct{}{}
	}

	var result component.Components

	for _, c := range components {
		if _, ok := excludeSet[c.Path()]; !ok {
			result = append(result, c)
		}
	}

	return result, nil
}

// evaluateInfixExpression evaluates an infix expression (intersection).
func evaluateInfixExpression(l log.Logger, expr *InfixExpression, components component.Components, ctx *EvaluationContext) (component.Components, error) {
	if expr.Operator != "|" {
		return nil, NewEvaluationError("unknown infix operator: " + expr.Operator)
	}

	leftResult, err := evaluate(l, expr.Left, components, ctx)
	if err != nil {
		return nil, err
	}

	rightResult, err := evaluate(l, expr.Right, leftResult, ctx)
	if err != nil {
		return nil, err
	}

	return rightResult, nil
}

// evaluateGraphExpression evaluates a graph expression by traversing dependency/dependent graphs.
func evaluateGraphExpression(l log.Logger, expr *GraphExpression, components component.Components, ctx *EvaluationContext) (component.Components, error) {
	targetMatches, err := evaluate(l, expr.Target, components, ctx)
	if err != nil {
		return nil, err
	}

	if len(targetMatches) == 0 {
		return component.Components{}, nil
	}

	resultSet := make(map[string]component.Component)

	if !expr.ExcludeTarget {
		for _, c := range targetMatches {
			resultSet[c.Path()] = c
		}
	}

	visited := make(map[string]bool)

	if expr.IncludeDependencies {
		for _, target := range targetMatches {
			traverseDependencies(l, target, resultSet, visited, MaxTraversalDepth)
		}
	}

	visited = make(map[string]bool)

	if expr.IncludeDependents {
		for _, target := range targetMatches {
			traverseDependents(l, target, resultSet, visited, MaxTraversalDepth)
		}
	}

	result := make(component.Components, 0, len(resultSet))
	for _, c := range resultSet {
		result = append(result, c)
	}

	return result, nil
}

// evaluateGitFilter evaluates a Git filter expression by comparing components between Git references.
// It returns components that were added, removed, or changed between FromRef and ToRef.
func evaluateGitFilter(_ log.Logger, filter *GitFilter, components component.Components, ctx *EvaluationContext) (component.Components, error) {
	if ctx == nil || ctx.WorkingDir == "" {
		return nil, NewEvaluationError("Git filter requires evaluation context with working directory")
	}

	// Determine the "to" reference - use HEAD (current working directory) if ToRef is empty
	var (
		toRef         string
		useCurrentDir bool
	)

	if filter.ToRef == "" {
		useCurrentDir = true
		toRef = "HEAD"
	} else {
		useCurrentDir = false
		toRef = filter.ToRef
	}

	// Get changed files using git diff
	changedFiles, err := getChangedFilesBetweenRefs(filter.FromRef, toRef, useCurrentDir, ctx.WorkingDir)
	if err != nil {
		return nil, NewEvaluationErrorWithCause("failed to get changed files between Git references", err)
	}

	if len(changedFiles) == 0 {
		return component.Components{}, nil
	}

	// Create a set of changed file paths for fast lookup
	// git diff returns paths relative to the repository root
	changedSet := make(map[string]struct{}, len(changedFiles))
	for _, file := range changedFiles {
		// Normalize paths for comparison (relative to repo root)
		normalized := filepath.ToSlash(filepath.Clean(file))
		changedSet[normalized] = struct{}{}
	}

	// Filter components based on changed files
	var result component.Components
	for _, comp := range components {
		compPath := comp.Path()

		// Normalize component path for comparison
		normalizedCompPath := compPath
		if !filepath.IsAbs(normalizedCompPath) && ctx.WorkingDir != "" {
			normalizedCompPath = filepath.Join(ctx.WorkingDir, normalizedCompPath)
		}
		normalizedCompPath = filepath.ToSlash(filepath.Clean(normalizedCompPath))

		// Check if the component's directory or any of its config files are in the changed set
		if isComponentChanged(compPath, normalizedCompPath, changedSet, ctx.WorkingDir) {
			result = append(result, comp)
		}
	}

	return result, nil
}

// getChangedFilesBetweenRefs returns a list of files that were added, removed, or changed between two Git references.
// It uses git diff to compare the references. The workingDir should be the repository root.
// fromRef is the starting Git reference, toRef is the ending Git reference.
// If useCurrentDir is true, toRef is compared against the current working directory (HEAD).
// Otherwise, both references are used directly in git diff.
func getChangedFilesBetweenRefs(fromRef, toRef string, useCurrentDir bool, workingDir string) ([]string, error) {
	var cmd *exec.Cmd

	if useCurrentDir {
		// Compare reference to current working directory (HEAD)
		cmd = exec.Command("git", "diff", "--name-only", "--diff-filter=ACDMR", fromRef, "HEAD")
	} else {
		// Compare two Git references directly
		cmd = exec.Command("git", "diff", "--name-only", "--diff-filter=ACDMR", fromRef, toRef)
	}

	cmd.Dir = workingDir
	output, err := cmd.Output()
	if err != nil {
		// If the command fails, return the error
		return nil, err
	}

	// Parse output into file list
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}

	return files, nil
}

// isComponentChanged checks if a component is affected by the changed files.
// A component is considered changed if:
// 1. Its directory path matches a changed file's directory
// 2. Any of its configuration files (terragrunt.hcl, terragrunt.stack.hcl) are in the changed files
func isComponentChanged(compPath, normalizedCompPath string, changedSet map[string]struct{}, workingDir string) bool {
	// Get relative path of component from working directory for comparison with git diff output
	var relCompPath string
	if filepath.IsAbs(normalizedCompPath) && workingDir != "" {
		relPath, err := filepath.Rel(workingDir, normalizedCompPath)
		if err == nil {
			relCompPath = filepath.ToSlash(relPath)
		}
	} else {
		relCompPath = filepath.ToSlash(normalizedCompPath)
	}

	if relCompPath == "" {
		return false
	}

	// Normalize relCompPath to ensure it doesn't have trailing slashes
	relCompPath = strings.TrimSuffix(relCompPath, "/")

	// Check each changed file (git diff returns relative paths from repo root)
	for changedFile := range changedSet {
		// Normalize the changed file path
		changedFile = filepath.ToSlash(filepath.Clean(changedFile))

		// Check if the changed file is in the component's directory
		// This is the primary check: if any file in the component directory changed
		if strings.HasPrefix(changedFile, relCompPath+"/") {
			return true
		}

		// Check if the changed file IS the component directory itself (directory was added/removed)
		if changedFile == relCompPath {
			return true
		}
	}

	return false
}

// traverseDependencies recursively traverses the dependency graph downward (from a component to its dependencies).
func traverseDependencies(
	l log.Logger,
	c component.Component,
	resultSet map[string]component.Component,
	visited map[string]bool,
	maxDepth int,
) {
	if maxDepth <= 0 {
		if l != nil {
			l.Warnf(
				"Maximum dependency traversal depth (%d) reached for component %s during filtering. Some dependencies may have been excluded from results.",
				MaxTraversalDepth,
				c.Path(),
			)
		}

		return
	}

	path := c.Path()
	if visited[path] {
		return
	}

	visited[path] = true

	for _, dep := range c.Dependencies() {
		depPath := dep.Path()
		resultSet[depPath] = dep

		traverseDependencies(l, dep, resultSet, visited, maxDepth-1)
	}
}

// traverseDependents recursively traverses the dependent graph upward (from a component to its dependents).
func traverseDependents(
	l log.Logger,
	c component.Component,
	resultSet map[string]component.Component,
	visited map[string]bool,
	maxDepth int,
) {
	if maxDepth <= 0 {
		if l != nil {
			l.Warnf(
				"Maximum dependent traversal depth (%d) reached for component %s during filtering. Some dependents may have been excluded from results.",
				MaxTraversalDepth,
				c.Path(),
			)
		}

		return
	}

	path := c.Path()
	if visited[path] {
		return
	}

	visited[path] = true

	for _, dependent := range c.Dependents() {
		depPath := dependent.Path()
		resultSet[depPath] = dependent

		traverseDependents(l, dependent, resultSet, visited, maxDepth-1)
	}
}
