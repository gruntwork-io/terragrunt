package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/gruntwork-io/terragrunt/util"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// ParsedVariable structure with input name, default value and description.
type ParsedVariable struct {
	Name                    string
	Description             string
	Type                    string
	DefaultValue            string
	DefaultValuePlaceholder string
}

func ParseVariables(opts *options.TerragruntOptions, directoryPath string) ([]*ParsedVariable, error) {
	tfFiles, err := util.ListTfFiles(directoryPath)
	if err != nil {
		return nil, errors.WithStackTrace(err)
	}
	parser := hclparse.NewParser()

	// Extract variables from all TF files
	var parsedInputs []*ParsedVariable
	for _, tfFile := range tfFiles {
		content, err := os.ReadFile(tfFile)
		if err != nil {
			opts.Logger.Warnf("Error reading file %s: %v", tfFile, err)
			continue
		}
		file, diags := parser.ParseHCL(content, tfFile)
		if diags.HasErrors() {
			opts.Logger.Warnf("Failed to parse HCL in file %s: %v", tfFile, diags)
			continue
		}

		ctx := &hcl.EvalContext{}

		if body, ok := file.Body.(*hclsyntax.Body); ok {
			for _, block := range body.Blocks {
				if block.Type == "variable" {
					if len(block.Labels[0]) > 0 {

						name := block.Labels[0]
						descriptionAttr, err := readBlockAttribute(ctx, block, "description")
						descriptionAttrText := ""
						if err != nil {
							opts.Logger.Warnf("Failed to read descriptionAttr for %s %v", name, err)
							descriptionAttr = nil
						}
						if descriptionAttr != nil {
							descriptionAttrText = descriptionAttr.AsString()
						} else {
							descriptionAttrText = fmt.Sprintf("No description for %s", name)
						}

						typeAttr, err := readBlockAttribute(ctx, block, "type")
						typeAttrText := ""
						if err != nil {
							opts.Logger.Warnf("Failed to read type attribute for %s %v", name, err)
							descriptionAttr = nil
						}
						if typeAttr != nil {
							typeAttrText = typeAttr.AsString()
						} else {
							typeAttrText = fmt.Sprintf("No type for %s", name)
						}

						defaultValue, err := readBlockAttribute(ctx, block, "default")
						if err != nil {
							opts.Logger.Warnf("Failed to read default value for %s %v", name, err)
							defaultValue = nil
						}

						defaultValueText := ""
						if defaultValue != nil {
							jsonBytes, err := ctyjson.Marshal(*defaultValue, cty.DynamicPseudoType)
							if err != nil {
								return nil, errors.WithStackTrace(err)
							}

							var ctyJsonOutput CtyJsonValue
							if err := json.Unmarshal(jsonBytes, &ctyJsonOutput); err != nil {
								return nil, errors.WithStackTrace(err)
							}

							jsonBytes, err = json.Marshal(ctyJsonOutput.Value)
							if err != nil {
								return nil, errors.WithStackTrace(err)
							}
							defaultValueText = string(jsonBytes)
						}

						input := &ParsedVariable{
							Name:                    name,
							Type:                    typeAttrText,
							Description:             descriptionAttrText,
							DefaultValue:            defaultValueText,
							DefaultValuePlaceholder: generateDefaultValue(typeAttrText),
						}

						parsedInputs = append(parsedInputs, input)
					}
				}
			}
		}
	}
	return parsedInputs, nil
}

// generate hcl default value
func generateDefaultValue(typetxt string) string {

	switch typetxt {
	case "number":
		return "0"
	case "bool":
		return "false"
	case "list":
		return "[]"
	case "map":
		return "{}"
	case "object":
		return "{}"
	}

	// fallback to empty value
	return "\"\""
}

type CtyJsonValue struct {
	Value interface{}
	Type  interface{}
}

func readBlockAttribute(ctx *hcl.EvalContext, block *hclsyntax.Block, name string) (*cty.Value, error) {
	if attr, ok := block.Body.Attributes[name]; ok {
		if attr.Expr != nil {

			if call, ok := attr.Expr.(*hclsyntax.FunctionCallExpr); ok {
				result := cty.StringVal(call.Name)
				return &result, nil
			}

			// check if first var is traversal
			if len(attr.Expr.Variables()) > 0 {
				v := attr.Expr.Variables()[0]
				// check if variable is traversal
				if varTr, ok := v[0].(hcl.TraverseRoot); ok {
					result := cty.StringVal(varTr.Name)
					return &result, nil
				}
			}

			value, err := attr.Expr.Value(ctx)
			if err != nil {
				return nil, err
			}
			return &value, nil
		}
	}
	return nil, nil
}
