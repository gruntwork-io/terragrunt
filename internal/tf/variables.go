package tf

import (
	"cmp"
	"errors"
	"path/filepath"
	"slices"
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
	// VariableParseUnknown means the module's declaration of the variable was not read, so
	// either mode may apply.
	VariableParseUnknown VariableParsingMode = iota
	// VariableParseLiteral means the value is taken as-is, with no HCL parsing.
	VariableParseLiteral
	// VariableParseHCL means the value is parsed as an HCL expression, so template sequences
	// like ${...} and %{...} have to be escaped to reach the module as literal text.
	VariableParseHCL
)

// ModuleVariable holds what a module's variable blocks say about the values it accepts.
//
// The zero value describes a variable the module does not declare. OpenTofu/Terraform ignore the
// environment variable that would set it.
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
// A variable declared with a `string`, `number` or `bool` type constraint, and a variable declared
// with no type constraint at all, is read literally. Every other type constraint makes
// OpenTofu/Terraform parse the value as an HCL expression.
//
// A variable block in an override file (override.tf, or a name ending in _override.tf, in any of
// the configuration file extensions) replaces only the arguments it sets. The other arguments
// keep the value from the block it overrides.
func ModuleVariables(fsys vfs.FS, modulePath string) (map[string]ModuleVariable, error) {
	files, err := util.ListTfFiles(fsys, modulePath)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(files, compareLoadOrder)

	hclFiles, err := readModuleFiles(fsys, files)
	if err != nil {
		return nil, err
	}

	variables := map[string]ModuleVariable{}

	for i, file := range hclFiles {
		path := files[i]

		content, _, diags := file.Body.PartialContent(variableBlockSchema)
		if diags.HasErrors() {
			return nil, diags
		}

		for _, block := range content.Blocks {
			name := block.Labels[0]

			base := ModuleVariable{ParsingMode: VariableParseLiteral}
			if isOverrideFile(path) {
				base = variables[name]
			}

			variable, err := mergeVariableBlock(base, block)
			if err != nil {
				return nil, err
			}

			variables[name] = variable
		}
	}

	return variables, nil
}

// compareLoadOrder orders configuration files the way OpenTofu/Terraform load them: override files
// after every other file, and by name within each group.
func compareLoadOrder(a, b string) int {
	return cmp.Or(
		cmp.Compare(loadGroup(a), loadGroup(b)),
		strings.Compare(a, b),
	)
}

func loadGroup(path string) int {
	if isOverrideFile(path) {
		return 1
	}

	return 0
}

func isOverrideFile(path string) bool {
	name := strings.TrimSuffix(filepath.Base(path), ".json")
	name = strings.TrimSuffix(name, filepath.Ext(name))

	return name == "override" || strings.HasSuffix(name, "_override")
}

// parseModuleFiles parses the OpenTofu/Terraform files at paths into one body.
func parseModuleFiles(fsys vfs.FS, paths []string) (hcl.Body, error) {
	hclFiles, err := readModuleFiles(fsys, paths)
	if err != nil {
		return nil, err
	}

	return hcl.MergeFiles(hclFiles), nil
}

// readModuleFiles parses the OpenTofu/Terraform files at paths, returning one file per path in
// the same order. It reports the failures of every file together.
func readModuleFiles(fsys vfs.FS, paths []string) ([]*hcl.File, error) {
	parser := hclparse.NewParser()
	hclFiles := make([]*hcl.File, 0, len(paths))
	errs := make([]error, 0, len(paths))

	var diags hcl.Diagnostics

	for _, path := range paths {
		src, err := vfs.ReadFile(fsys, path)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		parseFunc := parser.ParseHCL
		if strings.HasSuffix(path, ".json") {
			parseFunc = parser.ParseJSON
		}

		file, fileDiags := parseFunc(src, path)
		if fileDiags.HasErrors() {
			diags = append(diags, fileDiags...)

			continue
		}

		hclFiles = append(hclFiles, file)
	}

	if diags.HasErrors() {
		errs = append(errs, diags)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return hclFiles, nil
}

// mergeVariableBlock returns base with the arguments that block sets applied over it.
func mergeVariableBlock(base ModuleVariable, block *hcl.Block) (ModuleVariable, error) {
	attrs, _, diags := block.Body.PartialContent(variableAttributesSchema)
	if diags.HasErrors() {
		return ModuleVariable{}, diags
	}

	if typeAttr, ok := attrs.Attributes["type"]; ok {
		base.ParsingMode = typeParsingMode(typeAttr.Expr)
	}

	if _, ok := attrs.Attributes["default"]; ok {
		base.HasDefault = true
	}

	return base, nil
}

func typeParsingMode(typeExpr hcl.Expression) VariableParsingMode {
	switch hcl.ExprAsKeyword(typeExpr) {
	case "string", "number", "bool":
		return VariableParseLiteral
	}

	return VariableParseHCL
}
