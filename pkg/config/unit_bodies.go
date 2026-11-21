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
// Engine and MockValues are the exceptions to that scope: the parse decodes them with `dependency` unknown, as
// [PartialParseConfigFile] does when it skips resolving outputs, and [RunBodies] holds them again decoded with
// the outputs. MockValues aligns by index with the output blocks of the outputs block, and holds nil for an output
// with no mock value.
type QueueBodies struct {
	Exclude    *ExcludeBody
	Engine     *EngineBody
	Retry      []RetryBody
	Ignore     []IgnoreBody
	MockValues []*cty.Value
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
	MockValues               []*cty.Value
}

// decodeQueue decodes the queue parts of c with evalCtx, which defines no `dependency` variable in the parse.
func (c *UnitConfig) decodeQueue(evalCtx *hcl.EvalContext) (QueueBodies, hcl.Diagnostics) {
	var (
		q     QueueBodies
		diags hcl.Diagnostics
	)

	if c.Exclude != nil && !c.Exclude.readsDependency() {
		diags = append(diags, decodeInto(evalCtx, unmarkedBody{c.Exclude.Remain}, &q.Exclude)...)
	}

	if c.Errors != nil {
		diags = append(diags, decodeEach(evalCtx, c.Errors.Retry, &q.Retry, (*RetryHCL).remain)...)
		diags = append(diags, decodeEach(evalCtx, c.Errors.Ignore, &q.Ignore, (*IgnoreHCL).remain)...)
	}

	diags = append(diags, c.decodeMockValues(withUnknownDependency(evalCtx), &q.MockValues)...)

	return q, diags
}

// decodeMockValues evaluates the mock value of each output block of c's outputs block with evalCtx into *target,
// which stays nil when c has no outputs block.
func (c *UnitConfig) decodeMockValues(evalCtx *hcl.EvalContext, target *[]*cty.Value) hcl.Diagnostics {
	if c.Outputs == nil {
		return nil
	}

	var diags hcl.Diagnostics

	values := make([]*cty.Value, len(c.Outputs.Output))

	for i, output := range c.Outputs.Output {
		diags = append(diags, decodeAttr(evalCtx, output.MockValue, &values[i])...)
	}

	*target = values

	return diags
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
		diags = append(diags, decodeInto(evalCtx, unmarkedBody{c.Exclude.Remain}, &r.Exclude)...)
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
	diags = append(diags, c.decodeMockValues(evalCtx, &r.MockValues)...)

	return &r, diags
}

// decodeInto decodes remain into a new body at *target.
func decodeInto[B any](evalCtx *hcl.EvalContext, remain hcl.Body, target **B) hcl.Diagnostics {
	body := new(B)
	diags := gohcl.DecodeBody(remain, evalCtx, body)
	*target = body

	return diags
}

// unmarkedBody is a body whose attributes report a sensitive value as an error and pass it on unmarked, for a body
// decoded into Go types, which gohcl panics converting a marked value to. The bodies of its blocks are not wrapped.
type unmarkedBody struct {
	hcl.Body
}

func (b unmarkedBody) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	content, diags := b.Body.Content(schema)
	content.Attributes = unmarkedAttributes(content.Attributes)

	return content, diags
}

func (b unmarkedBody) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	content, remain, diags := b.Body.PartialContent(schema)
	content.Attributes = unmarkedAttributes(content.Attributes)

	return content, unmarkedBody{remain}, diags
}

func (b unmarkedBody) JustAttributes() (hcl.Attributes, hcl.Diagnostics) {
	attrs, diags := b.Body.JustAttributes()

	return unmarkedAttributes(attrs), diags
}

// unmarkedAttributes returns copies of attrs whose expressions report a sensitive value as [unmarkedExpr] does.
func unmarkedAttributes(attrs hcl.Attributes) hcl.Attributes {
	wrapped := make(hcl.Attributes, len(attrs))

	for name, attr := range attrs {
		c := *attr
		c.Expr = unmarkedExpr{attr.Expr}
		wrapped[name] = &c
	}

	return wrapped
}

// unmarkedExpr is an expression that reports a value holding a sensitive part as an error, and returns it unmarked.
type unmarkedExpr struct {
	hcl.Expression
}

func (e unmarkedExpr) Value(evalCtx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	val, diags := e.Expression.Value(evalCtx)
	if !val.ContainsMarked() {
		return val, diags
	}

	unmarked, _ := val.UnmarkDeep()

	return unmarked, append(diags, &hcl.Diagnostic{
		Severity:    hcl.DiagError,
		Summary:     "Sensitive value not allowed",
		Detail:      "This argument does not accept a value marked as sensitive.",
		Subject:     e.Range().Ptr(),
		Expression:  e.Expression,
		EvalContext: evalCtx,
	})
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

// requestedDependencyReaders returns the names of the run parts of c that a section of decodeList decodes and that
// read `dependency`, which [UnitConfig.withRequestedRunParts] leaves out.
func (c *UnitConfig) requestedDependencyReaders(decodeList []PartialDecodeSectionType) []string {
	var parts []string

	if slices.Contains(decodeList, RemoteStateBlock) && (attrReadsDependency(c.RemoteStateAttr) ||
		c.RemoteState != nil && bodyReadsDependency[RemoteStateBody](c.RemoteState.Remain)) {
		parts = append(parts, MetadataRemoteState)
	}

	if c.Terraform == nil {
		return parts
	}

	wholeTerraform := slices.Contains(decodeList, TerraformBlock)

	if (wholeTerraform || slices.Contains(decodeList, TerraformExtraArgs)) &&
		readsDependencyIn[ExtraArgumentsBody](c.Terraform.ExtraArgs, (*ExtraArgumentsHCL).remain) {
		parts = append(parts, "terraform.extra_arguments")
	}

	if wholeTerraform && (readsDependencyIn[HookBody](c.Terraform.BeforeHooks, (*HookHCL).remain) ||
		readsDependencyIn[HookBody](c.Terraform.AfterHooks, (*HookHCL).remain) ||
		readsDependencyIn[ErrorHookBody](c.Terraform.ErrorHooks, (*ErrorHookHCL).remain)) {
		parts = append(parts, "terraform hooks")
	}

	return parts
}

// readsDependencyIn reports whether the remain body of an item of items, which decodes into B, reads `dependency`.
func readsDependencyIn[B, T any](items []T, remain func(*T) hcl.Body) bool {
	for i := range items {
		if bodyReadsDependency[B](remain(&items[i])) {
			return true
		}
	}

	return false
}

// runSections is the decode list sections that ask for every run part [UnitConfig.withRequestedRunParts] can keep.
var runSections = []PartialDecodeSectionType{RemoteStateBlock, TerraformBlock}

// decodesRunParts reports whether section decodes run parts, which [UnitConfig.withRequestedRunParts] keeps.
func decodesRunParts(section PartialDecodeSectionType) bool {
	return section == RemoteStateBlock || section == TerraformBlock || section == TerraformExtraArgs
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
