package config

import (
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/gruntwork-io/terragrunt/options"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// parseHcl uses the HCL2 parser to parse the given string into an HCL file body.
func parseHcl(parser *hclparse.Parser, hcl string, filename string) (file *hcl.File, err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: filename})
		}
	}()

	if filepath.Ext(filename) == ".json" {
		file, parseDiagnostics := parser.ParseJSON([]byte(hcl), filename)
		if parseDiagnostics != nil && parseDiagnostics.HasErrors() {
			return nil, parseDiagnostics
		}

		return file, nil
	}

	file, parseDiagnostics := parser.ParseHCL([]byte(hcl), filename)
	if parseDiagnostics != nil && parseDiagnostics.HasErrors() {
		return nil, parseDiagnostics
	}

	return file, nil
}

// decodeHcl uses the HCL2 parser to decode the parsed HCL into the struct specified by out.
//
// Note that we take a two pass approach to support parsing include blocks without a label. Ideally we can parse include
// blocks with and without labels in a single pass, but the HCL parser is fairly restrictive when it comes to parsing
// blocks with labels, requiring the exact number of expected labels in the parsing step.  To handle this restriction,
// we first see if there are any include blocks without any labels, and if there is, we modify it in the file object to
// inject the label as "".
func decodeHcl(
	file *hcl.File,
	filename string,
	out interface{},
	terragruntOptions *options.TerragruntOptions,
	extensions EvalContextExtensions,
) (err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: filename})
		}
	}()

	// Check if we need to update the file to label any bare include blocks.
	updatedBytes, isUpdated, err := updateBareIncludeBlock(file, filename)
	if err != nil {
		return err
	}
	if isUpdated {
		// Code was updated, so we need to reparse the new updated contents. This is necessarily because the blocks
		// returned by hclparse does not support editing, and so we have to go through hclwrite, which leads to a
		// different AST representation.
		file, err = parseHcl(hclparse.NewParser(), string(updatedBytes), filename)
		if err != nil {
			return err
		}
	}

	evalContext, err := CreateTerragruntEvalContext(filename, terragruntOptions, extensions)
	if err != nil {
		return err
	}

	decodeDiagnostics := gohcl.DecodeBody(file.Body, evalContext, out)
	if decodeDiagnostics != nil && decodeDiagnostics.HasErrors() {
		return decodeDiagnostics
	}

	return nil
}

// ParseAndDecodeVarFile uses the HCL2 parser to parse the given varfile string into an HCL file body, and then decode it
// into the provided output.
func ParseAndDecodeVarFile(hclContents string, filename string, out interface{}) (err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: filename})
		}
	}()

	parser := hclparse.NewParser()
	file, err := parseHcl(parser, hclContents, filename)
	if err != nil {
		return err
	}

	// VarFiles should only have attributes, so extract the attributes and decode the expressions into the return map.
	attrs, hclDiags := file.Body.JustAttributes()
	if hclDiags != nil && hclDiags.HasErrors() {
		return hclDiags
	}

	valMap := map[string]cty.Value{}
	for name, attr := range attrs {
		val, hclDiags := attr.Expr.Value(nil) // nil because no function calls or variable references are allowed here
		if hclDiags != nil && hclDiags.HasErrors() {
			return hclDiags
		}
		valMap[name] = val
	}

	ctyVal, err := convertValuesMapToCtyVal(valMap)
	if err != nil {
		return err
	}

	typedOut, hasType := out.(*map[string]interface{})
	if hasType {
		genericMap, err := parseCtyValueToMap(ctyVal)
		if err != nil {
			return err
		}
		*typedOut = genericMap
		return nil
	}
	return gocty.FromCtyValue(ctyVal, out)
}
