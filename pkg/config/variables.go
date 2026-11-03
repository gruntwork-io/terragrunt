package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/ctyhelper"
	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/internal/venv"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

// VariablesKind is how a variables block passes its inputs to OpenTofu/Terraform.
type VariablesKind string

const (
	// VariablesKindEnv passes inputs as TF_VAR_ environment variables.
	VariablesKindEnv VariablesKind = "env"
	// VariablesKindTFVar writes inputs to a terragrunt.auto.tfvars file in the working directory.
	VariablesKindTFVar VariablesKind = "tfvar"
	// VariablesKindJSON writes inputs to a terragrunt.auto.tfvars.json file in the working directory.
	VariablesKindJSON VariablesKind = "json"
)

// VariablesBlock is a variables block as decoded from a config file.
type VariablesBlock struct {
	Kind     *string   `hcl:"kind,attr"`
	Nullable *bool     `hcl:"nullable,attr"`
	Validate *bool     `hcl:"validate,attr"`
	Inputs   cty.Value `hcl:"inputs,attr"`
}

// VariablesConfig is a parsed variables block.
type VariablesConfig struct {
	// Inputs maps variable names to values.
	Inputs map[string]any
	// Kind is how the inputs reach OpenTofu/Terraform.
	Kind VariablesKind
	// ConfigPath is the config file that sets the block.
	ConfigPath string
	// BlockIndex is the position of the block among the variables blocks of the file at ConfigPath.
	BlockIndex int
	// Nullable keeps null values in Inputs, so they set variables to null instead of leaving them unset.
	Nullable bool
	// Validate requires the module to declare every variable named in Inputs.
	Validate bool
}

// ResolvedInput is the value Terragrunt passes to one OpenTofu/Terraform variable.
type ResolvedInput struct {
	// Value is the variable value. Nil sets the variable to null.
	Value any
	// Kind is how the value reaches OpenTofu/Terraform.
	Kind VariablesKind
	// Validate requires the module to declare the variable.
	Validate bool
}

// ResolveInputs returns every input Terragrunt passes to OpenTofu/Terraform, keyed by variable name. Inputs from the
// inputs attribute are env inputs that keep null values.
func (cfg *TerragruntConfig) ResolveInputs() map[string]ResolvedInput {
	resolved := make(map[string]ResolvedInput, len(cfg.Inputs))

	for name, value := range cfg.Inputs {
		resolved[name] = ResolvedInput{Value: value, Kind: VariablesKindEnv}
	}

	for _, block := range cfg.Variables {
		for name, value := range block.Inputs {
			resolved[name] = ResolvedInput{Value: value, Kind: block.Kind, Validate: block.Validate}
		}
	}

	return resolved
}

