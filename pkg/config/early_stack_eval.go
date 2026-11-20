package config

import (
	"context"
	"maps"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// StackParseFunctionsFrom returns a copy of funcs with get_working_dir overridden to return baseDir, for callers that already hold the stack-file eval context (the input map is not mutated).
func StackParseFunctionsFrom(
	funcs map[string]function.Function,
	baseDir string,
) map[string]function.Function {
	out := maps.Clone(funcs)
	out[FuncNameGetWorkingDir] = stackDirGetWorkingDir(baseDir)

	return out
}

// EarlyStackParseFunctions returns the HCL function map used to evaluate
// expressions inside a terragrunt.stack.hcl. The keyset matches
// CreateTerragruntEvalContext given [hclparse.AnyFunctionCalls], and each
// function is bound to a parsing context rescoped to baseDir.
//
// get_working_dir is overridden to return baseDir: the production impl
// re-parses the current config to compute a Terraform source URL, which a
// terragrunt.stack.hcl does not have.
//
// The returned map is freshly allocated on every call. Callers own the result
// outright: concurrent discovery goroutines each get their own map, and a write
// to it is not visible to any other caller.
//
// Callers building a ParsingContext from *options.TerragruntOptions should use
// configbridge.NewParsingContext.
func EarlyStackParseFunctions(
	ctx context.Context,
	l log.Logger,
	v *venv.Venv,
	pctx *ParsingContext,
	baseDir string,
) (map[string]function.Function, error) {
	stackFilePath := filepath.Join(baseDir, DefaultStackFile)

	_, scoped, err := pctx.WithConfigPath(l, stackFilePath)
	if err != nil {
		return nil, err
	}

	evalCtx, err := CreateTerragruntEvalContext(ctx, l, v, scoped, stackFilePath, hclparse.AnyFunctionCalls())
	if err != nil {
		return nil, err
	}

	return StackParseFunctionsFrom(EvalFunctions(evalCtx), baseDir), nil
}

func stackDirGetWorkingDir(baseDir string) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func(_ []cty.Value, _ cty.Type) (cty.Value, error) {
			return cty.StringVal(baseDir), nil
		},
	})
}
