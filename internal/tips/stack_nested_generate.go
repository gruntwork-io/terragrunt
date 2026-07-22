package tips

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	inthclparse "github.com/gruntwork-io/terragrunt/internal/hclparse"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveStackNestedGenerateTip emits the StackNestedStacksNotGenerated tip after
// generation when a literal (non-glob) path with `| type=stack` targets a stack
// whose nested stacks were not themselves recursively generated.
// The user likely expected the whole subtree, so the tip shows how to include it.
func GiveStackNestedGenerateTip(
	l log.Logger,
	fs vfs.FS,
	funcsFor inthclparse.StackFuncFactory,
	workingDir string,
	filters filter.Filters,
	allTips Tips,
) {
	if len(filters) == 0 || allTips == nil || funcsFor == nil {
		return
	}

	tip := allTips.Find(StackNestedStacksNotGenerated)
	if tip == nil {
		return
	}

	var paths []string

	for _, f := range filters.RestrictToStacks() {
		path := literalStackFilterPath(f)
		if path == "" {
			continue
		}

		dir := path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(workingDir, dir)
		}

		if !stackHasUngeneratedNestedStacks(l, fs, funcsFor, dir) {
			continue
		}

		paths = append(paths, path)
	}

	if len(paths) == 0 {
		return
	}

	tip.EvaluateWith(l, buildStackNestedGenerateMessage(paths))
}

// SuggestRecursiveStackFilter returns the recursive stack filter that selects
// the nested stacks beneath path.
func SuggestRecursiveStackFilter(path string) string {
	return path + "/** | type=stack"
}

// literalStackFilterPath returns the first literal (non-glob) path targeted by
// the filter, or "" if it has none.
func literalStackFilterPath(f *filter.Filter) string {
	var found string

	filter.WalkExpressions(f.Expression(), func(e filter.Expression) bool {
		pe, ok := e.(*filter.PathExpression)
		if !ok {
			return true
		}

		if containsGlobMeta(pe.Value) {
			return true
		}

		found = pe.Value

		return false
	})

	return found
}

// stackHasUngeneratedNestedStacks reports whether the stack generated at dir has
// nested stacks that were not themselves recursively generated. For each nested
// stack the parent generated, it checks whether that nested stack's own components
// exist on disk (honoring no_dot_terragrunt_stack).
func stackHasUngeneratedNestedStacks(
	l log.Logger,
	fs vfs.FS,
	funcsFor inthclparse.StackFuncFactory,
	dir string,
) bool {
	_, nestedStackDirs, err := inthclparse.DirectComponentPaths(fs, dir, funcsFor)
	if err != nil {
		l.Debugf("stack-nested-generate tip: skipping %q: %v", dir, err)
		return false
	}

	for _, nestedDir := range nestedStackDirs {
		if !nestedStackGenerated(l, fs, funcsFor, nestedDir) {
			return true
		}
	}

	return false
}

// nestedStackGenerated reports whether every direct component of the nested stack
// generated at nestedDir exists on disk, i.e. the nested stack was itself generated.
func nestedStackGenerated(
	l log.Logger,
	fs vfs.FS,
	funcsFor inthclparse.StackFuncFactory,
	nestedDir string,
) bool {
	unitPaths, stackPaths, err := inthclparse.DirectComponentPaths(fs, nestedDir, funcsFor)
	if err != nil {
		l.Debugf("stack-nested-generate tip: skipping %q: %v", nestedDir, err)
		return true
	}

	return !anyPathMissing(l, fs, unitPaths) && !anyPathMissing(l, fs, stackPaths)
}

// anyPathMissing reports whether any of paths does not exist on fs.
func anyPathMissing(l log.Logger, fs vfs.FS, paths []string) bool {
	for _, p := range paths {
		exists, err := vfs.FileExists(fs, p)
		if err != nil {
			l.Debugf("stack-nested-generate tip: cannot stat %q: %v", p, err)
			continue
		}

		if !exists {
			return true
		}
	}

	return false
}

func buildStackNestedGenerateMessage(paths []string) string {
	var b strings.Builder

	b.WriteString(StackNestedStacksNotGeneratedMessage)
	b.WriteString(" For example:")

	for _, p := range paths {
		fmt.Fprintf(
			&b,
			"\n  --filter %q --filter %q",
			p+" | type=stack",
			SuggestRecursiveStackFilter(p),
		)
	}

	return b.String()
}
