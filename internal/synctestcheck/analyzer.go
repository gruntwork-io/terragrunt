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

// Analyzer is the synctestcheck analysis pass.
var Analyzer = &analysis.Analyzer{
	Name:     "synctestcheck",
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

		pass.Reportf(call.Pos(),
			"prefer testing/synctest (synctest.Test) over real-clock time.%s in tests",
			name)

		return true
	})

	return nil, nil
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