// ValidateInputDefinitions reports a diagnostic for each input cfg defines more than once, in the inputs attribute or
// in variables blocks. Values of the inputs attribute merged through an include count as one definition.
//
// file is the file cfg was parsed from. Each diagnostic points at the key of the later definition and goes through the
// file that holds it, so an include or autoinclude that holds the key renders its own source.
func ValidateInputDefinitions(v *venv.Venv, file *hclparse.File, cfg *TerragruntConfig) error {
	var errs []error

	for _, duplicate := range cfg.duplicateInputs() {
		if err := reportDuplicateInput(v, file, duplicate); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// inputsAttribute is the [inputDefinition.blockIndex] of a definition in the inputs attribute.
const inputsAttribute = -1

// inputDefinition is a place in a config file that defines inputs.
type inputDefinition struct {
	configPath string
	// blockIndex is the position of the variables block among the file's blocks, or inputsAttribute.
	blockIndex int
}

// duplicateInput is an input defined in two places, in merge order.
type duplicateInput struct {
	name   string
	first  inputDefinition
	second inputDefinition
}

// duplicateInputs returns every input defined more than once, in merge order and then by name.
func (cfg *TerragruntConfig) duplicateInputs() []duplicateInput {
	if len(cfg.Variables) == 0 {
		return nil
	}

	defined := make(map[string]inputDefinition, len(cfg.Inputs))

	for name := range cfg.Inputs {
		metadata, _ := cfg.GetMapFieldMetadata(MetadataInputs, name)
		defined[name] = inputDefinition{configPath: metadata[FoundInFile], blockIndex: inputsAttribute}
	}

	var duplicates []duplicateInput

	for _, block := range cfg.Variables {
		definition := inputDefinition{configPath: block.ConfigPath, blockIndex: block.BlockIndex}

		for _, name := range slices.Sorted(maps.Keys(block.Inputs)) {
			if first, found := defined[name]; found {
				duplicates = append(duplicates, duplicateInput{name: name, first: first, second: definition})

				continue
			}

			defined[name] = definition
		}
	}

	return duplicates
}

// reportDuplicateInput raises the diagnostic for duplicate through the file that holds its later definition.
func reportDuplicateInput(v *venv.Venv, file *hclparse.File, duplicate duplicateInput) error {
	firstFile, err := definitionFile(v, file, duplicate.first)
	if err != nil {
		return err
	}

	secondFile, err := definitionFile(v, file, duplicate.second)
	if err != nil {
		return err
	}

	first, err := inputKeyRange(firstFile, duplicate.first, duplicate.name)
	if err != nil {
		return err
	}

	second, err := inputKeyRange(secondFile, duplicate.second, duplicate.name)
	if err != nil {
		return err
	}

	return secondFile.HandleDiagnostics(hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  "Duplicate input",
		Detail:   fmt.Sprintf("The input %q was already set at %s. Each input may be set only once.", duplicate.name, first),
		Subject:  second.Ptr(),
	}})
}

// definitionFile returns the parsed file that holds definition. A file other than file is read into file's parser,
// whose diagnostics writer renders only source it has parsed.
func definitionFile(v *venv.Venv, file *hclparse.File, definition inputDefinition) (*hclparse.File, error) {
	if definition.configPath == file.ConfigPath {
		return file, nil
	}

	return file.Parser.ParseFromFile(v.FS, definition.configPath)
}

// inputKeyRange returns the range of the key that sets name in definition. It is the range of the whole inputs
// expression when the expression is computed, such as a merge call, and has no such key.
func inputKeyRange(file *hclparse.File, definition inputDefinition, name string) (hcl.Range, error) {
	body := file.Body

	if definition.blockIndex != inputsAttribute {
		blocks, err := file.Blocks(MetadataVariables, true)
		if err != nil {
			return hcl.Range{}, err
		}

		body = blocks[definition.blockIndex].Body
	}

	schema := &hcl.BodySchema{Attributes: []hcl.AttributeSchema{{Name: MetadataInputs}}}

	content, _, diags := body.PartialContent(schema)
	if diags.HasErrors() {
		return hcl.Range{}, diags
	}

	expr := content.Attributes[MetadataInputs].Expr

	items, diags := hcl.ExprMap(expr)
	if diags.HasErrors() {
		return expr.Range(), nil
	}

	for _, item := range items {
		key, diags := item.Key.Value(nil)
		if diags.HasErrors() || !key.Type().Equals(cty.String) || key.IsNull() || !key.IsKnown() {
			continue
		}

		if key.AsString() == name {
			return item.Key.Range(), nil
		}
	}

	return expr.Range(), nil
}

// mergeVariablesBlocks appends the blocks of source to blocks, skipping any block already present. A file reached
// through two includes contributes its blocks once.
func mergeVariablesBlocks(blocks, source []VariablesConfig) []VariablesConfig {
	merged := slices.Clone(blocks)

	for _, block := range source {
		present := slices.ContainsFunc(merged, func(other VariablesConfig) bool {
			return other.ConfigPath == block.ConfigPath && other.BlockIndex == block.BlockIndex
		})
		if !present {
			merged = append(merged, block)
		}
	}

	return merged
}

