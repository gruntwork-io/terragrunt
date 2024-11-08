package config

import (
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// FeatureFlags represents a list of feature flags.
type FeatureFlags []*FeatureFlag

// FeatureFlag feature flags struct.
type FeatureFlag struct {
	Name    string     `cty:"name"    hcl:",label"`
	Default *cty.Value `cty:"default" hcl:"default,attr"`
}

// ctyFeatureFlag struct used to pass FeatureFlag to cty.Value.
type ctyFeatureFlag struct {
	Name  string    `cty:"name"`
	Value cty.Value `cty:"value"`
}

// DeepMerge merges the source FeatureFlag into the target FeatureFlag.
func (feature *FeatureFlag) DeepMerge(source *FeatureFlag) error {
	if source.Name != "" {
		feature.Name = source.Name
	}

	if source.Default == nil {
		feature.Default = source.Default
	} else {
		updatedDefaults, err := deepMergeCtyMaps(*feature.Default, *source.Default)
		if err != nil {
			return err
		}

		feature.Default = updatedDefaults
	}

	return nil
}

// DefaultAsString returns the default value of the feature flag as a string.
func (feature *FeatureFlag) DefaultAsString() (string, error) {
	if feature.Default == nil {
		return "", nil
	}

	if feature.Default.Type() == cty.String {
		return feature.Default.AsString(), nil
	}

	// convert other types as json representation
	jsonBytes, err := ctyjson.Marshal(*feature.Default, feature.Default.Type())
	if err != nil {
		return "", errors.WithStack(err)
	}

	return string(jsonBytes), nil
}
