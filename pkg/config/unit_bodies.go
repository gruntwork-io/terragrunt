package config

import (
	"maps"
	"reflect"
	"slices"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/vendored/hcl/hcldecode"
)

// QueueBodies is the parts of a [UnitConfig] beyond its tagged fields that discovery reads, decoded. The parse
// decodes them once, with no `dependency` variable in scope, and keeps them as [UnitConfig.Queue]. A nil pointer is
// an absent part, or a catalog, exclude, terraform, or errors block whose body reads `dependency`, which [RunBodies]
// holds instead. Each slice aligns by index with the HCL slice it decodes.
//
// Engine and MockValues are the exceptions to that scope: the parse decodes them with `dependency` unknown, and
// [RunBodies] holds them again decoded with the outputs. MockValues aligns by index with the output blocks of the
// outputs block, and holds nil for an output with no mock value.
type QueueBodies struct {
	Catalog    *CatalogBody
	Exclude    *ExcludeBody
	Engine     *EngineBody
	Terraform  *TerraformBody
	Errors     *ErrorsBodies
	MockValues []*cty.Value
}

// ErrorsBodies is the retry and ignore blocks of an errors block, decoded. Each slice aligns by index with the HCL
// slice it decodes.
type ErrorsBodies struct {
	Retry  []RetryBody
	Ignore []IgnoreBody
}

// RunBodies is the parts of a [UnitConfig] that only the run reads, decoded. [UnitConfig.ResolveRunConfig] decodes them
// with the `dependency` outputs resolved, and the parse never does. A nil pointer is an absent part, and each slice
// aligns by index with the HCL slice it decodes. Catalog, Exclude, Terraform, and Errors are set only for a block
// whose body reads `dependency`.
type RunBodies struct {
	Catalog                  *CatalogBody
	Exclude                  *ExcludeBody
	Terraform                *TerraformBody
	Errors                   *ErrorsBodies
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

	if c.Catalog != nil && !c.Catalog.readsDependency() {
		diags = append(diags, decodeInto(evalCtx, unmarkedBody{c.Catalog.Remain}, &q.Catalog)...)
	}

	if c.Exclude != nil && !c.Exclude.readsDependency() {
		diags = append(diags, decodeInto(evalCtx, unmarkedBody{c.Exclude.Remain}, &q.Exclude)...)
	}

	if c.Terraform != nil && !c.terraformRunPart {
		diags = append(diags, decodeInto(evalCtx, c.Terraform.Remain, &q.Terraform)...)
	}

	if c.Errors != nil && !c.errorsRunPart {
		diags = append(diags, c.Errors.decode(evalCtx, &q.Errors)...)
	}

	diags = append(diags, c.decodeMockValues(withUnknownDependency(evalCtx), &q.MockValues)...)

	return q, diags
}

// decode decodes the retry and ignore blocks of e with evalCtx into a new [ErrorsBodies] at *target.
func (e *ErrorsHCL) decode(evalCtx *hcl.EvalContext, target **ErrorsBodies) hcl.Diagnostics {
	bodies := new(ErrorsBodies)

	diags := decodeEach(evalCtx, e.Retry, &bodies.Retry, (*RetryHCL).remain)
	diags = append(diags, decodeEach(evalCtx, e.Ignore, &bodies.Ignore, (*IgnoreHCL).remain)...)
	*target = bodies

	return diags
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
// unknown when pctx skips resolving outputs, and evalCtx otherwise.
func dependencyBlockContext(pctx *ParsingContext, evalCtx *hcl.EvalContext) *hcl.EvalContext {
	if !pctx.SkipOutputsResolution {
		return evalCtx
	}

	return withUnknownDependency(evalCtx)
}

// dependencyBlocksSchema names the dependency blocks of a body.
var dependencyBlocksSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{{Type: MetadataDependency, LabelNames: []string{"name"}}},
}

// declaresDependencies reports whether body, a parsed file's body, has a dependency block. A body whose dependency
// blocks fail to parse reports true, so that their decode reports why.
func declaresDependencies(body hcl.Body) bool {
	content, diags := hcldecode.PartialContent(body, dependencyBlocksSchema)

	return diags.HasErrors() || len(content.Blocks) > 0
}

// decodeRun decodes the run parts of c with evalCtx.
func (c *UnitConfig) decodeRun(evalCtx *hcl.EvalContext) (*RunBodies, hcl.Diagnostics) {
	var (
		r     RunBodies
		diags hcl.Diagnostics
	)

	if c.Catalog != nil && c.Catalog.readsDependency() {
		diags = append(diags, decodeInto(evalCtx, unmarkedBody{c.Catalog.Remain}, &r.Catalog)...)
	}

	if c.Exclude != nil && c.Exclude.readsDependency() {
		diags = append(diags, decodeInto(evalCtx, unmarkedBody{c.Exclude.Remain}, &r.Exclude)...)
	}

	if c.terraformRunPart {
		diags = append(diags, decodeInto(evalCtx, c.Terraform.Remain, &r.Terraform)...)
	}

	if c.errorsRunPart {
		diags = append(diags, c.Errors.decode(evalCtx, &r.Errors)...)
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
	diags := hcldecode.DecodeBody(remain, evalCtx, body)
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
		diags = append(diags, hcldecode.DecodeBody(remain(&items[i]), evalCtx, &bodies[i])...)
	}

	*target = bodies

	return diags
}