// convertVariablesBlocks parses the variables blocks of the config file at configPath.
//
// Returns [VariablesBlockRequiresExperimentError] when there are blocks and the variables-block experiment is off,
// [InvalidVariablesKindError] when a block sets a kind other than env, tfvar, or json, and
// [InvalidVariablesInputsError] when a block sets inputs to something other than a map.
func convertVariablesBlocks(
	pctx *ParsingContext,
	configPath string,
	blocks []VariablesBlock,
) ([]VariablesConfig, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	if !pctx.Experiments.Evaluate(experiment.VariablesBlock) {
		return nil, VariablesBlockRequiresExperimentError{ConfigPath: configPath}
	}

	parsed := make([]VariablesConfig, 0, len(blocks))

	for i, block := range blocks {
		kind, err := parseVariablesKind(configPath, block.Kind)
		if err != nil {
			return nil, err
		}

		if ty := block.Inputs.Type(); !block.Inputs.IsNull() && !ty.IsObjectType() && !ty.IsMapType() {
			return nil, InvalidVariablesInputsError{ConfigPath: configPath, Type: ty.FriendlyName()}
		}

		inputs, err := ctyhelper.ParseCtyValueToMap(block.Inputs)
		if err != nil {
			return nil, err
		}

		nullable := block.Nullable != nil && *block.Nullable
		if !nullable {
			maps.DeleteFunc(inputs, func(_ string, value any) bool { return value == nil })
		}

		parsed = append(parsed, VariablesConfig{
			Inputs:     inputs,
			Kind:       kind,
			ConfigPath: configPath,
			BlockIndex: i,
			Nullable:   nullable,
			Validate:   block.Validate != nil && *block.Validate,
		})
	}

	return parsed, nil
}

// parseVariablesKind returns the kind a variables block sets, which is env when kind is nil.
func parseVariablesKind(configPath string, kind *string) (VariablesKind, error) {
	if kind == nil {
		return VariablesKindEnv, nil
	}

	switch parsed := VariablesKind(*kind); parsed {
	case VariablesKindEnv, VariablesKindTFVar, VariablesKindJSON:
		return parsed, nil
	}

	return "", InvalidVariablesKindError{ConfigPath: configPath, Kind: *kind}
}

// variablesBlocksAsCty serializes variables blocks as a tuple, since the inputs of each block have their own type.
func variablesBlocksAsCty(blocks []VariablesConfig) (cty.Value, error) {
	if len(blocks) == 0 {
		return cty.NilVal, nil
	}

	out := make([]cty.Value, 0, len(blocks))

	for _, block := range blocks {
		asCty, err := variablesBlockAsCty(block)
		if err != nil {
			return cty.NilVal, err
		}

		out = append(out, asCty)
	}

	return cty.TupleVal(out), nil
}

// variablesBlocksAsCtyWithMetadata serializes variables blocks like [variablesBlocksAsCty], with each block wrapped as
// a value and metadata naming the file that sets it.
func variablesBlocksAsCtyWithMetadata(blocks []VariablesConfig) (cty.Value, error) {
	if len(blocks) == 0 {
		return cty.NilVal, nil
	}

	out := make([]cty.Value, 0, len(blocks))

	for _, block := range blocks {
		asCty, err := variablesBlockAsCty(block)
		if err != nil {
			return cty.NilVal, err
		}

		out = append(out, cty.ObjectVal(map[string]cty.Value{
			"value":    asCty,
			"metadata": cty.ObjectVal(map[string]cty.Value{FoundInFile: cty.StringVal(block.ConfigPath)}),
		}))
	}

	return cty.TupleVal(out), nil
}

// variablesBlockAsCty serializes one variables block.
func variablesBlockAsCty(block VariablesConfig) (cty.Value, error) {
	inputs, err := convertToCtyWithJSON(block.Inputs)
	if err != nil {
		return cty.NilVal, err
	}

	return cty.ObjectVal(map[string]cty.Value{
		"kind":     cty.StringVal(string(block.Kind)),
		"nullable": cty.BoolVal(block.Nullable),
		"validate": cty.BoolVal(block.Validate),
		"inputs":   inputs,
	}), nil
}

// variablesBlock renders one variables block from its cty form, writing every attribute.
func variablesBlock(asCty cty.Value) *hclwrite.Block {
	block := hclwrite.NewBlock(MetadataVariables, nil)
	body := block.Body()

	for _, name := range []string{"kind", "nullable", "validate", "inputs"} {
		body.SetAttributeValue(name, asCty.GetAttr(name))
	}

	return block
}
