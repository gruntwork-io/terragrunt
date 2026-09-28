package config

import (
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"

	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
)

// QueueBodies is the parts of a [UnitConfig] beyond its tagged fields that discovery reads, decoded. The parse
// decodes them once, with no `dependency` variable in scope, and keeps them as [UnitConfig.Queue]. A nil pointer is
// an absent part, or an exclude block that reads `dependency`, which [RunBodies] holds instead. Each slice aligns by
// index with the HCL slice it decodes.
type QueueBodies struct {
	Exclude *ExcludeBody
	Retry   []RetryBody
	Ignore  []IgnoreBody
}

// RunBodies is the parts of a [UnitConfig] that only the run reads, decoded. [UnitConfig.ToV1] decodes them with
// the `dependency` outputs resolved, and the parse never does. A nil pointer is an absent part, and each slice
// aligns by index with the HCL slice it decodes. Exclude is set only for an exclude block that reads `dependency`.
type RunBodies struct {
	Exclude         *ExcludeBody
	Inputs          *cty.Value
	GenerateAttrs   *cty.Value
	RemoteStateAttr *cty.Value
	Engine          *EngineBody
	RemoteState     *RemoteStateBody
	ExtraArgs       []ExtraArgumentsBody
	BeforeHooks     []HookBody
	AfterHooks      []HookBody
	ErrorHooks      []ErrorHookBody
	Generate        []GenerateBody
}

// decodeQueue decodes the queue parts of c with evalCtx, which defines no `dependency` variable in the parse.
func (c *UnitConfig) decodeQueue(evalCtx *hcl.EvalContext) (QueueBodies, hcl.Diagnostics) {
	var (
		q     QueueBodies
		diags hcl.Diagnostics
	)

	if c.Exclude != nil && !c.Exclude.readsDependency() {
		diags = append(diags, decodeInto(evalCtx, c.Exclude.Remain, &q.Exclude)...)
	}

	if c.Errors != nil {
		diags = append(diags, decodeEach(evalCtx, c.Errors.Retry, &q.Retry, (*RetryHCL).remain)...)
		diags = append(diags, decodeEach(evalCtx, c.Errors.Ignore, &q.Ignore, (*IgnoreHCL).remain)...)
	}

	return q, diags
}

// decodeRun decodes the run parts of c with evalCtx.
func (c *UnitConfig) decodeRun(evalCtx *hcl.EvalContext) (*RunBodies, hcl.Diagnostics) {
	var (
		r     RunBodies
		diags hcl.Diagnostics
	)

	if c.Exclude != nil && c.Exclude.readsDependency() {
		diags = append(diags, decodeInto(evalCtx, c.Exclude.Remain, &r.Exclude)...)
	}

	diags = append(diags, decodeAttr(evalCtx, c.Inputs, &r.Inputs)...)
	diags = append(diags, decodeAttr(evalCtx, c.GenerateAttrs, &r.GenerateAttrs)...)
	diags = append(diags, decodeAttr(evalCtx, c.RemoteStateAttr, &r.RemoteStateAttr)...)

	if c.Engine != nil {
		diags = append(diags, decodeInto(evalCtx, c.Engine.Remain, &r.Engine)...)
	}

	if c.RemoteState != nil {
		diags = append(diags, decodeInto(evalCtx, c.RemoteState.Remain, &r.RemoteState)...)
	}

	if c.Terraform != nil {
		diags = append(diags, decodeEach(evalCtx, c.Terraform.ExtraArgs, &r.ExtraArgs, (*ExtraArgumentsHCL).remain)...)
		diags = append(diags, decodeEach(evalCtx, c.Terraform.BeforeHooks, &r.BeforeHooks, (*HookHCL).remain)...)
		diags = append(diags, decodeEach(evalCtx, c.Terraform.AfterHooks, &r.AfterHooks, (*HookHCL).remain)...)
		diags = append(diags, decodeEach(evalCtx, c.Terraform.ErrorHooks, &r.ErrorHooks, (*ErrorHookHCL).remain)...)
	}

	diags = append(diags, decodeEach(evalCtx, c.GenerateBlocks, &r.Generate, (*GenerateHCL).remain)...)

	return &r, diags
}

// decodeInto decodes remain into a new body at *target.
func decodeInto[B any](evalCtx *hcl.EvalContext, remain hcl.Body, target **B) hcl.Diagnostics {
	body := new(B)
	diags := gohcl.DecodeBody(remain, evalCtx, body)
	*target = body

	return diags
}

// decodeAttr evaluates attr into *target. A nil attr is absent, and a failed evaluation leaves *target nil.
func decodeAttr(evalCtx *hcl.EvalContext, attr *hcl.Attribute, target **cty.Value) hcl.Diagnostics {
	if attr == nil {
		return nil
	}

	return gohcl.DecodeExpression(attr.Expr, evalCtx, target)
}

// decodeEach decodes the remain body of each item into a body at the same index of *target, which stays nil for no
// items.
func decodeEach[T, B any](
	evalCtx *hcl.EvalContext,
	items []T,
	target *[]B,
	remain func(*T) hcl.Body,
) hcl.Diagnostics {
	if len(items) == 0 {
		return nil
	}

	var diags hcl.Diagnostics

	bodies := make([]B, len(items))

	for i := range items {
		diags = append(diags, gohcl.DecodeBody(remain(&items[i]), evalCtx, &bodies[i])...)
	}

	*target = bodies

	return diags
}

// readsDependency reports whether an attribute of e reads the `dependency` variable.
func (e *ExcludeHCL) readsDependency() bool {
	schema, _ := gohcl.ImpliedBodySchema(new(ExcludeBody))

	spec := make(hcldec.ObjectSpec, len(schema.Attributes))
	for _, attr := range schema.Attributes {
		spec[attr.Name] = &hcldec.AttrSpec{Name: attr.Name, Type: cty.DynamicPseudoType}
	}

	return slices.ContainsFunc(hcldec.Variables(e.Remain, spec), func(traversal hcl.Traversal) bool {
		return traversal.RootName() == pkgconfig.MetadataDependency
	})
}

func (a *ExtraArgumentsHCL) remain() hcl.Body {
	return a.Remain
}

func (h *HookHCL) remain() hcl.Body {
	return h.Remain
}

func (h *ErrorHookHCL) remain() hcl.Body {
	return h.Remain
}

func (r *RetryHCL) remain() hcl.Body {
	return r.Remain
}

func (i *IgnoreHCL) remain() hcl.Body {
	return i.Remain
}

func (g *GenerateHCL) remain() hcl.Body {
	return g.Remain
}
