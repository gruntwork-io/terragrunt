package config

import "github.com/zclconf/go-cty/cty"

// FeatureFlags represents a list of feature flags
type FeatureFlags []*FeatureFlag

// FeatureFlag feature flags struct
type FeatureFlag struct {
	Name    string     `hcl:",label" cty:"name"`
	Default *cty.Value `hcl:"default,attr" cty:"default"`
}

// ctyFeatureFlag struct used to pass FeatureFlag to cty.Value
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
