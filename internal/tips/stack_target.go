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

// GiveStackTargetTip emits the StackTargetMissingTypeStack tip when a filter
// has a literal path expression pointing at a directory that contains
// terragrunt.stack.hcl.
//
// Filters that already mention a `type=` attribute are skipped, since the
// user has chosen explicitly. Glob path expressions are skipped so the
// suggested rewrite can be pasted verbatim.
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
		if filterMentionsTypeAttribute(f) {
			continue
		}

		if filterTargetsStackDir(l, f, fs, workingDir) {
			offenders = append(offenders, f.String())
		}
	}

	if len(offenders) == 0 {
		return
	}

	tip.EvaluateWith(l, buildStackTargetMessage(offenders))
}

// SuggestStackTargetRewrite returns the suggested replacement for a filter
// query that targets a stack directory but is missing `| type=stack`. It is
// exported so the contract test can pin its behavior from a `tips_test`
// black-box test.
//
// The contract holds because `|` is the only infix operator in the filter
// grammar and `!` binds tighter, so appending `| type=stack` always produces
// an InfixExpression whose right side is the stack-type attribute filter.
func SuggestStackTargetRewrite(originalQuery string) string {
	return originalQuery + " | type=stack"
}

// filterMentionsTypeAttribute reports whether the filter contains a
// `type=` attribute (e.g. `type=stack`, `!type=stack`, `type=unit`).
func filterMentionsTypeAttribute(f *filter.Filter) bool {
	var found bool

	filter.WalkExpressions(f.Expression(), func(e filter.Expression) bool {
		attr, ok := e.(*filter.AttributeExpression)
		if !ok {
			return true
		}

		if attr.Key == filter.AttributeType {
			found = true
			return false
		}

		return true
	})

	return found
}

func filterTargetsStackDir(l log.Logger, f *filter.Filter, fs vfs.FS, workingDir string) bool {
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
		if err != nil {
			l.Debugf("stack-target tip: skipping %q: %v", stackFile, err)
			return true
		}

		if !exists {
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

// containsGlobMeta reports whether s contains any glob metacharacter that can
// appear in a parsed PathExpression.Value. The filter lexer emits '[', '{',
// '}', and ']' as their own tokens, so the only reachable meta chars here are
// '*', '?', and '\\'.
func containsGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?\\")
}
