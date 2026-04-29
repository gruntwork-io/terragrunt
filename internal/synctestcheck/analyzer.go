// Package synctestcheck provides a go/analysis pass that flags real-clock
// time primitives in *_test.go files that are not lexically nested inside
// a testing/synctest bubble. Inside a bubble, time.Sleep / time.After /
// time.Tick / time.NewTimer / time.NewTicker / time.AfterFunc advance the
// fake clock deterministically; outside one they introduce real wall-clock
// waits that make tests slow and flaky.
package synctestcheck

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Name is the analyzer's identifier, used both as the registered analyzer
// name and as the keyword recognized in `//nolint:` directives.
const Name = "synctestcheck"

// Analyzer is the synctestcheck analysis pass.
var Analyzer = &analysis.Analyzer{
	Name:     Name,
	Doc:      "flags real-clock time.* calls in tests that are not inside a testing/synctest bubble",
	URL:      "https://pkg.go.dev/testing/synctest",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

const (
	timePkg     = "time"
	synctestPkg = "testing/synctest"
)

// watched holds the time package functions that read or wait on the real
// monotonic clock. Calls to these inside a synctest bubble are redirected
// to the fake clock; calls outside one are the bug we're trying to catch.
var watched = map[string]bool{
	"Sleep":     true,
	"After":     true,
	"Tick":      true,
	"NewTimer":  true,
	"NewTicker": true,
	"AfterFunc": true,
}

func run(pass *analysis.Pass) (any, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	suppressed := suppressedLines(pass)

	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}

	insp.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return false
		}

		pos := pass.Fset.Position(n.Pos())
		if !strings.HasSuffix(pos.Filename, "_test.go") {
			return false
		}

		call := n.(*ast.CallExpr)

		name, ok := timeFuncName(call, pass.TypesInfo)
		if !ok {
			return true
		}

		if !watched[name] {
			return true
		}

		if insideSynctestBubble(stack, pass.TypesInfo) {
			return true
		}

		if suppressed[fileLine{pos.Filename, pos.Line}] {
			return true
		}

		pass.Reportf(call.Pos(),
			"prefer testing/synctest (synctest.Test) over real-clock time.%s in tests",
			name)

		return true
	})

	return nil, nil
}

// fileLine identifies a position by file and 1-indexed line number, which
// is the granularity at which suppression directives apply.
type fileLine struct {
	file string
	line int
}

// suppressedLines scans every comment in every file of the pass and returns
// the set of (file, line) pairs at which a report should be silenced. Two
// directive shapes are recognized, mirroring golangci-lint's nolint syntax:
//
//   - A `//nolint:synctestcheck` (or `//nolint:all`, or a comma-separated
//     list including `synctestcheck`) on the same source line as the
//     reportable call site silences that line.
//   - The same directive on its own immediately preceding line silences
//     the next line, allowing call sites that wouldn't fit a trailing
//     comment to suppress.
func suppressedLines(pass *analysis.Pass) map[fileLine]bool {
	suppressed := make(map[fileLine]bool)

	for _, f := range pass.Files {
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				if !directiveMatches(c.Text) {
					continue
				}

				pos := pass.Fset.Position(c.Slash)
				suppressed[fileLine{pos.Filename, pos.Line}] = true
				suppressed[fileLine{pos.Filename, pos.Line + 1}] = true
			}
		}
	}

	return suppressed
}

// directiveMatches reports whether comment text is a //nolint directive
// that names this analyzer (either explicitly or via the `all` shorthand).
func directiveMatches(text string) bool {
	const prefix = "//nolint:"

	if !strings.HasPrefix(text, prefix) {
		return false
	}

	// Strip the prefix and any trailing explanatory comment after a space.
	rest := strings.TrimPrefix(text, prefix)
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		rest = rest[:i]
	}

	for name := range strings.SplitSeq(rest, ",") {
		switch strings.TrimSpace(name) {
		case Name, "all":
			return true
		}
	}

	return false
}

// timeFuncName returns the function name if call is a selector expression
// of the form time.X, resolved via the type checker so aliased imports
// (e.g. `import t "time"`) and same-named identifiers from other packages
// don't produce false positives or negatives.
func timeFuncName(call *ast.CallExpr, info *types.Info) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}

	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", false
	}

	pkgName, ok := info.Uses[ident].(*types.PkgName)
	if !ok {
		return "", false
	}

	if pkgName.Imported().Path() != timePkg {
		return "", false
	}

	return sel.Sel.Name, true
}

// insideSynctestBubble reports whether any ancestor in stack is a call to
// testing/synctest.Test or testing/synctest.Run. The Go runtime treats
// goroutines started inside the closure passed to either function as
// members of the same bubble; lexical containment is the right
// approximation because goroutines spawned outside the closure are not
// in the bubble even at runtime.
func insideSynctestBubble(stack []ast.Node, info *types.Info) bool {
	for i := len(stack) - 2; i >= 0; i-- { //nolint:mnd
		call, ok := stack[i].(*ast.CallExpr)
		if !ok {
			continue
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok {
			continue
		}

		pkgName, ok := info.Uses[ident].(*types.PkgName)
		if !ok {
			continue
		}

		if pkgName.Imported().Path() != synctestPkg {
			continue
		}

		if sel.Sel.Name == "Test" {
			return true
		}
	}

	return false
}
