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

// ValidateInputDefinitions reports a diagnostic for each input that file defines more than once, in the inputs attribute
// or in variables blocks. Each diagnostic points at the key of the later definition.
//
// cfg is the config parsed from file alone, before any include or autoinclude merges into it.
func ValidateInputDefinitions(file *hclparse.File, cfg *TerragruntConfig) error {
	var errs []error

	for _, duplicate := range cfg.duplicateInputs() {
		if err := reportDuplicateInput(file, duplicate); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// inputDefinition is the position of a variables block among the blocks of a file, or inputsAttribute.
type inputDefinition int

// inputsAttribute is the [inputDefinition] of the inputs attribute.
const inputsAttribute inputDefinition = -1

// duplicateInput is an input that a file defines twice, in source order.
type duplicateInput struct {
	name   string
	first  inputDefinition
	second inputDefinition
}

// duplicateInputs returns every input cfg defines more than once, in block order and then by name.
func (cfg *TerragruntConfig) duplicateInputs() []duplicateInput {
	if len(cfg.Variables) == 0 {
		return nil
	}

	defined := make(map[string]inputDefinition, len(cfg.Inputs))

	for name := range cfg.Inputs {
		defined[name] = inputsAttribute
	}

	var duplicates []duplicateInput

	for _, block := range cfg.Variables {
		definition := inputDefinition(block.BlockIndex)

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

// reportDuplicateInput raises the diagnostic for duplicate through file.
func reportDuplicateInput(file *hclparse.File, duplicate duplicateInput) error {
	first, err := inputKeyRange(file, duplicate.first, duplicate.name)
	if err != nil {
		return err
	}

	second, err := inputKeyRange(file, duplicate.second, duplicate.name)
	if err != nil {
		return err
	}

	return file.HandleDiagnostics(hcl.Diagnostics{{
		Severity: hcl.DiagError,
		Summary:  "Duplicate input",
		Detail:   fmt.Sprintf("The input %q was already set at %s. A file may set each input only once.", duplicate.name, first),
		Subject:  second.Ptr(),
	}})
}

// inputKeyRange returns the range of the key that sets name in definition. It is the range of the whole inputs
// expression when the expression is computed, such as a merge call, and has no such key.
func inputKeyRange(file *hclparse.File, definition inputDefinition, name string) (hcl.Range, error) {
	body := file.Body

	if definition != inputsAttribute {
		blocks, err := file.Blocks(MetadataVariables, true)
		if err != nil {
			return hcl.Range{}, err
		}

		body = blocks[definition].Body
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

// mergeVariables appends blocks to cfg and removes from cfg every input that inputs or blocks define, leaving each input
// with one definition. A block in both is kept once, so a file reached through two includes does not override itself.
func (cfg *TerragruntConfig) mergeVariables(inputs map[string]any, blocks []VariablesConfig) {
	defined := make(map[string]struct{})

	for _, block := range blocks {
		for name := range block.Inputs {
			defined[name] = struct{}{}
		}
	}

	cfg.Inputs = withoutInputs(cfg.Inputs, defined)

	for name := range inputs {
		defined[name] = struct{}{}
	}

	earlier := cfg.variablesOutside(blocks)

	for i := range earlier {
		earlier[i].Inputs = withoutInputs(earlier[i].Inputs, defined)
	}

	cfg.Variables = slices.Concat(earlier, blocks)
}

// deepMergedDefinitions returns the inputs attribute and variables blocks of source, with the value of each input that
// cfg also defines deep merged over the value in cfg.
func (cfg *TerragruntConfig) deepMergedDefinitions(
	source *TerragruntConfig,
) (map[string]any, []VariablesConfig, error) {
	earlier := make(map[string]any, len(cfg.Inputs))
	maps.Copy(earlier, cfg.Inputs)

	for _, block := range cfg.variablesOutside(source.Variables) {
		maps.Copy(earlier, block.Inputs)
	}

	inputs, err := deepMergeDefined(source.Inputs, earlier)
	if err != nil {
		return nil, nil, err
	}

	blocks := slices.Clone(source.Variables)

	for i := range blocks {
		merged, err := deepMergeDefined(blocks[i].Inputs, earlier)
		if err != nil {
			return nil, nil, err
		}

		blocks[i].Inputs = merged
	}

	return inputs, blocks, nil
}

// variablesOutside returns a copy of the variables blocks of cfg without any block that blocks also holds.
func (cfg *TerragruntConfig) variablesOutside(blocks []VariablesConfig) []VariablesConfig {
	return slices.DeleteFunc(slices.Clone(cfg.Variables), func(block VariablesConfig) bool {
		return slices.ContainsFunc(blocks, func(other VariablesConfig) bool {
			return other.ConfigPath == block.ConfigPath && other.BlockIndex == block.BlockIndex
		})
	})
}

// deepMergeDefined returns inputs with the value of each input that earlier also defines deep merged over the value in
// earlier. It adds no other inputs from earlier.
func deepMergeDefined(inputs, earlier map[string]any) (map[string]any, error) {
	defined := make(map[string]any, len(inputs))

	for name := range inputs {
		if value, found := earlier[name]; found {
			defined[name] = value
		}
	}

	return deepMergeInputs(inputs, defined)
}

// withoutInputs returns a copy of inputs that leaves out names.
func withoutInputs(inputs map[string]any, names map[string]struct{}) map[string]any {
	kept := maps.Clone(inputs)

	maps.DeleteFunc(kept, func(name string, _ any) bool {
		_, found := names[name]
		return found
	})

	return kept
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
