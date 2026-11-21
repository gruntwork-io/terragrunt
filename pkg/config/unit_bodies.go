package config

import (
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/zclconf/go-cty/cty"

)

// QueueBodies is the parts of a [UnitConfig] beyond its tagged fields that discovery reads, decoded. The parse
// decodes them once, with no `dependency` variable in scope, and keeps them as [UnitConfig.Queue]. A nil pointer is
// an absent part, or an exclude block that reads `dependency`, which [RunBodies] holds instead. Each slice aligns by
// index with the HCL slice it decodes.
//
// Engine is the exception to that scope: the parse decodes it with `dependency` unknown, as
// [PartialParseConfigFile] does when it skips resolving outputs, and [RunBodies] holds it again decoded with
// the outputs.
type QueueBodies struct {
	Exclude *ExcludeBody
	Engine  *EngineBody
	Retry   []RetryBody
	Ignore  []IgnoreBody
}

// RunBodies is the parts of a [UnitConfig] that only the run reads, decoded. [UnitConfig.ToV1] decodes them with
// the `dependency` outputs resolved, and the parse never does. A nil pointer is an absent part, and each slice
// aligns by index with the HCL slice it decodes. Exclude is set only for an exclude block that reads `dependency`.
type RunBodies struct {
	Exclude                  *ExcludeBody
	Inputs                   *cty.Value
	GenerateAttrs            *cty.Value
	RemoteStateAttr          *cty.Value
	Engine                   *EngineBody
	RemoteState              *RemoteStateBody
	DownloadDir              *string
	IamAssumeRoleDuration    *int64
	IamAssumeRoleSessionName *string
	ExtraArgs                []ExtraArgumentsBody
	BeforeHooks              []HookBody
	AfterHooks               []HookBody
	ErrorHooks               []ErrorHookBody
	Generate                 []GenerateBody
	Variables                []VariablesBody
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

// decodeQueueEngine decodes the engine block of c with evalCtx and `dependency` unknown. It decodes nothing when c
// has no engine block.
func (c *UnitConfig) decodeQueueEngine(evalCtx *hcl.EvalContext) (*EngineBody, hcl.Diagnostics) {
	if c.Engine == nil {
		return nil, nil
	}

	var body *EngineBody

	diags := decodeInto(withUnknownDependency(evalCtx), c.Engine.Remain, &body)

	return body, diags
}

// withUnknownDependency returns evalCtx with an unknown `dependency` variable, or evalCtx itself when it is nil or
// already defines one, which is the value the run supplied.
func withUnknownDependency(evalCtx *hcl.EvalContext) *hcl.EvalContext {
	if evalCtx == nil {
		return nil
	}

	if _, ok := evalCtx.Variables[MetadataDependency]; ok {
		return evalCtx
	}

	withDependency := *evalCtx
	withDependency.Variables = maps.Clone(evalCtx.Variables)
	withDependency.Variables[MetadataDependency] = cty.DynamicVal

	return &withDependency
}

// dependencyBlockContext returns the eval context the dependency blocks decode with: evalCtx with `dependency`
// unknown when pctx skips resolving outputs, as [PartialParseConfigFile] decodes them, and evalCtx
// otherwise.
func dependencyBlockContext(pctx *ParsingContext, evalCtx *hcl.EvalContext) *hcl.EvalContext {
	if !pctx.SkipOutputsResolution {
		return evalCtx
	}

	return withUnknownDependency(evalCtx)
}

// declaresDependencies reports whether body has a dependency block. A body whose dependency blocks fail to parse
// reports true, so that their decode reports why.
func declaresDependencies(body hcl.Body) bool {
	content, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: MetadataDependency, LabelNames: []string{"name"}}},
	})

	return diags.HasErrors() || len(content.Blocks) > 0
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

	diags = append(diags, c.decodeRunAttributes(evalCtx, &r)...)
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
	diags = append(diags, decodeEach(evalCtx, c.VariablesBlocks, &r.Variables, (*VariablesHCL).remain)...)

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
	return bodyReadsDependency[ExcludeBody](e.Remain)
}

// withRequestedRunParts returns a copy of c with only the run parts that a section of decodeList decodes, as
// [PartialParseConfig] does, and that read no `dependency`. It keeps the engine block, which the queue
// decodes with `dependency` unknown whatever the decode list.
func (c *UnitConfig) withRequestedRunParts(decodeList []PartialDecodeSectionType) *UnitConfig {
	requested := *c
	requested.Inputs = nil
	requested.GenerateAttrs = nil
	requested.GenerateBlocks = nil
	requested.VariablesBlocks = nil
	requested.runAttrs = nil

	if !slices.Contains(decodeList, RemoteStateBlock) || attrReadsDependency(c.RemoteStateAttr) {
		requested.RemoteStateAttr = nil
	}

	if !slices.Contains(decodeList, RemoteStateBlock) ||
		c.RemoteState != nil && bodyReadsDependency[RemoteStateBody](c.RemoteState.Remain) {
		requested.RemoteState = nil
	}

	if c.Terraform == nil {
		return &requested
	}

	terraform := *c.Terraform
	terraform.ExtraArgs = nil
	terraform.BeforeHooks = nil
	terraform.AfterHooks = nil
	terraform.ErrorHooks = nil
	requested.Terraform = &terraform

	wholeTerraform := slices.Contains(decodeList, TerraformBlock)

	if wholeTerraform || slices.Contains(decodeList, TerraformExtraArgs) {
		terraform.ExtraArgs = dropDependencyReaders[ExtraArgumentsBody](c.Terraform.ExtraArgs, (*ExtraArgumentsHCL).remain)
	}

	if !wholeTerraform {
		return &requested
	}

	terraform.BeforeHooks = dropDependencyReaders[HookBody](c.Terraform.BeforeHooks, (*HookHCL).remain)
	terraform.AfterHooks = dropDependencyReaders[HookBody](c.Terraform.AfterHooks, (*HookHCL).remain)
	terraform.ErrorHooks = dropDependencyReaders[ErrorHookBody](c.Terraform.ErrorHooks, (*ErrorHookHCL).remain)

	return &requested
}

// dropDependencyReaders returns a copy of items without those whose remain body, which decodes into B, reads the
// `dependency` variable.
func dropDependencyReaders[B, T any](items []T, remain func(*T) hcl.Body) []T {
	return slices.DeleteFunc(slices.Clone(items), func(item T) bool {
		return bodyReadsDependency[B](remain(&item))
	})
}

// bodyReadsDependency reports whether an attribute of remain, which decodes into B, reads the `dependency` variable.
func bodyReadsDependency[B any](remain hcl.Body) bool {
	schema, _ := gohcl.ImpliedBodySchema(new(B))

	spec := make(hcldec.ObjectSpec, len(schema.Attributes))
	for _, attr := range schema.Attributes {
		spec[attr.Name] = &hcldec.AttrSpec{Name: attr.Name, Type: cty.DynamicPseudoType}
	}

	return slices.ContainsFunc(hcldec.Variables(remain, spec), isDependencyTraversal)
}

// attrReadsDependency reports whether attr, which is nil when absent, reads the `dependency` variable.
func attrReadsDependency(attr *hcl.Attribute) bool {
	return attr != nil && slices.ContainsFunc(attr.Expr.Variables(), isDependencyTraversal)
}

// isDependencyTraversal reports whether traversal reads the `dependency` variable.
func isDependencyTraversal(traversal hcl.Traversal) bool {
	return traversal.RootName() == MetadataDependency
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

func (b *VariablesHCL) remain() hcl.Body {
	return b.Remain
}
