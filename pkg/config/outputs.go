package config

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config/hclparse"
)

const (
	// MetadataOutputs is the name of the outputs block.
	MetadataOutputs = "outputs"
	// MetadataOutput is the name of the output blocks nested in an outputs block.
	MetadataOutput = "output"
	// MetadataMockStrategy is the name of the mock_strategy block nested in an outputs block.
	MetadataMockStrategy = "mock_strategy"
)

// mockMergeStrategies is the merge strategies a mock_strategy block may name. A full deep merge has no meaning for
// mock outputs, so it is left out.
var mockMergeStrategies = []MergeStrategyType{NoMerge, ShallowMerge, DeepMergeMapOnly}

// OutputsConfig is the outputs block of a unit. It declares the outputs the unit emits, each with the mock value
// its dependents read when the unit's state cannot be read, and the strategy that says when mocks apply.
//
// The block is recognized only under the outputs-block experiment.
type OutputsConfig struct {
	// MockStrategy is the mock_strategy block, nil when the block has none.
	MockStrategy *OutputsMockStrategy `hcl:"mock_strategy,block"`
	// Outputs is the output blocks, in source order.
	Outputs []*OutputDeclaration `hcl:"output,block"`
}

// OutputsMockStrategy is the mock_strategy block of an outputs block. Its attributes mean what the
// mock_outputs_allowed_terraform_commands and mock_outputs_merge_strategy_with_state attributes of a dependency
// block mean, and a dependency block that sets those wins over them.
type OutputsMockStrategy struct {
	// AllowedCommands is the commands that may read mock values. Unset or empty allows every command.
	AllowedCommands *[]string `hcl:"allowed_commands,attr"`
	// MergeWithState is how the state of the unit is merged into the mock values: no_merge, shallow or
	// deep_map_only.
	MergeWithState *MergeStrategyType `hcl:"merge_with_state,attr"`
}

// OutputDeclaration is one output block of an outputs block.
type OutputDeclaration struct {
	// MockValue is the value dependents read in place of the output, nil when the output declares none.
	MockValue *cty.Value `hcl:"mock_value,optional"`
	// Name is the output name, the block label.
	Name string `hcl:"name,label"`
}

// Clone returns a deep copy of c.
func (c *OutputsConfig) Clone() *OutputsConfig {
	if c == nil {
		return nil
	}

	clone := &OutputsConfig{MockStrategy: c.MockStrategy.Clone()}

	if c.Outputs != nil {
		clone.Outputs = make([]*OutputDeclaration, len(c.Outputs))
		for i, output := range c.Outputs {
			clone.Outputs[i] = output.Clone()
		}
	}

	return clone
}

// Merge merges other into c, with other winning: an output both declare takes other's declaration, an output only
// other declares is appended, and each mock_strategy attribute other sets replaces c's.
func (c *OutputsConfig) Merge(other *OutputsConfig) {
	if c == nil || other == nil {
		return
	}

	if other.MockStrategy != nil {
		if c.MockStrategy == nil {
			c.MockStrategy = &OutputsMockStrategy{}
		}

		c.MockStrategy.merge(other.MockStrategy)
	}

	for _, output := range other.Outputs {
		i := slices.IndexFunc(c.Outputs, func(declared *OutputDeclaration) bool {
			return declared.Name == output.Name
		})
		if i >= 0 {
			c.Outputs[i] = output.Clone()

			continue
		}

		c.Outputs = append(c.Outputs, output.Clone())
	}
}

// Validate checks that the block may be used and is well formed: the outputs-block experiment is enabled, no
// output is declared twice, and merge_with_state names a mock merge strategy.
func (c *OutputsConfig) Validate(experiments experiment.Experiments, configPath string) error {
	if c == nil {
		return nil
	}

	if !experiments.Evaluate(experiment.OutputsBlock) {
		return OutputsBlockRequiresExperimentError{ConfigPath: configPath}
	}

	var errs []error

	seen := make(map[string]struct{}, len(c.Outputs))

	for _, output := range c.Outputs {
		if _, duplicate := seen[output.Name]; duplicate {
			errs = append(errs, DuplicateOutputDeclarationError{ConfigPath: configPath, Name: output.Name})
		}

		seen[output.Name] = struct{}{}
	}

	if strategy := c.MockStrategy; strategy != nil && strategy.MergeWithState != nil &&
		!slices.Contains(mockMergeStrategies, *strategy.MergeWithState) {
		errs = append(errs, InvalidOutputsMergeStrategyError{
			ConfigPath: configPath,
			Strategy:   string(*strategy.MergeWithState),
		})
	}

	return errors.Join(errs...)
}

// Names returns the declared output names in source order.
func (c *OutputsConfig) Names() []string {
	if c == nil {
		return nil
	}

	names := make([]string, 0, len(c.Outputs))
	for _, output := range c.Outputs {
		names = append(names, output.Name)
	}

	return names
}

