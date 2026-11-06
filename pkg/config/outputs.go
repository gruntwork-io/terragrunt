package config

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"math/big"
	"slices"
	"strconv"
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

// maxBlockNesting bounds the block nesting the reference walk descends into.
const maxBlockNesting = 32

// OutputsConfig is the outputs block of a unit. It declares the outputs the unit emits, each with the mock value
// its dependents read while the unit's state holds no outputs, and the strategy that says when mocks apply.
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
// `dependency.vpc.outputs.vpc_id` or `dependency.rds["primary"].outputs.endpoint`.
type DependencyOutputReference struct {
	// Instance is the literal key of the instance of an expanded dependency the reference reads, nil when it reads
	// the dependency whole.
	Instance *string
	// Dependency is the dependency block label.
	Dependency string
	// Output is the referenced output name.
	Output string
	// Range is where the reference is in the file.
	Range hcl.Range
}

// Address returns the address of the dependency the reference reads, as [Dependency.Address] gives it: the label
// alone, or label[key] for one instance of an expanded dependency.
func (r *DependencyOutputReference) Address() string {
	return hclparse.InstanceKey{EachKey: r.Instance}.Address(r.Dependency)
}

// DependencyOutputReferences returns every reference in file to a named output of a dependency, in source order,
// given the dependency blocks the file resolves against: a reference to a dependency that declares an expansion
// names one instance, and one to any other dependency names the dependency whole. Each step may be spelled as an
// attribute or as a literal string index, so `dependency["vpc"].outputs["vpc_id"]` is `dependency.vpc.outputs.vpc_id`.
// A reference whose output name or instance key is not a literal, such as `dependency.vpc.outputs[local.key]` or
// `dependency.rds[local.name].outputs.endpoint`, is left out, as is a reference to the outputs object as a whole
// and a reference through a variable a for expression binds. A JSON config has no traversals to walk and yields
// none.
//
// Returns [BlockNestingExceededError] when the blocks of file nest deeper than the walk descends.
func DependencyOutputReferences(
	file *hclparse.File,
	dependencies Dependencies,
) ([]DependencyOutputReference, error) {
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, nil
	}

	refs, err := bodyDependencyOutputReferences(file.ConfigPath, body, dependencies.expandedLabels(), 0, nil)
	if err != nil {
		return nil, err
	}

	// The attributes of a body are visited in map order, so sort by position for a stable report.
	slices.SortFunc(refs, func(a, b DependencyOutputReference) int {
		return cmp.Compare(a.Range.Start.Byte, b.Range.Start.Byte)
	})

	return refs, nil
}

