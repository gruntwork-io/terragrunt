package config

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// parseFunctions is the functions one caller's parse of a file calls, built on first use and shared by the
// stages of that parse. Each stage gives its own parsing context, which the functions read when called, so the stages
// that share them must run one after another.
type parseFunctions struct {
	ctx     context.Context
	l       log.Logger
	v       *venv.Venv
	pctx    *ParsingContext
	evalCtx *hcl.EvalContext
	file    *hclparse.File
}

// newParseFunctions returns the functions of a parse of file that file calls, which use ctx, l, and v when called.
func newParseFunctions(ctx context.Context, l log.Logger, v *venv.Venv, file *hclparse.File) *parseFunctions {
	return &parseFunctions{ctx: ctx, l: l, v: v, file: file}
}

// evalContext returns an eval context with vars and the functions, which read a copy of pctx when called until the
// next call.
func (f *parseFunctions) evalContext(pctx *ParsingContext, vars map[string]cty.Value) *hcl.EvalContext {
	if f.evalCtx == nil {
		bound := *pctx
		f.pctx = &bound
		f.evalCtx = TerragruntEvalContext(f.ctx, f.l, f.v, f.pctx, f.file.ConfigPath, f.file.FunctionCalls(), nil)
	}

	*f.pctx = *pctx

	evalCtx := *f.evalCtx
	evalCtx.Variables = vars

	return &evalCtx
}