// Declares reports whether c declares an output named name.
func (c *OutputsConfig) Declares(name string) bool {
	return c != nil && slices.ContainsFunc(c.Outputs, func(output *OutputDeclaration) bool {
		return output.Name == name
	})
}

// MockValues returns an object of the mock values the declared outputs carry, keyed by output name, or nil when
// no output declares one.
func (c *OutputsConfig) MockValues() *cty.Value {
	if c == nil {
		return nil
	}

	values := make(map[string]cty.Value, len(c.Outputs))

	for _, output := range c.Outputs {
		if output.MockValue != nil {
			values[output.Name] = *output.MockValue
		}
	}

	if len(values) == 0 {
		return nil
	}

	object := cty.ObjectVal(values)

	return &object
}

// Clone returns a deep copy of s.
func (s *OutputsMockStrategy) Clone() *OutputsMockStrategy {
	if s == nil {
		return nil
	}

	clone := &OutputsMockStrategy{}

	if s.AllowedCommands != nil {
		clone.AllowedCommands = new(slices.Clone(*s.AllowedCommands))
	}

	if s.MergeWithState != nil {
		clone.MergeWithState = new(*s.MergeWithState)
	}

	return clone
}

// merge replaces each attribute of s that other sets.
func (s *OutputsMockStrategy) merge(other *OutputsMockStrategy) {
	if other.AllowedCommands != nil {
		s.AllowedCommands = new(slices.Clone(*other.AllowedCommands))
	}

	if other.MergeWithState != nil {
		s.MergeWithState = new(*other.MergeWithState)
	}
}

// Clone returns a copy of d.
func (d *OutputDeclaration) Clone() *OutputDeclaration {
	if d == nil {
		return nil
	}

	clone := &OutputDeclaration{Name: d.Name}

	if d.MockValue != nil {
		clone.MockValue = new(*d.MockValue)
	}

	return clone
}

// DependencyOutputReference is a reference in a config file to a named output of a dependency, such as
// `dependency.vpc.outputs.vpc_id`.
type DependencyOutputReference struct {
	// Dependency is the dependency block label.
	Dependency string
	// Output is the referenced output name.
	Output string
	// Range is where the reference is in the file.
	Range hcl.Range
}

// DependencyOutputReferences returns every reference in file to a named output of a dependency, in source order.
// A reference whose output name is not a literal, such as `dependency.vpc.outputs[local.key]`, is left out, as
// is a reference to the outputs object as a whole. A JSON config has no traversals to walk and yields none.
//
// Returns the diagnostics of the walk as an error when it reports any.
func DependencyOutputReferences(file *hclparse.File) ([]DependencyOutputReference, error) {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}

	var refs []DependencyOutputReference

	diags := hclsyntax.VisitAll(body, func(node hclsyntax.Node) hcl.Diagnostics {
		expr, ok := node.(*hclsyntax.ScopeTraversalExpr)
		if !ok {
			return nil
		}

		if ref, ok := dependencyOutputReference(expr.Traversal); ok {
			refs = append(refs, ref)
		}

		return nil
	})
	if diags.HasErrors() {
		return nil, diags
	}

	// The walk visits the attributes of a body in map order, so sort by position for a stable report.
	slices.SortFunc(refs, func(a, b DependencyOutputReference) int {
		return cmp.Compare(a.Range.Start.Byte, b.Range.Start.Byte)
	})

	return refs, nil
}

// OutputDeclarationRanges returns where each output block of the outputs block in file is declared, by output
// name. A name declared twice keeps its first range. A JSON config yields none.
func OutputDeclarationRanges(file *hclparse.File) map[string]hcl.Range {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}

	ranges := map[string]hcl.Range{}

	for _, block := range body.Blocks {
		if block.Type != MetadataOutputs {
			continue
		}

		for _, output := range block.Body.Blocks {
			if output.Type != MetadataOutput || len(output.Labels) != 1 {
				continue
			}

			if _, seen := ranges[output.Labels[0]]; !seen {
				ranges[output.Labels[0]] = output.DefRange()
			}
		}
	}

	return ranges
}

// dependencyOutputReference reads traversal as `dependency.<name>[<key>].outputs.<output>`, where the instance key
// is optional and the output may be an attribute or a literal string index.
func dependencyOutputReference(traversal hcl.Traversal) (DependencyOutputReference, bool) {
	if traversal.RootName() != MetadataDependency {
		return DependencyOutputReference{}, false
	}

	name, rest, ok := attributeStep(traversal[1:])
	if !ok {
		return DependencyOutputReference{}, false
	}

	if len(rest) > 0 {
		if _, isIndex := rest[0].(hcl.TraverseIndex); isIndex {
			rest = rest[1:]
		}
	}

	outputs, rest, ok := attributeStep(rest)
	if !ok || outputs != "outputs" {
		return DependencyOutputReference{}, false
	}

	output, _, ok := keyStep(rest)
	if !ok {
		return DependencyOutputReference{}, false
	}

	return DependencyOutputReference{
		Dependency: name,
		Output:     output,
		Range:      traversal.SourceRange(),
	}, true
}

