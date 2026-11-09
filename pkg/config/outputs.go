package config

import (
	"errors"
	"slices"

	"github.com/zclconf/go-cty/cty"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
)

const (
	// MetadataOutputs is the name of the outputs block.
	MetadataOutputs = "outputs"
	// MetadataOutput is the name of the output blocks nested in an outputs block.
	MetadataOutput = "output"
	// MetadataMockStrategy is the name of the mock_strategy block nested in an outputs block.
	MetadataMockStrategy = "mock_strategy"
)

// mockMergeStrategies is the merge strategies a mock_strategy block may name.
var mockMergeStrategies = []MergeStrategyType{NoMerge, ShallowMerge, DeepMergeMapOnly}

// OutputsConfig is the outputs block of a unit: its declared outputs, their mock values and the mock strategy.
type OutputsConfig struct {
	// MockStrategy is the mock_strategy block, nil when the block has none.
	MockStrategy *OutputsMockStrategy `hcl:"mock_strategy,block"`
	// Outputs is the output blocks, in source order.
	Outputs []*OutputDeclaration `hcl:"output,block"`
}

// OutputsMockStrategy is the mock_strategy block of an outputs block, overridden by a dependency's own mock attributes.
type OutputsMockStrategy struct {
	// AllowedCommands is the commands that may read mock values, every command when unset or empty.
	AllowedCommands *[]string `hcl:"allowed_commands,attr"`
	// MergeWithState is how the state of the unit is merged into the mock values: no_merge, shallow or deep_map_only.
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

// MockValues returns an object of the mock values the declared outputs set, by output name, or nil when none is set.
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
