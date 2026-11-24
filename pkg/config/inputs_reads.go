package config

import (
	"slices"

	"github.com/gruntwork-io/terragrunt/internal/vendored/opentofu/patch"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// InputsRead is a call to a file-reading function in an inputs attribute.
type InputsRead struct {
	// Function is the name of the function called.
	Function string
	// Range is the expression that names the file read: the path argument of templatefile, since
	// the rest of that call may render dependency outputs and stays where it is, and the whole call
	// of any other function.
	Range hcl.Range
	// MarkedAsRead reports whether a mark_as_read call that stack generation runs names the file.
	MarkedAsRead bool
}

// FindInputsReads returns the calls to file-reading functions in the inputs attribute of body, in source order,
// including calls nested in other expressions. Terragrunt evaluates inputs after it builds the run queue, so these
// reads never reach [ParsingContext.FilesRead] and reading filters cannot see them. A call whose file name depends
// on a dependency output is left out: locals cannot reference dependency outputs, so that read cannot move. The
// result is nil when body has no inputs attribute or is not native HCL syntax, as a JSON config is.
func FindInputsReads(body hcl.Body) ([]InputsRead, hcl.Diagnostics) {
	syntaxBody, ok := body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}

	attr, ok := syntaxBody.Attributes[MetadataInputs]
	if !ok {
		return nil, nil
	}

	var reads []InputsRead

	var deferredLoops []*hclsyntax.ForExpr

	diags := hclsyntax.VisitAll(attr.Expr, func(node hclsyntax.Node) hcl.Diagnostics {
		if loop, ok := node.(*hclsyntax.ForExpr); ok && referencesDependency(loop) {
			deferredLoops = append(deferredLoops, loop)

			return nil
		}

		call, ok := node.(*hclsyntax.FunctionCallExpr)
		if !ok {
			return nil
		}

		if _, ok := fileReadingFunctions[call.Name]; !ok {
			return nil
		}

		fileExpr := fileExpression(call)
		if referencesDependency(fileExpr) {
			return nil
		}

		reads = append(reads, InputsRead{
			Function:     call.Name,
			Range:        fileExpr.Range(),
			MarkedAsRead: marksPathAsRead(call, deferredLoops),
		})

		return nil
	})

	return reads, diags
}

// fileExpression returns the expression of call that names the file it reads, as [InputsRead.Range]
// describes. A templatefile call with no arguments, which parses but cannot evaluate, is the expression itself.
func fileExpression(call *hclsyntax.FunctionCallExpr) hclsyntax.Expression {
	if call.Name == patch.FuncNameTemplateFile && len(call.Args) > 0 {
		return call.Args[0]
	}

	return call
}

// marksPathAsRead reports whether call, or its path argument, is a mark_as_read call that no loop in loops defers.
func marksPathAsRead(call *hclsyntax.FunctionCallExpr, loops []*hclsyntax.ForExpr) bool {
	marker := pathMarker(call)
	if marker == nil {
		return false
	}

	deferred := make(map[string]struct{})

	for _, loop := range loops {
		if loop.Range().ContainsOffset(marker.Range().Start.Byte) {
			deferred[loop.KeyVar] = struct{}{}
			deferred[loop.ValVar] = struct{}{}
		}
	}

	for _, traversal := range marker.Variables() {
		if _, ok := deferred[traversal.RootName()]; ok {
			return false
		}
	}

	return true
}

// pathMarker returns call when it is mark_as_read, its first argument when that is, and nil otherwise.
func pathMarker(call *hclsyntax.FunctionCallExpr) *hclsyntax.FunctionCallExpr {
	if call.Name == FuncNameMarkAsRead {
		return call
	}

	if len(call.Args) == 0 {
		return nil
	}

	arg, ok := call.Args[0].(*hclsyntax.FunctionCallExpr)
	if !ok || arg.Name != FuncNameMarkAsRead {
		return nil
	}

	return arg
}

// referencesDependency reports whether expr reads a dependency output.
func referencesDependency(expr hclsyntax.Expression) bool {
	for _, traversal := range expr.Variables() {
		if traversal.RootName() == MetadataDependency {
			return true
		}
	}

	return false
}

// fileReadingFunctions is the set of functions that record the file they read in [ParsingContext.FilesRead].
var fileReadingFunctions = fileReadingFunctionSet()

func fileReadingFunctionSet() map[string]struct{} {
	names := slices.Concat(patch.ReadingFunctionNames(), []string{
		FuncNameReadTerragruntConfig,
		FuncNameReadTfvarsFile,
		FuncNameSopsDecryptFile,
		FuncNameMarkAsRead,
		FuncNameMarkGlobAsRead,
	})

	set := make(map[string]struct{}, len(names))
	for _, name := range names {
		set[name] = struct{}{}
	}

	return set
}