// attributeStep returns the name of the attribute step at the head of steps and the steps after it.
func attributeStep(steps hcl.Traversal) (string, hcl.Traversal, bool) {
	if len(steps) == 0 {
		return "", nil, false
	}

	attr, ok := steps[0].(hcl.TraverseAttr)
	if !ok {
		return "", nil, false
	}

	return attr.Name, steps[1:], true
}

// keyStep returns the name the step at the head of steps addresses: an attribute name or a known string index.
func keyStep(steps hcl.Traversal) (string, hcl.Traversal, bool) {
	if name, rest, ok := attributeStep(steps); ok {
		return name, rest, true
	}

	if len(steps) == 0 {
		return "", nil, false
	}

	index, ok := steps[0].(hcl.TraverseIndex)
	if !ok || index.Key.IsNull() || !index.Key.IsKnown() || !index.Key.Type().Equals(cty.String) {
		return "", nil, false
	}

	return index.Key.AsString(), steps[1:], true
}

// mergeDeclaredMockValues returns the mock values a dependent reads: overrides, the dependent's mock_outputs, on
// top of declared, the mock_value attributes of the unit's outputs block, key by key. Either side may be nil. An
// override that is not a known object or map replaces declared outright.
func mergeDeclaredMockValues(declared, overrides *cty.Value) *cty.Value {
	if overrides == nil {
		return declared
	}

	if declared == nil {
		return overrides
	}

	if overrides.IsNull() || !overrides.IsKnown() || !overrides.CanIterateElements() {
		return overrides
	}

	merged := make(map[string]cty.Value, declared.LengthInt()+overrides.LengthInt())
	maps.Copy(merged, declared.AsValueMap())
	maps.Copy(merged, overrides.AsValueMap())

	object := cty.ObjectVal(merged)

	return &object
}

// OutputsBlockRequiresExperimentError reports an outputs block read without the outputs-block experiment.
type OutputsBlockRequiresExperimentError struct {
	ConfigPath string
}

func (err OutputsBlockRequiresExperimentError) Error() string {
	return fmt.Sprintf(
		"%s declares an outputs block, which needs the %s experiment. "+
			"Enable it with --experiment %s or TG_EXPERIMENT=%s, or remove the block.",
		err.ConfigPath,
		experiment.OutputsBlock,
		experiment.OutputsBlock,
		experiment.OutputsBlock,
	)
}

// DuplicateOutputDeclarationError reports an output declared more than once in an outputs block.
type DuplicateOutputDeclarationError struct {
	ConfigPath string
	Name       string
}

func (err DuplicateOutputDeclarationError) Error() string {
	return fmt.Sprintf(
		"%s declares output %q more than once in its outputs block. Keep one output %q block.",
		err.ConfigPath,
		err.Name,
		err.Name,
	)
}

// InvalidOutputsMergeStrategyError reports a merge_with_state value that is not a mock merge strategy.
type InvalidOutputsMergeStrategyError struct {
	ConfigPath string
	Strategy   string
}

func (err InvalidOutputsMergeStrategyError) Error() string {
	valid := make([]string, 0, len(mockMergeStrategies))
	for _, strategy := range mockMergeStrategies {
		valid = append(valid, string(strategy))
	}

	return fmt.Sprintf(
		"merge_with_state %q in the outputs block of %s is unknown. Valid values are: %s.",
		err.Strategy,
		err.ConfigPath,
		strings.Join(valid, ", "),
	)
}

// DeclaredOutputsReadError reports that the outputs block of a dependency could not be read.
type DeclaredOutputsReadError struct {
	Err          error
	TargetConfig string
}

func (err DeclaredOutputsReadError) Error() string {
	return fmt.Sprintf("reading the outputs block of %s: %v", err.TargetConfig, err.Err)
}

func (err DeclaredOutputsReadError) Unwrap() error {
	return err.Err
}

// DeclaredOutputMissingError reports a declared output that the state of the unit does not hold.
type DeclaredOutputMissingError struct {
	DependencyName string
	TargetConfig   string
	CurrentConfig  string
	Output         string
}

func (err DeclaredOutputMissingError) Error() string {
	return fmt.Sprintf(
		"%s declares output %q in its outputs block, but its state has no output %q, read as dependency %q by %s. "+
			"Apply %s after adding the output to its module, remove the declaration, "+
			"or set merge_with_state = \"shallow\" in its mock_strategy block to fill it from mock_value.",
		err.TargetConfig,
		err.Output,
		err.Output,
		err.DependencyName,
		err.CurrentConfig,
		err.TargetConfig,
	)
}
