package config

import (
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"

	pkgconfig "github.com/gruntwork-io/terragrunt/pkg/config"
)

// The top-level attributes that [pkgconfig.ParseConfig] decodes after it resolves the dependency outputs, so each
// can read `dependency`.
const (
	attrDownloadDir              = "download_dir"
	attrIamAssumeRoleDuration    = "iam_assume_role_duration"
	attrIamAssumeRoleSessionName = "iam_assume_role_session_name"
)

// runAttributeNames is the attributes that become a run part when they read `dependency`.
var runAttributeNames = []string{
	attrDownloadDir,
	attrIamAssumeRoleDuration,
	attrIamAssumeRoleSessionName,
}

// dependencyAttributes returns the attributes of body among runAttributeNames that read `dependency`, by name. It
// returns none for a body whose content has errors, which the decode of the whole body reports.
func dependencyAttributes(body hcl.Body) map[string]*hcl.Attribute {
	schema := &hcl.BodySchema{Attributes: make([]hcl.AttributeSchema, 0, len(runAttributeNames))}
	for _, name := range runAttributeNames {
		schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: name})
	}

	content, _, diags := body.PartialContent(schema)
	if diags.HasErrors() {
		return nil
	}

	reading := map[string]*hcl.Attribute{}

	for name, attr := range content.Attributes {
		readsDependency := slices.ContainsFunc(attr.Expr.Variables(), func(traversal hcl.Traversal) bool {
			return traversal.RootName() == pkgconfig.MetadataDependency
		})
		if readsDependency {
			reading[name] = attr
		}
	}

	return reading
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

// decodeRunAttributes decodes the attributes of c that read `dependency` into r with evalCtx.
func (c *UnitConfig) decodeRunAttributes(evalCtx *hcl.EvalContext, r *RunBodies) hcl.Diagnostics {
	var diags hcl.Diagnostics

	targets := map[string]any{
		attrDownloadDir:              &r.DownloadDir,
		attrIamAssumeRoleDuration:    &r.IamAssumeRoleDuration,
		attrIamAssumeRoleSessionName: &r.IamAssumeRoleSessionName,
	}

	for _, name := range runAttributeNames {
		attr, ok := c.dependencyAttrs[name]
		if !ok {
			continue
		}

		diags = append(diags, gohcl.DecodeExpression(attr.Expr, evalCtx, targets[name])...)
	}

	return diags
}

// fillRunAttributes sets on file the attributes of c that read `dependency`, as r holds them.
func (c *UnitConfig) fillRunAttributes(r *RunBodies, file *pkgconfig.TerragruntConfigFile) {
	for name := range c.dependencyAttrs {
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
