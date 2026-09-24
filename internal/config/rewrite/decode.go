package config

import (
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

// decode decodes c's tagged fields, then resolves the remaining bodies, and returns the diagnostics of both.
func (c *UnitConfig) decode(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	diags := gohcl.DecodeBody(c.file.Body, evalCtx, c)

	return append(diags, c.resolve(evalCtx)...)
}

// resolve decodes Remain into Body, evaluates the remote_state attribute, and resolves each block.
func (c *UnitConfig) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	diags := gohcl.DecodeBody(c.Remain, evalCtx, &c.Body)

	if c.RemoteStateAttr != nil {
		diags = append(diags, gohcl.DecodeExpression(c.RemoteStateAttr.Expr, evalCtx, &c.Body.RemoteStateAttr)...)
	}

	if c.Terraform != nil {
		diags = append(diags, c.Terraform.resolve(evalCtx)...)
	}

	if c.RemoteState != nil {
		diags = append(diags, c.RemoteState.resolve(evalCtx)...)
	}

	if c.Engine != nil {
		diags = append(diags, c.Engine.resolve(evalCtx)...)
	}

	if c.Errors != nil {
		diags = append(diags, c.Errors.resolve(evalCtx)...)
	}

	return diags
}

// resolve decodes the body of each nested block of t.
func (t *Terraform) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return slices.Concat(
		resolveEach(t.ExtraArgs, evalCtx),
		resolveEach(t.BeforeHooks, evalCtx),
		resolveEach(t.AfterHooks, evalCtx),
		resolveEach(t.ErrorHooks, evalCtx),
	)
}

// resolve decodes the body of each retry and ignore block of e.
func (e *Errors) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return slices.Concat(resolveEach(e.Retry, evalCtx), resolveEach(e.Ignore, evalCtx))
}

func (r *RemoteState) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return gohcl.DecodeBody(r.Remain, evalCtx, &r.Body)
}

func (e *Engine) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return gohcl.DecodeBody(e.Remain, evalCtx, &e.Body)
}

func (a *ExtraArguments) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return gohcl.DecodeBody(a.Remain, evalCtx, &a.Body)
}

func (h *Hook) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return gohcl.DecodeBody(h.Remain, evalCtx, &h.Body)
}

func (h *ErrorHook) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return gohcl.DecodeBody(h.Remain, evalCtx, &h.Body)
}

func (r *Retry) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return gohcl.DecodeBody(r.Remain, evalCtx, &r.Body)
}

func (i *Ignore) resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics {
	return gohcl.DecodeBody(i.Remain, evalCtx, &i.Body)
}

// resolveEach resolves each item and returns their diagnostics.
func resolveEach[T any, P interface {
	*T
	resolve(evalCtx *hcl.EvalContext) hcl.Diagnostics
}](items []T, evalCtx *hcl.EvalContext) hcl.Diagnostics {
	diags := make(hcl.Diagnostics, 0, len(items))

	for i := range items {
		diags = append(diags, P(&items[i]).resolve(evalCtx)...)
	}

	return diags
}
