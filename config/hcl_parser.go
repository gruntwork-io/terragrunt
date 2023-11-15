package config

import (
	"encoding/json"
	"path/filepath"

	"github.com/gruntwork-io/go-commons/errors"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// parseHcl uses the HCL2 parser to parse the given string into an HCL file body.
func (config *terragruntConfigFile) parseHcl() (err error) {
	config.parser = hclparse.NewParser()

	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: config.configPath})
		}
	}()

	var parseDiagnostics hcl.Diagnostics

	if filepath.Ext(config.configPath) == ".json" {
		config.file, parseDiagnostics = config.parser.ParseJSON([]byte(config.fileContents), config.configPath)
		if parseDiagnostics != nil && parseDiagnostics.HasErrors() {
			return parseDiagnostics
		}

		return nil
	}

	config.file, parseDiagnostics = config.parser.ParseHCL([]byte(config.fileContents), config.configPath)
	if parseDiagnostics != nil && parseDiagnostics.HasErrors() {
		return parseDiagnostics
	}

	return nil
}

func (config *terragruntConfigFile) ParseJson(variables any) error {
	if err := json.Unmarshal([]byte(config.fileContents), &variables); err != nil {
		return errors.Errorf("could not unmarshal json body of tfvar file: %w", err)
	}

	return nil
}

// decodeHcl uses the HCL2 parser to decode the parsed HCL into the struct specified by out.
//
// Note that we take a two pass approach to support parsing include blocks without a label. Ideally we can parse include
// blocks with and without labels in a single pass, but the HCL parser is fairly restrictive when it comes to parsing
// blocks with labels, requiring the exact number of expected labels in the parsing step.  To handle this restriction,
// we first see if there are any include blocks without any labels, and if there is, we modify it in the file object to
// inject the label as "".
func (terragruntConfigFile *terragruntConfigFile) decodeHcl(out interface{}, extensions EvalContextExtensions) (err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: terragruntConfigFile.configPath})
		}
	}()

	// Check if we need to update the file to label any bare include blocks.
	updatedBytes, isUpdated, err := updateBareIncludeBlock(terragruntConfigFile.file, terragruntConfigFile.configPath)
	if err != nil {
		return err
	}
	if isUpdated {
		terragruntConfigFile.fileContents = string(updatedBytes)
		// Code was updated, so we need to reparse the new updated contents. This is necessarily because the blocks
		// returned by hclparse does not support editing, and so we have to go through hclwrite, which leads to a
		// different AST representation.

		if err = terragruntConfigFile.parseHcl(); err != nil {
			return err
		}
	}

	evalContext, err := extensions.CreateTerragruntEvalContext(terragruntConfigFile.configPath, terragruntConfigFile.terragruntOptions)
	if err != nil {
		return err
	}

	delete(evalContext.Functions, "get_working_dir")

	decodeDiagnostics := gohcl.DecodeBody(terragruntConfigFile.file.Body, evalContext, out)
	if decodeDiagnostics != nil && decodeDiagnostics.HasErrors() {
		return decodeDiagnostics
	}

	return nil
}

// ParseAndDecodeVarFile uses the HCL2 parser to parse the given varfile string into an HCL file body, and then decode it
// into the provided output.
func ParseAndDecodeVarFile(terragruntConfigFile *terragruntConfigFile, out interface{}) (err error) {
	// The HCL2 parser and especially cty conversions will panic in many types of errors, so we have to recover from
	// those panics here and convert them to normal errors
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.WithStackTrace(PanicWhileParsingConfig{RecoveredValue: recovered, ConfigFile: terragruntConfigFile.configPath})
		}
	}()

	if err := terragruntConfigFile.parseHcl(); err != nil {
		return err
	}

	// VarFiles should only have attributes, so extract the attributes and decode the expressions into the return map.
	attrs, hclDiags := terragruntConfigFile.file.Body.JustAttributes()
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
