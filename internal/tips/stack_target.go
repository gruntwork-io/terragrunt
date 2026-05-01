package tips

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/filter"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// GiveStackTargetTip emits the StackTargetMissingTypeStack tip when one of the
// provided filters has a literal path expression pointing at a directory that
// contains terragrunt.stack.hcl, and the filter as a whole is not restricted
// to stacks via `| type=stack`.
//
// Glob path expressions are skipped so the suggested rewrite can be pasted
// verbatim.
func GiveStackTargetTip(l log.Logger, fs vfs.FS, workingDir string, filters filter.Filters, allTips Tips) {
	if len(filters) == 0 || allTips == nil {
		return
	}

	tip := allTips.Find(StackTargetMissingTypeStack)
	if tip == nil {
		return
	}

	var offenders []string

	for _, f := range filters {
		if f.Expression().IsRestrictedToStacks() {
			continue
		}

		if filterTargetsStackDir(f, fs, workingDir) {
			offenders = append(offenders, f.String())
		}
	}

	if len(offenders) == 0 {
		return
	}

	tip.Message = buildStackTargetMessage(offenders)
	tip.Evaluate(l)
}

func filterTargetsStackDir(f *filter.Filter, fs vfs.FS, workingDir string) bool {
	var hit bool

	filter.WalkExpressions(f.Expression(), func(e filter.Expression) bool {
		pe, ok := e.(*filter.PathExpression)
		if !ok {
			return true
		}

		if containsGlobMeta(pe.Value) {
			return true
		}

		candidate := pe.Value
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(workingDir, candidate)
		}

		stackFile := filepath.Join(candidate, config.DefaultStackFile)

		exists, err := vfs.FileExists(fs, stackFile)
		if err != nil || !exists {
			return true
		}

		hit = true

		return false
	})

	return hit
}

func buildStackTargetMessage(offenders []string) string {
	var b strings.Builder

	b.WriteString(StackTargetMissingTypeStackMessage)
	b.WriteString(" Offending filter(s):")

	for _, o := range offenders {
		fmt.Fprintf(&b, "\n  --filter %q -> --filter %q", o, SuggestStackTargetRewrite(o))
	}

	return b.String()
}

// SuggestStackTargetRewrite returns the suggested replacement for a filter
// query that targets a stack directory but is missing `| type=stack`.
//
// Tests pin the contract that the returned string parses to a filter whose
// Expression.IsRestrictedToStacks() is true. The contract holds because `|`
// is the only infix operator in the filter grammar and `!` binds tighter, so
// appending `| type=stack` always produces an InfixExpression whose right
// side is the stack-type attribute filter.
func SuggestStackTargetRewrite(originalQuery string) string {
	return originalQuery + " | type=stack"
}

func containsGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}
