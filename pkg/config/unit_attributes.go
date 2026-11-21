package config

import (
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
)

// The top-level attributes that [UnitConfig.ToV1] decodes after it resolves the dependency outputs, so each
// can read `dependency`. Discovery never reads them.
const (
	attrDownloadDir              = "download_dir"
	attrIamAssumeRoleDuration    = "iam_assume_role_duration"
	attrIamAssumeRoleSessionName = "iam_assume_role_session_name"
)

// runAttributeNames is the attributes that are always run parts.
var runAttributeNames = []string{
	attrDownloadDir,
	attrIamAssumeRoleDuration,
	attrIamAssumeRoleSessionName,
}

// versionAttributeNames is the top-level attributes Terragrunt reads to check versions before any dependency has
// run.
var versionAttributeNames = []string{
	MetadataTerraformBinary,
	MetadataTerraformVersionConstraint,
	MetadataTerragruntVersionConstraint,
}

// runAttributes returns the attributes of body among runAttributeNames, by name. It returns none for a body whose
// content has errors, which the decode of the whole body reports.
func runAttributes(body hcl.Body) map[string]*hcl.Attribute {
	return attributesNamed(body, runAttributeNames)
}

// dependencyVersionAttributes returns the attributes of body among versionAttributeNames that read `dependency`, by
// name, which [ValidateVersionAttributeReferences] reports. It returns none for a body whose content has
// errors, which the decode of the whole body reports.
func dependencyVersionAttributes(body hcl.Body) map[string]*hcl.Attribute {
	attrs := attributesNamed(body, versionAttributeNames)
	maps.DeleteFunc(attrs, func(_ string, attr *hcl.Attribute) bool {
		return !slices.ContainsFunc(attr.Expr.Variables(), func(traversal hcl.Traversal) bool {
			return traversal.RootName() == MetadataDependency
		})
	})

	return attrs
}

// attributesNamed returns the top-level attributes of body among names, by name, and none for a body whose content
// has errors.
func attributesNamed(body hcl.Body, names []string) map[string]*hcl.Attribute {
	schema := &hcl.BodySchema{Attributes: make([]hcl.AttributeSchema, 0, len(names))}
	for _, name := range names {
		schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: name})
	}

	content, _, diags := body.PartialContent(schema)
	if diags.HasErrors() {
		return nil
	}

	return content.Attributes
}

// withoutAttributes is a body whose content leaves out the attributes in hidden.
type withoutAttributes struct {
	hcl.Body

	hidden map[string]*hcl.Attribute
}

func (b withoutAttributes) Content(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Diagnostics) {
	content, diags := b.Body.Content(schema)
	b.hide(content.Attributes)

	return content, diags
}

func (b withoutAttributes) PartialContent(schema *hcl.BodySchema) (*hcl.BodyContent, hcl.Body, hcl.Diagnostics) {
	content, remain, diags := b.Body.PartialContent(schema)
	b.hide(content.Attributes)

	return content, withoutAttributes{Body: remain, hidden: b.hidden}, diags
}

func (b withoutAttributes) JustAttributes() (hcl.Attributes, hcl.Diagnostics) {
	attrs, diags := b.Body.JustAttributes()
	b.hide(attrs)

	return attrs, diags
}

func (b withoutAttributes) hide(attrs hcl.Attributes) {
	for name := range b.hidden {
		delete(attrs, name)
	}
}

// decodeRunAttributes decodes the run attributes of c into r with evalCtx.
func (c *UnitConfig) decodeRunAttributes(evalCtx *hcl.EvalContext, r *RunBodies) hcl.Diagnostics {
	var diags hcl.Diagnostics

	targets := map[string]any{
		attrDownloadDir:              &r.DownloadDir,
		attrIamAssumeRoleDuration:    &r.IamAssumeRoleDuration,
		attrIamAssumeRoleSessionName: &r.IamAssumeRoleSessionName,
	}

	for _, name := range runAttributeNames {
		attr, ok := c.runAttrs[name]
		if !ok {
			continue
		}

		diags = append(diags, gohcl.DecodeExpression(attr.Expr, evalCtx, targets[name])...)
	}

	return diags
}

// fillRunAttributes sets on file the run attributes of c, as r holds them.
func (c *UnitConfig) fillRunAttributes(r *RunBodies, file *TerragruntConfigFile) {
	for name := range c.runAttrs {
		switch name {
		case attrDownloadDir:
			file.DownloadDir = r.DownloadDir
		case attrIamAssumeRoleDuration:
			file.IamAssumeRoleDuration = r.IamAssumeRoleDuration
		case attrIamAssumeRoleSessionName:
			file.IamAssumeRoleSessionName = r.IamAssumeRoleSessionName
		}
	}
}
