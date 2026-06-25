package tips

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/glob"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveStackNestedGenerateTip emits the StackNestedStacksNotGenerated tip after
// generation when a literal (non-glob) path with `| type=stack` generated a stack
// whose generated directory still contains nested stacks that were not generated.
// The user likely expected the whole subtree, so the tip shows how to include it.
func GiveStackNestedGenerateTip(l log.Logger, fs vfs.FS, workingDir string, filters filter.Filters, allTips Tips) {
	if len(filters) == 0 || allTips == nil {
		return
	}

	tip := allTips.Find(StackNestedStacksNotGenerated)
	if tip == nil {
		return
	}

	suggestions := make([]string, 0)

	for _, f := range filters.RestrictToStacks() {
		path := literalStackFilterPath(f)
		if path == "" {
			continue
		}

		dir := path
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(workingDir, dir)
		}

		if !hasUngeneratedNestedStacks(fs, dir) {
			continue
		}

		suggestions = append(suggestions, SuggestRecursiveStackFilter(path))
	}

	if len(suggestions) == 0 {
		return
	}

	tip.EvaluateWith(l, buildStackNestedGenerateMessage(suggestions))
}

// SuggestRecursiveStackFilter returns a recursive stack filter that also selects
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

// hasUngeneratedNestedStacks reports whether dir's generated tree contains a
// nested stack file that has not itself been expanded (its own .terragrunt-stack
// directory is missing). That is exactly what a non-glob `| type=stack` filter
// leaves behind: the nested stack file is copied in, but never generated.
func hasUngeneratedNestedStacks(fs vfs.FS, dir string) bool {
	pattern := filepath.ToSlash(filepath.Join(dir, config.StackDir, "**", config.DefaultStackFile))

	matches, err := glob.Expand(fs, pattern, glob.WithFilesOnly())
	if err != nil {
		return false
	}

	for _, m := range matches {
		expansion := filepath.Join(filepath.Dir(m), config.StackDir)

		exists, err := vfs.FileExists(fs, expansion)
		if err == nil && !exists {
			return true
		}
	}

	return false
}

func buildStackNestedGenerateMessage(suggestions []string) string {
	var b strings.Builder

	b.WriteString(StackNestedStacksNotGeneratedMessage)
	b.WriteString(" For example:")

	for _, s := range suggestions {
		fmt.Fprintf(&b, "\n  --filter %q", s)
	}

	return b.String()
}
