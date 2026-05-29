package config

import (
	"context"
	"path/filepath"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/gruntwork-io/terragrunt/internal/vexec"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// EarlyStackParseFunctions returns the HCL function map used to evaluate
// expressions inside a terragrunt.stack.hcl. The keyset matches
// createTerragruntEvalContext and each function is bound to a parsing context
// rescoped to baseDir.
//
// get_working_dir is overridden to return baseDir: the production impl
// re-parses the current config to compute a Terraform source URL, which a
// terragrunt.stack.hcl does not have.
//
// The returned map is freshly allocated on every call (via
// [createTerragruntEvalContext], which builds a new `map[string]function.Function{}`
// per invocation). Callers own the result outright: concurrent discovery
// goroutines each get their own map, and the override write on
// [FuncNameGetWorkingDir] is not visible to any other caller.
//
// Callers building a ParsingContext from *options.TerragruntOptions should use
// configbridge.NewParsingContext.
func EarlyStackParseFunctions(ctx context.Context, l log.Logger, baseDir string, pctx *ParsingContext) (map[string]function.Function, error) {
	stackFilePath := filepath.Join(baseDir, DefaultStackFile)

	_, scoped, err := pctx.WithConfigPath(l, stackFilePath)
	if err != nil {
		return nil, err
	}

	evalCtx, err := createTerragruntEvalContext(ctx, scoped, l, vexec.NewOSExec(), stackFilePath)
	if err != nil {
		return nil, err
	}

	if evalCtx.Functions == nil {
		panic("config.EarlyStackParseFunctions: evalCtx.Functions is nil")
	}

	funcs := evalCtx.Functions
	funcs[FuncNameGetWorkingDir] = stackDirGetWorkingDir(baseDir)

	return funcs, nil
}

func stackDirGetWorkingDir(baseDir string) function.Function {
	return function.New(&function.Spec{
		Type: function.StaticReturnType(cty.String),
		Impl: func(_ []cty.Value, _ cty.Type) (cty.Value, error) {
			return cty.StringVal(baseDir), nil
		},
	})
}
