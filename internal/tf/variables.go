package tf

import (
	"strings"

	"github.com/gruntwork-io/terragrunt/internal/util"
	"github.com/gruntwork-io/terragrunt/internal/vfs"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// VariableParsingMode describes how OpenTofu/Terraform reads the value of a TF_VAR_* environment
// variable: verbatim, or as an HCL expression.
type VariableParsingMode int

const (
	// VariableParseLiteral means the value is taken as-is, with no HCL parsing.
	VariableParseLiteral VariableParsingMode = iota
	// VariableParseHCL means the value is parsed as an HCL expression, so interpolation
	// sequences like ${...} have to be escaped to reach the module as literal text.
	VariableParseHCL
)

// ModuleVariable holds what a module's variable block says about the values it accepts.
//
// The zero value describes a variable the module does not declare: OpenTofu/Terraform ignore the
// environment variable that would set it, and read any value that does reach it verbatim.
type ModuleVariable struct {
	ParsingMode VariableParsingMode
	HasDefault  bool
}

var variableBlockSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "variable",
			LabelNames: []string{"name"},
		},
	},
}

var variableAttributesSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "type"},
		{Name: "default"},
	},
}

// ModuleVariables returns every variable declared by the module at modulePath, keyed by name,
// taking into account all the generated sources.
//
// A variable declared with a `string` type constraint, and a variable declared with no type
// constraint at all, is read literally; every other type constraint makes OpenTofu/Terraform parse
// the value as an HCL expression.
func ModuleVariables(fsys vfs.FS, modulePath string) (map[string]ModuleVariable, error) {
	files, err := util.ListTfFiles(fsys, modulePath)
	if err != nil {
		return nil, err
	}

	parser := hclparse.NewParser()
	hclFiles := make([]*hcl.File, 0, len(files))

	for _, path := range files {
		src, err := vfs.ReadFile(fsys, path)
		if err != nil {
			return nil, err
		}

		parseFunc := parser.ParseHCL
		if strings.HasSuffix(path, ".json") {
			parseFunc = parser.ParseJSON
		}

		file, diags := parseFunc(src, path)
		if diags.HasErrors() {
			return nil, diags
		}

		hclFiles = append(hclFiles, file)
	}

	content, _, diags := hcl.MergeFiles(hclFiles).PartialContent(variableBlockSchema)
	if diags.HasErrors() {
		return nil, diags
	}

	variables := make(map[string]ModuleVariable, len(content.Blocks))

	for _, block := range content.Blocks {
		attrs, _, diags := block.Body.PartialContent(variableAttributesSchema)
		if diags.HasErrors() {
			return nil, diags
		}

		variable := ModuleVariable{}

		if typeAttr, ok := attrs.Attributes["type"]; ok &&
			hcl.ExprAsKeyword(typeAttr.Expr) != "string" {
			variable.ParsingMode = VariableParseHCL
		}

		_, variable.HasDefault = attrs.Attributes["default"]

		variables[block.Labels[0]] = variable
	}

	return variables, nil
}
