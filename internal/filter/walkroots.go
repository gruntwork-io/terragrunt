package filter

import (
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/glob"
)

// globMetaChars are the characters that make a glob pattern match more than
// the literal string it spells.
const globMetaChars = `*?[{\`

// WalkRoots returns the roots below workingDir that hold every component the
// filters can select, with reasons for debug logs and telemetry. Roots are
// '/'-separated and relative to workingDir. Every component outside them is
// one [Classifier.Classify] marks [StatusExcluded].
//
// Only positive path filters take roots. A git filter takes none, because it
// selects only components found inside its worktrees. Any attribute, parse,
// or graph filter can select a component anywhere, and without a positive
// filter every component is included by default, so either one yields the
// single recursive root ".", the whole working directory.
func (c *Classifier) WalkRoots(workingDir string) ([]glob.Root, []string) {
	if reasons := c.fullWalkReasons(); len(reasons) > 0 {
		return walkEverything(), reasons
	}

	wd := path.Clean(filepath.ToSlash(workingDir))

	if strings.ContainsAny(wd, globMetaChars) {
		return walkEverything(), []string{fmt.Sprintf("working directory %s holds glob metacharacters", workingDir)}
	}

	var (
		roots   []glob.Root
		reasons []string
	)

	for _, expr := range c.pathExprs {
		exprRoots, fullReason := pathExprRoots(expr, wd)
		if fullReason != "" {
			return walkEverything(), []string{fullReason}
		}

		roots = append(roots, exprRoots...)
		reasons = append(reasons, fmt.Sprintf("path filter %s resolves to %d root(s)", expr.Value, len(exprRoots)))
	}

	for _, expr := range c.gitExprs {
		reasons = append(reasons, fmt.Sprintf("git filter %s matches only worktree components", expr))
	}

	return collapseRoots(roots), reasons
}

// fullWalkReasons lists the filters that can select a component anywhere in
// the working directory.
func (c *Classifier) fullWalkReasons() []string {
	var reasons []string

	if !c.hasPositiveFilters {
		reasons = append(reasons, "no positive filter, so every component is included by default")
	}

	for _, expr := range c.attributeExprs {
		reasons = append(reasons, fmt.Sprintf("attribute filter %s can match anywhere", expr))
	}

	for _, expr := range c.parseExprs {
		reasons = append(reasons, fmt.Sprintf("filter %s needs a parse to evaluate", expr))
	}

	for _, info := range c.graphExprs {
		reasons = append(reasons, fmt.Sprintf("graph filter %s follows dependency edges", info.FullExpression))
	}

	return reasons
}

// pathExprRoots returns the roots of expr relative to wd, dropping roots no
// walked component can match. A non-empty reason means the filter can match
// anywhere below wd.
func pathExprRoots(expr *PathExpression, wd string) ([]glob.Root, string) {
	pattern := path.Clean(filepath.ToSlash(expr.Value))
	absolute := filepath.IsAbs(expr.Value)

	var roots []glob.Root

	for _, root := range glob.Roots(pattern) {
		dir := root.Dir

		if absolute {
			rel, fit := relativeTo(wd, root)

			switch fit {
			case rootOutside:
				continue
			case rootCoversAll:
				return nil, fmt.Sprintf("path filter %s can match anywhere below the working directory", expr.Value)
			case rootUnknown:
				return nil, fmt.Sprintf("path filter %s has wildcards above the working directory", expr.Value)
			case rootInside:
				dir = rel
			}
		}

		if !canHoldComponentPath(dir) {
			continue
		}

		if dir == "." && root.Recursive {
			return nil, fmt.Sprintf("path filter %s can match anywhere below the working directory", expr.Value)
		}

		roots = append(roots, glob.Root{Dir: dir, Recursive: root.Recursive})
	}

	return roots, ""
}

// canHoldComponentPath reports whether dir, relative to the working directory,
// has the form a component's path takes there: clean, relative, and at or
// below the working directory.
func canHoldComponentPath(dir string) bool {
	return dir == path.Clean(dir) && !path.IsAbs(dir) && dir != ".." && !strings.HasPrefix(dir, "../")
}

// rootFit says where an absolute root sits against the working directory.
type rootFit int

const (
	// rootInside is at or below the working directory.
	rootInside rootFit = iota
	// rootOutside matches nothing at or below the working directory.
	rootOutside
	// rootCoversAll is a recursive root above the working directory.
	rootCoversAll
	// rootUnknown has wildcards where it would have to be compared with the
	// working directory.
	rootUnknown
)

// relativeTo places the absolute root against wd, returning its Dir relative
// to wd when it sits inside.
func relativeTo(wd string, root glob.Root) (string, rootFit) {
	dir := root.Dir

	if dir == wd {
		return ".", rootInside
	}

	if rel, ok := strings.CutPrefix(dir, strings.TrimSuffix(wd, "/")+"/"); ok {
		return rel, rootInside
	}

	if dir == "/" || strings.HasPrefix(wd, dir+"/") {
		if root.Recursive {
			return "", rootCoversAll
		}

		return "", rootOutside
	}

	if strings.ContainsAny(dir, globMetaChars) {
		return "", rootUnknown
	}

	return "", rootOutside
}

// collapseRoots sorts and deduplicates roots, dropping any root that sits at
// or below a recursive root with a literal Dir.
func collapseRoots(roots []glob.Root) []glob.Root {
	slices.SortFunc(roots, func(a, b glob.Root) int {
		return strings.Compare(a.Dir, b.Dir)
	})

	roots = slices.Compact(roots)

	collapsed := make([]glob.Root, 0, len(roots))

	for _, r := range roots {
		covered := slices.ContainsFunc(roots, func(q glob.Root) bool {
			return q != r && coversRoot(q, r)
		})
		if !covered {
			collapsed = append(collapsed, r)
		}
	}

	return collapsed
}

// coversRoot reports whether walking outer reaches everything inner can match:
// outer is recursive, its Dir is literal, and inner's Dir is at or below it.
func coversRoot(outer, inner glob.Root) bool {
	return outer.Recursive && !strings.ContainsAny(outer.Dir, globMetaChars) &&
		(inner.Dir == outer.Dir || strings.HasPrefix(inner.Dir, outer.Dir+"/"))
}

// walkEverything returns the single recursive root that covers the whole
// working directory.
func walkEverything() []glob.Root {
	return []glob.Root{{Dir: ".", Recursive: true}}
}
