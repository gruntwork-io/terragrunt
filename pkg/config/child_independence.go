package config

import (
	"maps"
	"slices"
	"sync"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	tflang "github.com/hashicorp/terraform/lang"
)

// maxChildIndependenceDepth bounds the expression walk. HCL nests without limit, and an
// expression deeper than this is treated as unclassifiable rather than walked further.
const maxChildIndependenceDepth = 1000

// childIndependenceUsage records what the walked expressions reach for, so the caller can
// fold it into the cache key.
type childIndependenceUsage struct {
	// readsEnv is set when an expression calls get_env.
	readsEnv bool
}

// funcNameChildIndependence records, for every Terragrunt HCL function, whether its result
// can vary between the units that pull one shared parent in. Terragrunt parses a parent with
// the child's config path, working directory, command, IAM role, and read set still in the
// parsing context, so a function that reaches for any of those answers differently for each
// child.
//
// A name missing from this map is unclassified, and a file that calls it is never cached. A
// function added to Terragrunt therefore stays uncached until someone classifies it here,
// rather than silently handing one child another child's value.
var funcNameChildIndependence = map[string]bool{
	// Read only their arguments, the process environment, or a list baked into Terragrunt,
	// all of which are the same for every unit in a run. get_env additionally folds the
	// environment into the cache key, since dependency output fetching can add keys mid-run.
	FuncNameGetEnv:                                  true,
	FuncNameGetPlatform:                             true,
	FuncNameGetTerraformCommandsThatNeedVars:        true,
	FuncNameGetTerraformCommandsThatNeedLocking:     true,
	FuncNameGetTerraformCommandsThatNeedInput:       true,
	FuncNameGetTerraformCommandsThatNeedParallelism: true,
	FuncNameGetDefaultRetryableErrors:               true,
	FuncNameConstraintCheck:                         true,
	FuncNameDeepMerge:                               true,
	FuncNameStartsWith:                              true,
	FuncNameEndsWith:                                true,
	FuncNameStrContains:                             true,
	FuncNameTimeCmp:                                 true,

	// Answer from the command the unit being parsed is running. A run rewrites both per
	// unit: the runner gives each unit its own arguments, down to its own plan file, and
	// dependency output resolution swaps in "output" before it parses the dependency target.
	FuncNameGetTerraformCommand: false,
	FuncNameGetTerraformCLIArgs: false,

	// Answer from the child's own path: they walk up from, or resolve against, the config
	// path and working directory of the unit being parsed.
	FuncNameFindInParentFolders:        false,
	FuncNamePathRelativeToInclude:      false,
	FuncNamePathRelativeFromInclude:    false,
	FuncNameGetTerragruntDir:           false,
	FuncNameGetOriginalTerragruntDir:   false,
	FuncNameGetParentTerragruntDir:     false,
	FuncNameGetTerragruntSourceCLIFlag: false,
	FuncNameGetWorkingDir:              false,
	FuncNameGetRepoRoot:                false,
	FuncNameGetPathFromRepoRoot:        false,
	FuncNameGetPathToRepoRoot:          false,

	// Resolve a relative path against the child's directory, and run_cmd also caches and
	// replays its output per calling directory.
	FuncNameRunCmd:               false,
	FuncNameReadTerragruntConfig: false,
	FuncNameReadTfvarsFile:       false,
	FuncNameSopsDecryptFile:      false,

	// Record the file they read on the child's read set, a side effect a cache hit would
	// skip for every unit after the first.
	FuncNameMarkAsRead:     false,
	FuncNameMarkGlobAsRead: false,

	// Assume the IAM role the child's own config selected, which setIAMRole resolves per
	// unit before the parent is decoded.
	FuncNameGetAWSAccountAlias:         false,
	FuncNameGetAWSAccountID:            false,
	FuncNameGetAWSCallerIdentityArn:    false,
	FuncNameGetAWSCallerIdentityUserID: false,
}

// nondeterministicTFLangFuncs are the OpenTofu and Terraform builtins that answer
// differently on every call. Caching one would hand every later unit the first unit's
// value, so they stay off the allowlist.
var nondeterministicTFLangFuncs = map[string]bool{
	"bcrypt":    true,
	"timestamp": true,
	"uuid":      true,
}

// fileReadingTFLangFuncs are the OpenTofu and Terraform builtins that answer from a file on
// disk. Nothing but the parsed file's own content is in the cache key, so a unit parsed
// after one of these files changed would be handed what the first unit read, which is not
// what it would have computed for itself. They stay off the allowlist.
var fileReadingTFLangFuncs = map[string]bool{
	"file":             true,
	"filebase64":       true,
	"filebase64sha256": true,
	"filebase64sha512": true,
	"fileexists":       true,
	"filemd5":          true,
	"fileset":          true,
	"filesha1":         true,
	"filesha256":       true,
	"filesha512":       true,
	"templatefile":     true,
}

// childIndependentTFLangFuncs returns the builtin function names that may appear in a
// cached file. They depend on their arguments and on the scope's base directory, which is
// the directory of the file being parsed rather than the child's.
var childIndependentTFLangFuncs = sync.OnceValue(func() map[string]bool {
	scope := tflang.Scope{}
	names := map[string]bool{}

	for name := range scope.Functions() {
		if nondeterministicTFLangFuncs[name] || fileReadingTFLangFuncs[name] {
			continue
		}

		names[name] = true
	}

	return names
})

// ClassifiedFuncNames returns, sorted, every Terragrunt HCL function name the cache has a
// classification for. Pair it with [FuncNameIsChildIndependent] to walk the whole table: a
// caller that enumerates it is looking at what the cache actually consults, rather than at a
// copy that can fall behind.
func ClassifiedFuncNames() []string {
	return slices.Sorted(maps.Keys(funcNameChildIndependence))
}