// readsDependency reports whether an attribute of c reads the `dependency` variable.
func (c *CatalogHCL) readsDependency() bool {
	return bodyReadsDependency[CatalogBody](c.Remain)
}

// readsDependency reports whether an attribute of e reads the `dependency` variable.
func (e *ExcludeHCL) readsDependency() bool {
	return bodyReadsDependency[ExcludeBody](e.Remain)
}

// readsDependency reports whether an attribute of the body of t reads the `dependency` variable.
func (t *TerraformHCL) readsDependency() bool {
	return bodyReadsDependency[TerraformBody](t.Remain)
}

// readsDependency reports whether an attribute of a retry or ignore block of e reads the `dependency` variable.
func (e *ErrorsHCL) readsDependency() bool {
	return readsDependencyIn[RetryBody](e.Retry, (*RetryHCL).remain) ||
		readsDependencyIn[IgnoreBody](e.Ignore, (*IgnoreHCL).remain)
}

// withRequestedRunParts returns a copy of c with only the run parts that a section of decodeList decodes and that
// read no `dependency`. It keeps the engine block, which the queue
// decodes with `dependency` unknown whatever the decode list.
func (c *UnitConfig) withRequestedRunParts(decodeList []PartialDecodeSectionType) *UnitConfig {
	requested := *c
	requested.Inputs = nil
	requested.GenerateAttrs = nil
	requested.GenerateBlocks = nil
	requested.VariablesBlocks = nil
	requested.runAttrs = nil
	requested.terraformRunPart = false
	requested.errorsRunPart = false

	if c.Catalog != nil && c.Catalog.readsDependency() {
		requested.Catalog = nil
	}

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

	if wholeTerraform && c.terraformRunPart {
		parts = append(parts, "terraform attributes")
	}

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

// exposedRunSections returns [runSections] when an expression of body may read the remote_state or terraform of the
// exposed include name, and none otherwise. It may read them through include.<name>.remote_state,
// include.<name>.terraform, or include or include.<name> as a whole. A body whose expressions fail to list may read
// them.
func exposedRunSections(body hcl.Body, name string) []PartialDecodeSectionType {
	traversals, diags := bodyTraversals(body)
	if diags.HasErrors() || slices.ContainsFunc(traversals, func(traversal hcl.Traversal) bool {
		return readsExposedRunParts(traversal, name)
	}) {
		return runSections
	}

	return nil
}

// readsExposedRunParts reports whether traversal may read the remote_state or terraform of the exposed include name.
func readsExposedRunParts(traversal hcl.Traversal, name string) bool {
	if traversal.RootName() != MetadataInclude {
		return false
	}

	label, ok := keyStep(traversal[1:])
	if !ok {
		return true
	}

	if label.name != name {
		return false
	}

	part, ok := keyStep(label.rest)

	return !ok || part.name == MetadataRemoteState || part.name == MetadataTerraform
}

// bodyTraversals returns the variable traversals of every expression in body. A body of HCL JSON gives those of
// each of its properties.
//
// Returns the diagnostics of a body of HCL JSON whose properties fail to list.
func bodyTraversals(body hcl.Body) ([]hcl.Traversal, hcl.Diagnostics) {
	var traversals []hcl.Traversal

	syntax, ok := body.(*hclsyntax.Body)
	if !ok {
		attrs, diags := body.JustAttributes()
		for _, attr := range attrs {
			traversals = append(traversals, attr.Expr.Variables()...)
		}

		return traversals, diags
	}

	walkNodes(syntax, func(node hclsyntax.Node) {
		if expr, ok := node.(*hclsyntax.ScopeTraversalExpr); ok {
			traversals = append(traversals, expr.Traversal)
		}
	})

	return traversals, nil
}

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
// A body whose attributes fail to list reads none, and its decode reports why.
func bodyReadsDependency[B any](remain hcl.Body) bool {
	content, _, diags := remain.PartialContent(attributesSchema[B]())
	if diags.HasErrors() {
		return false
	}

	for _, attr := range content.Attributes {
		if attrReadsDependency(attr) {
			return true
		}
	}

	return false
}

var (
	// attributesSchemasMu guards attributesSchemas.
	attributesSchemasMu sync.RWMutex
	// attributesSchemas holds the schema [attributesSchema] returns for each type.
	attributesSchemas = map[reflect.Type]*hcl.BodySchema{}
)

// attributesSchema returns a schema of the attributes of the body B decodes, none of them required. The schema is
// shared and must not be changed.
func attributesSchema[B any]() *hcl.BodySchema {
	ty := reflect.TypeFor[B]()
	if schema, ok := cachedAttributesSchema(ty); ok {
		return schema
	}

	implied, _ := hcldecode.ImpliedBodySchema(new(B))

	schema := &hcl.BodySchema{Attributes: make([]hcl.AttributeSchema, 0, len(implied.Attributes))}
	for _, attr := range implied.Attributes {
		schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: attr.Name})
	}

	attributesSchemasMu.Lock()
	defer attributesSchemasMu.Unlock()

	if stored, ok := attributesSchemas[ty]; ok {
		return stored
	}

	attributesSchemas[ty] = schema

	return schema
}

// cachedAttributesSchema returns the schema [attributesSchema] built for ty, and whether it has built one.
func cachedAttributesSchema(ty reflect.Type) (*hcl.BodySchema, bool) {
	attributesSchemasMu.RLock()
	defer attributesSchemasMu.RUnlock()

	schema, ok := attributesSchemas[ty]

	return schema, ok
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