// bodyDependencyOutputReferences appends the references of the attributes of body, and of the bodies of its
// blocks, to refs. It reads the variables of each attribute, which leave out the names a for expression binds, so
// a loop variable named dependency is not read as the dependency object.
func bodyDependencyOutputReferences(
	configPath string,
	body *hclsyntax.Body,
	expanded map[string]struct{},
	depth int,
	refs []DependencyOutputReference,
) ([]DependencyOutputReference, error) {
	if depth > maxBlockNesting {
		return nil, BlockNestingExceededError{ConfigPath: configPath, Limit: maxBlockNesting}
	}

	for _, attr := range body.Attributes {
		for _, traversal := range attr.Expr.Variables() {
			if ref, ok := dependencyOutputReference(traversal, expanded); ok {
				refs = append(refs, ref)
			}
		}
	}

	for _, block := range body.Blocks {
		var err error

		refs, err = bodyDependencyOutputReferences(configPath, block.Body, expanded, depth+1, refs)
		if err != nil {
			return nil, err
		}
	}

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

// dependencyOutputReference reads traversal as `dependency.<name>.outputs.<output>`, or as
// `dependency.<name>.<key>.outputs.<output>` when name is in expanded, the labels of the dependency blocks that
// declare an expansion. Every step may be an attribute or a literal string index, and the instance key may also
// be a whole number index.
func dependencyOutputReference(
	traversal hcl.Traversal,
	expanded map[string]struct{},
) (DependencyOutputReference, bool) {
	if traversal.RootName() != MetadataDependency {
		return DependencyOutputReference{}, false
	}

	name, rest, ok := keyStep(traversal[1:])
	if !ok {
		return DependencyOutputReference{}, false
	}

	var instance *string

	if _, isExpanded := expanded[name]; isExpanded {
		instance, rest, ok = instanceStep(rest)
		if !ok {
			return DependencyOutputReference{}, false
		}
	}

	outputs, rest, ok := keyStep(rest)
	if !ok || outputs != "outputs" {
		return DependencyOutputReference{}, false
	}

	output, _, ok := keyStep(rest)
	if !ok {
		return DependencyOutputReference{}, false
	}

	return DependencyOutputReference{
		Instance:   instance,
		Dependency: name,
		Output:     output,
		Range:      traversal.SourceRange(),
	}, true
}

// instanceStep returns the instance key the step at the head of steps addresses, as the dependency map keys it,
// and the steps after it: an attribute name, or a string or whole number index. Any other step is not literal.
func instanceStep(steps hcl.Traversal) (*string, hcl.Traversal, bool) {
	if len(steps) == 0 {
		return nil, nil, false
	}

	switch step := steps[0].(type) {
	case hcl.TraverseAttr:
		return new(step.Name), steps[1:], true
	case hcl.TraverseIndex:
		key, ok := literalInstanceKey(step.Key)
		if !ok {
			return nil, nil, false
		}

		return new(key), steps[1:], true
	default:
		return nil, nil, false
	}
}

// literalInstanceKey returns key as the dependency map keys an instance: a string as is, and a whole number in
// decimal. Any other key is not literal.
func literalInstanceKey(key cty.Value) (string, bool) {
	if key.IsNull() {
		return "", false
	}

	switch {
	case key.Type().Equals(cty.String):
		return key.AsString(), true
	case key.Type().Equals(cty.Number):
		whole, accuracy := key.AsBigFloat().Int64()
		if accuracy != big.Exact {
			return "", false
		}

		return strconv.FormatInt(whole, 10), true
	default:
		return "", false
	}
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

	overrideValues, ok := mockValueMap(overrides)
	if !ok {
		return overrides
	}

	merged := make(map[string]cty.Value, declared.LengthInt()+len(overrideValues))
	maps.Copy(merged, declared.AsValueMap())
	maps.Copy(merged, overrideValues)

	object := cty.ObjectVal(merged)

	return &object
}

// mockValueMap returns the entries of mocks, a mock_outputs value, by key, each carrying the marks of mocks as a
// whole, so the entries of a sensitive() object stay sensitive on their own. It reports false when mocks is not a
// known object or map, which no mock merge reads by key.
func mockValueMap(mocks *cty.Value) (map[string]cty.Value, bool) {
	if mocks == nil || mocks.IsNull() || !mocks.IsKnown() {
		return nil, false
	}

	if mocksType := mocks.Type(); !mocksType.IsObjectType() && !mocksType.IsMapType() {
		return nil, false
	}

	unmarked, marks := mocks.Unmark()
	values := unmarked.AsValueMap()

	if len(marks) == 0 {
		return values, true
	}

	for key, value := range values {
		values[key] = value.WithMarks(marks)
	}

	return values, true
}

// expandedLabels returns the labels of the dependency blocks that declare an expansion, whose references name
// one instance.
func (deps Dependencies) expandedLabels() map[string]struct{} {
	expanded := map[string]struct{}{}

	for i := range deps {
		if deps[i].Expansion != nil {
			expanded[deps[i].Name] = struct{}{}
		}
	}

	return expanded
}

// BlockNestingExceededError reports a config file whose blocks nest deeper than the reference walk descends.
type BlockNestingExceededError struct {
	ConfigPath string
	Limit      int
}

func (err BlockNestingExceededError) Error() string {
	return fmt.Sprintf(
		"%s nests blocks more than %d levels deep. Flatten the nested blocks.",
		err.ConfigPath,
		err.Limit,
	)
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