// FuncNameIsChildIndependent reports whether the HCL function called name returns a value
// that cannot vary with the unit a shared parent is being parsed for, and whether name is
// classified at all. An unclassified name is never treated as independent.
func FuncNameIsChildIndependent(name string) (independent, classified bool) {
	independent, classified = funcNameChildIndependence[name]

	return independent, classified
}

// funcCallIsChildIndependent reports whether a call to name may appear in a cached file.
// Terragrunt's own table shadows the builtins, so its classification is consulted first.
func funcCallIsChildIndependent(name string) bool {
	if independent, classified := FuncNameIsChildIndependent(name); classified {
		return independent
	}

	return childIndependentTFLangFuncs()[name]
}

// bodyIsChildIndependent reports whether every expression in body, and in the blocks nested
// under it, is safe to cache. roots names the variable prefixes the expressions may read;
// anything else is a value the caller supplied for one particular child.
func bodyIsChildIndependent(
	body *hclsyntax.Body,
	roots map[string]bool,
	usage *childIndependenceUsage,
	depth int,
) bool {
	if depth > maxChildIndependenceDepth {
		return false
	}

	for _, attr := range body.Attributes {
		if !attrIsChildIndependent(attr.Expr, roots, usage) {
			return false
		}
	}

	for _, block := range body.Blocks {
		if !bodyIsChildIndependent(block.Body, roots, usage, depth+1) {
			return false
		}
	}

	return true
}

// attrIsChildIndependent reports whether one attribute expression is safe to cache.
func attrIsChildIndependent(
	expr hclsyntax.Expression,
	roots map[string]bool,
	usage *childIndependenceUsage,
) bool {
	// Variables reports the traversals the whole tree reads, with a for expression's own
	// loop variables and a splat's anonymous symbol already excluded.
	for _, traversal := range expr.Variables() {
		if !roots[traversal.RootName()] {
			return false
		}
	}

	return callsAreChildIndependent(expr, usage, 0)
}

// callsAreChildIndependent walks expr for function calls and reports whether every one of
// them is on the allowlist. An expression type this walk does not model could hide a call
// in a child node, so it answers false rather than guessing.
func callsAreChildIndependent(
	expr hclsyntax.Expression,
	usage *childIndependenceUsage,
	depth int,
) bool {
	if depth > maxChildIndependenceDepth {
		return false
	}

	depth++

	switch e := expr.(type) {
	case *hclsyntax.LiteralValueExpr, *hclsyntax.ScopeTraversalExpr, *hclsyntax.AnonSymbolExpr:
		return true
	case *hclsyntax.FunctionCallExpr:
		if !funcCallIsChildIndependent(e.Name) {
			return false
		}

		if e.Name == FuncNameGetEnv {
			usage.readsEnv = true
		}

		return callsAreChildIndependentIn(e.Args, usage, depth)
	case *hclsyntax.TemplateExpr:
		return callsAreChildIndependentIn(e.Parts, usage, depth)
	case *hclsyntax.TemplateWrapExpr:
		return callsAreChildIndependent(e.Wrapped, usage, depth)
	case *hclsyntax.TemplateJoinExpr:
		return callsAreChildIndependent(e.Tuple, usage, depth)
	case *hclsyntax.ObjectConsExpr:
		for _, item := range e.Items {
			if !callsAreChildIndependentIn(
				[]hclsyntax.Expression{item.KeyExpr, item.ValueExpr},
				usage,
				depth,
			) {
				return false
			}
		}

		return true
	case *hclsyntax.ObjectConsKeyExpr:
		return callsAreChildIndependent(e.Wrapped, usage, depth)
	case *hclsyntax.TupleConsExpr:
		return callsAreChildIndependentIn(e.Exprs, usage, depth)
	case *hclsyntax.ConditionalExpr:
		return callsAreChildIndependentIn(
			[]hclsyntax.Expression{e.Condition, e.TrueResult, e.FalseResult},
			usage,
			depth,
		)
	case *hclsyntax.ParenthesesExpr:
		return callsAreChildIndependent(e.Expression, usage, depth)
	case *hclsyntax.BinaryOpExpr:
		return callsAreChildIndependentIn([]hclsyntax.Expression{e.LHS, e.RHS}, usage, depth)
	case *hclsyntax.UnaryOpExpr:
		return callsAreChildIndependent(e.Val, usage, depth)
	case *hclsyntax.IndexExpr:
		return callsAreChildIndependentIn(
			[]hclsyntax.Expression{e.Collection, e.Key},
			usage,
			depth,
		)
	case *hclsyntax.RelativeTraversalExpr:
		return callsAreChildIndependent(e.Source, usage, depth)
	case *hclsyntax.ForExpr:
		return callsAreChildIndependentIn(
			[]hclsyntax.Expression{e.CollExpr, e.KeyExpr, e.ValExpr, e.CondExpr},
			usage,
			depth,
		)
	case *hclsyntax.SplatExpr:
		return callsAreChildIndependentIn([]hclsyntax.Expression{e.Source, e.Each}, usage, depth)
	}

	return false
}

// callsAreChildIndependentIn walks a list of sub-expressions, skipping the optional ones a
// parent node leaves nil.
func callsAreChildIndependentIn(
	exprs []hclsyntax.Expression,
	usage *childIndependenceUsage,
	depth int,
) bool {
	for _, expr := range exprs {
		if expr == nil {
			continue
		}

		if !callsAreChildIndependent(expr, usage, depth) {
			return false
		}
	}

	return true
}
