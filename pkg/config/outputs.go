package config

import (
	"errors"
	"slices"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
)

const (
	// MetadataOutputs names the `outputs` block.
	MetadataOutputs = "outputs"
	// MetadataOutput names the `output` blocks of the `outputs` block.
	MetadataOutput = "output"
	// MetadataMockStrategy names the `mock_strategy` block of the `outputs` block.
	MetadataMockStrategy = "mock_strategy"
)

var mockMergeStrategies = []MergeStrategyType{NoMerge, ShallowMerge, DeepMergeMapOnly}

// OutputsConfig is the outputs block of a unit.
type OutputsConfig struct {
	MockStrategy *OutputsMockStrategy `hcl:"mock_strategy,block"`
	Outputs      []*OutputDeclaration `hcl:"output,block"`
}

// OutputsMockStrategy is the mock_strategy block of an outputs block or of an output block.
type OutputsMockStrategy struct {
	AllowedCommands *[]string          `hcl:"allowed_commands,attr"`
	MergeWithState  *MergeStrategyType `hcl:"merge_with_state,attr"`
}

// OutputDeclaration is an output block of an outputs block.
type OutputDeclaration struct {
	// MockValue is the mock value converted to Type, or nil when the block sets none.
	MockValue *cty.Value `hcl:"mock_value,optional"`
	// MockStrategy overrides the mock_strategy of the outputs block for this output, attribute by attribute.
	MockStrategy *OutputsMockStrategy `hcl:"mock_strategy,block"`
	// Type is the type constraint, or [cty.NilType] when the block sets none.
	Type cty.Type `hcl:"type,optional"`
	// Name is the block label.
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

// Merge merges other into c, with other winning by output name and by mock_strategy attribute.
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

// Validate checks that the experiment is on, no output is declared twice and merge_with_state names a mock strategy.
func (c *OutputsConfig) Validate(experiments experiment.Experiments, configPath string) error {
	if c == nil {
		return nil
	}

	if !experiments.Evaluate(experiment.OutputsBlock) {
		return OutputsBlockRequiresExperimentError{ConfigPath: configPath}
	}

	var errs []error

	seen := make(map[string]struct{}, len(c.Outputs))
	strategies := []*OutputsMockStrategy{c.MockStrategy}

	for _, output := range c.Outputs {
		if _, duplicate := seen[output.Name]; duplicate {
			errs = append(errs, DuplicateOutputDeclarationError{ConfigPath: configPath, Name: output.Name})
		}

		seen[output.Name] = struct{}{}
		strategies = append(strategies, output.MockStrategy)
	}

	for _, strategy := range strategies {
		if strategy == nil || strategy.MergeWithState == nil ||
			slices.Contains(mockMergeStrategies, *strategy.MergeWithState) {
			continue
		}

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
	return c.Declaration(name) != nil
}

// Declaration returns the output block named name, or nil when c declares none.
func (c *OutputsConfig) Declaration(name string) *OutputDeclaration {
	if c == nil {
		return nil
	}

	i := slices.IndexFunc(c.Outputs, func(output *OutputDeclaration) bool {
		return output.Name == name
	})
	if i < 0 {
		return nil
	}

	return c.Outputs[i]
}

// MockValues returns an object of the mock values the declared outputs set, by output name, or nil when none is set.
func (c *OutputsConfig) MockValues() *cty.Value {
	if c == nil || len(c.Outputs) == 0 {
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

	clone := &OutputDeclaration{Name: d.Name, Type: d.Type, MockStrategy: d.MockStrategy.Clone()}

	if d.MockValue != nil {
		clone.MockValue = new(*d.MockValue)
	}

	return clone
}

// Conform returns value converted to the type of d, or value as it is when d sets no type.
func (d *OutputDeclaration) Conform(value cty.Value) (cty.Value, error) {
	if d.Type == cty.NilType {
		return value, nil
	}

	return convert.Convert(value, d.Type)
}
